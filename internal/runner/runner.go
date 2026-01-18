package runner

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/nholik/swarm-sentinel/internal/compose"
	"github.com/nholik/swarm-sentinel/internal/health"
	"github.com/nholik/swarm-sentinel/internal/state"
	"github.com/nholik/swarm-sentinel/internal/swarm"
	"github.com/nholik/swarm-sentinel/internal/transition"
	"github.com/rs/zerolog"
)

// Ticker is the minimal interface needed for driving the runner loop.
type Ticker interface {
	C() <-chan time.Time
	Stop()
}

type timeTicker struct {
	ticker *time.Ticker
}

func (t timeTicker) C() <-chan time.Time {
	return t.ticker.C
}

func (t timeTicker) Stop() {
	t.ticker.Stop()
}

// Runner orchestrates the main execution loop.
type Runner struct {
	logger           zerolog.Logger
	pollInterval     time.Duration
	tickerFactory    func(time.Duration) Ticker
	runOnce          func(context.Context) error
	composeFetcher   compose.Fetcher
	swarmClient      swarm.Client
	stackName        string
	composeETag      string
	composeHash      string
	lastDesiredState *compose.DesiredState
	lastActualState  *swarm.ActualState
	stateStore       state.Store
	stateMu          *sync.Mutex
}

// Option customizes runner behavior.
type Option func(*Runner)

// WithTickerFactory overrides how tickers are created.
func WithTickerFactory(factory func(time.Duration) Ticker) Option {
	return func(r *Runner) {
		r.tickerFactory = factory
	}
}

// WithRunOnce overrides the single-cycle execution step.
func WithRunOnce(runOnce func(context.Context) error) Option {
	return func(r *Runner) {
		r.runOnce = runOnce
	}
}

// WithComposeFetcher sets the compose fetcher used by the default RunOnce.
func WithComposeFetcher(fetcher compose.Fetcher) Option {
	return func(r *Runner) {
		r.composeFetcher = fetcher
	}
}

// WithSwarmClient sets the Swarm client used by the default RunOnce.
func WithSwarmClient(client swarm.Client) Option {
	return func(r *Runner) {
		r.swarmClient = client
	}
}

// WithStackName scopes Swarm service collection to a stack name.
func WithStackName(name string) Option {
	return func(r *Runner) {
		r.stackName = name
	}
}

// WithStateStore enables state persistence for transitions.
func WithStateStore(store state.Store, lock *sync.Mutex) Option {
	return func(r *Runner) {
		r.stateStore = store
		r.stateMu = lock
	}
}

// New constructs a Runner with the given logger and poll interval.
func New(logger zerolog.Logger, pollInterval time.Duration, opts ...Option) *Runner {
	r := &Runner{
		logger:       logger,
		pollInterval: pollInterval,
		tickerFactory: func(d time.Duration) Ticker {
			return timeTicker{ticker: time.NewTicker(d)}
		},
	}
	r.runOnce = r.defaultRunOnce

	for _, opt := range opts {
		opt(r)
	}
	if r.stateStore != nil && r.stateMu == nil {
		r.stateMu = &sync.Mutex{}
	}

	return r
}

// Run starts the main loop and blocks until the context is canceled.
func (r *Runner) Run(ctx context.Context) error {
	if r.pollInterval <= 0 {
		return errors.New("poll interval must be greater than zero")
	}

	// Run immediately on startup
	if err := r.RunOnce(ctx); err != nil {
		r.logger.Error().Err(err).Msg("initial run cycle failed")
	}

	ticker := r.tickerFactory(r.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.logger.Info().Msg("runner stopped")
			return nil
		case <-ticker.C():
			if err := r.RunOnce(ctx); err != nil {
				r.logger.Error().Err(err).Msg("run cycle failed")
			}
		}
	}
}

// RunOnce executes a single cycle of the runner.
func (r *Runner) RunOnce(ctx context.Context) error {
	return r.runOnce(ctx)
}

func (r *Runner) defaultRunOnce(ctx context.Context) error {
	if r.composeFetcher != nil {
		result, err := r.composeFetcher.Fetch(ctx, r.composeETag)
		if err != nil {
			return err
		}

		if result.ETag != "" {
			r.composeETag = result.ETag
		}

		if result.NotModified {
			r.logger.Debug().Msg("compose unchanged")
		} else {
			fingerprint, err := compose.Fingerprint(result.Body)
			if err != nil {
				return err
			}
			if fingerprint == r.composeHash {
				r.logger.Debug().Msg("compose fingerprint unchanged")
			} else {
				r.composeHash = fingerprint

				r.logger.Info().
					Int("bytes", len(result.Body)).
					Str("etag", result.ETag).
					Str("last_modified", result.LastModified).
					Str("fingerprint", fingerprint).
					Msg("compose fetched")

				desiredState, err := compose.ParseDesiredState(ctx, result.Body)
				if err != nil {
					return err
				}
				r.lastDesiredState = &desiredState

				r.logger.Info().
					Int("services", len(desiredState.Services)).
					Msg("parsed desired state")
			}
		}
	}

	if r.swarmClient == nil {
		return nil
	}

	if r.lastDesiredState == nil {
		r.logger.Warn().Msg("desired state not yet available, collecting actual state only")
	}

	actualState, err := r.swarmClient.GetActualState(ctx, r.stackName)
	if err != nil {
		return err
	}
	r.lastActualState = actualState

	serviceCount := 0
	runningReplicas := 0
	if actualState != nil {
		serviceCount = len(actualState.Services)
		for _, service := range actualState.Services {
			runningReplicas += service.RunningReplicas
		}
	}

	event := r.logger.Info().
		Int("services", serviceCount).
		Int("running_replicas", runningReplicas)
	if r.stackName != "" {
		event = event.Str("stack_name", r.stackName)
	}
	event.Msg("collected actual state")

	if r.stateStore != nil && r.lastDesiredState != nil {
		if err := r.evaluateAndPersist(ctx); err != nil {
			return err
		}
	}

	return nil
}

func (r *Runner) evaluateAndPersist(ctx context.Context) error {
	stackScoped := r.stackName != ""
	stackHealth := health.EvaluateStackHealth(*r.lastDesiredState, r.lastActualState, stackScoped)

	stackKey := r.stackKey()
	now := time.Now().UTC()

	var snapshot *state.StackSnapshot
	err := r.withStateLock(func() error {
		loaded, err := r.stateStore.Load(ctx)
		if err != nil {
			return err
		}
		if existing, ok := loaded.Stacks[stackKey]; ok {
			copySnapshot := existing
			snapshot = &copySnapshot
		}

		if loaded.Stacks == nil {
			loaded.Stacks = map[string]state.StackSnapshot{}
		}
		loaded.Stacks[stackKey] = state.StackSnapshot{
			DesiredFingerprint: r.composeHash,
			Services:           stackHealth.Services,
			EvaluatedAt:        now,
		}

		return r.stateStore.Save(ctx, loaded)
	})
	if err != nil {
		return err
	}

	transitions := transition.DetectServiceTransitions(snapshot, stackHealth)
	for _, change := range transitions {
		event := r.logger.Info().
			Str("service", change.Name).
			Str("previous_status", string(change.PreviousStatus)).
			Str("current_status", string(change.CurrentStatus)).
			Strs("reasons", change.Reasons)

		switch change.CurrentStatus {
		case health.StatusFailed:
			event = r.logger.Error().
				Str("service", change.Name).
				Str("previous_status", string(change.PreviousStatus)).
				Str("current_status", string(change.CurrentStatus)).
				Strs("reasons", change.Reasons)
		case health.StatusDegraded:
			event = r.logger.Warn().
				Str("service", change.Name).
				Str("previous_status", string(change.PreviousStatus)).
				Str("current_status", string(change.CurrentStatus)).
				Strs("reasons", change.Reasons)
		}

		if change.ReplicaChange != nil {
			event = event.Int("desired_replicas", change.ReplicaChange.CurrentDesired).
				Int("running_replicas", change.ReplicaChange.CurrentRunning).
				Int("desired_delta", change.ReplicaChange.DesiredDelta).
				Int("running_delta", change.ReplicaChange.RunningDelta)
		}
		if change.ImageChange != nil {
			event = event.Str("desired_image", change.ImageChange.CurrentDesired).
				Str("actual_image", change.ImageChange.CurrentActual)
		}
		if len(change.Drift) > 0 {
			event = event.Interface("drift", change.Drift)
		}
		event.Msg("service transition detected")
	}

	return nil
}

func (r *Runner) withStateLock(fn func() error) error {
	if r.stateMu == nil {
		return fn()
	}
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	return fn()
}

func (r *Runner) stackKey() string {
	if r.stackName != "" {
		return r.stackName
	}
	return "default"
}
