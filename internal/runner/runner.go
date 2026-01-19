package runner

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/nholik/swarm-sentinel/internal/compose"
	"github.com/nholik/swarm-sentinel/internal/health"
	"github.com/nholik/swarm-sentinel/internal/healthcheck"
	"github.com/nholik/swarm-sentinel/internal/metrics"
	"github.com/nholik/swarm-sentinel/internal/notify"
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
	logger                   zerolog.Logger
	pollInterval             time.Duration
	tickerFactory            func(time.Duration) Ticker
	runOnce                  func(context.Context) error
	composeFetcher           compose.Fetcher
	swarmClient              swarm.Client
	stackName                string
	composeETag              string
	composeHash              string
	lastDesiredState         *compose.DesiredState
	lastActualState          *swarm.ActualState
	stateStore               state.Store
	stateMu                  *sync.Mutex
	notifier                 notify.Notifier
	alertStabilizationCycles int
	cycleTracker             *healthcheck.Tracker
	metrics                  *metrics.Metrics
	stacksEvaluated          int
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

// WithNotifier enables transition notifications.
func WithNotifier(notifier notify.Notifier) Option {
	return func(r *Runner) {
		r.notifier = notifier
	}
}

// WithAlertStabilizationCycles sets how many consecutive cycles a status must persist before alerting.
func WithAlertStabilizationCycles(cycles int) Option {
	return func(r *Runner) {
		r.alertStabilizationCycles = cycles
	}
}

// WithCycleTracker records cycle timing for health endpoints.
func WithCycleTracker(tracker *healthcheck.Tracker) Option {
	return func(r *Runner) {
		r.cycleTracker = tracker
	}
}

// WithMetrics attaches Prometheus metrics collectors.
func WithMetrics(metricsCollector *metrics.Metrics) Option {
	return func(r *Runner) {
		r.metrics = metricsCollector
	}
}

// WithStacksEvaluated sets the total number of stacks evaluated per cycle.
func WithStacksEvaluated(count int) Option {
	return func(r *Runner) {
		r.stacksEvaluated = count
	}
}

// New constructs a Runner with the given logger and poll interval.
func New(logger zerolog.Logger, pollInterval time.Duration, opts ...Option) *Runner {
	r := &Runner{
		logger:                   logger,
		pollInterval:             pollInterval,
		alertStabilizationCycles: 1,
		stacksEvaluated:          1,
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
		logEvent := r.logger.Error().Err(err)
		var runtimeErr *RuntimeError
		if errors.As(err, &runtimeErr) {
			logEvent = logEvent.Bool("runtime_error", true)
		}
		logEvent.Msg("initial run cycle failed")
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
				logEvent := r.logger.Error().Err(err)
				var runtimeErr *RuntimeError
				if errors.As(err, &runtimeErr) {
					logEvent = logEvent.Bool("runtime_error", true)
				}
				logEvent.Msg("run cycle failed")
			}
		}
	}
}

// RunOnce executes a single cycle of the runner.
func (r *Runner) RunOnce(ctx context.Context) error {
	start := time.Now()
	err := r.runOnce(ctx)
	if err == nil {
		duration := time.Since(start)
		if r.cycleTracker != nil {
			stacksEvaluated := r.stacksEvaluated
			if stacksEvaluated <= 0 {
				stacksEvaluated = 1
			}
			r.cycleTracker.RecordCycle(duration, stacksEvaluated)
		}
		if r.metrics != nil {
			r.metrics.ObserveCycleDuration(duration)
			r.metrics.SetLastSuccessfulCycleTimestamp(time.Now().UTC())
		}
	}
	return err
}

func (r *Runner) defaultRunOnce(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if r.composeFetcher != nil {
		result, err := r.composeFetcher.Fetch(ctx, r.composeETag)
		if err != nil {
			return wrapRuntime("compose fetch", err)
		}

		if result.ETag != "" {
			r.composeETag = result.ETag
		}

		if result.NotModified {
			r.logger.Debug().Msg("compose unchanged")
		} else {
			fingerprint, err := compose.Fingerprint(result.Body)
			if err != nil {
				return wrapRuntime("compose fingerprint", err)
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
					return wrapRuntime("compose parse", err)
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

	if err := ctx.Err(); err != nil {
		return err
	}

	if r.lastDesiredState == nil {
		r.logger.Warn().Msg("desired state not yet available, collecting actual state only")
	}

	actualState, err := r.swarmClient.GetActualState(ctx, r.stackName)
	if err != nil {
		if r.metrics != nil {
			r.metrics.IncDockerAPIErrors()
		}
		return wrapRuntime("swarm actual state", err)
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
			return wrapRuntime("state evaluation", err)
		}
	}

	return nil
}

func (r *Runner) evaluateAndPersist(ctx context.Context) error {
	stackScoped := r.stackName != ""
	stackHealth := health.EvaluateStackHealth(*r.lastDesiredState, r.lastActualState, stackScoped)

	if r.lastActualState != nil {
		for _, service := range r.lastActualState.Services {
			if service.UpdateState == "" {
				continue
			}
			r.logger.Info().
				Str("service", service.Name).
				Str("update_state", service.UpdateState).
				Str("stack_name", r.stackKey()).
				Msg("service update status")
		}
	}

	stackKey := r.stackKey()
	now := time.Now().UTC()

	var snapshot *state.StackSnapshot
	var updatedServices map[string]health.ServiceHealth
	var transitions []transition.ServiceTransition
	err := r.withStateLock(func() error {
		loaded, err := r.stateStore.Load(ctx)
		if err != nil {
			return wrapRuntime("state load", err)
		}
		if existing, ok := loaded.Stacks[stackKey]; ok {
			copySnapshot := existing
			snapshot = &copySnapshot
		}

		updatedServices, transitions = r.stabilizeTransitions(snapshot, stackHealth)
		if loaded.Stacks == nil {
			loaded.Stacks = map[string]state.StackSnapshot{}
		}
		loaded.Stacks[stackKey] = state.StackSnapshot{
			DesiredFingerprint: r.composeHash,
			Services:           updatedServices,
			EvaluatedAt:        now,
		}

		if err := r.stateStore.Save(ctx, loaded); err != nil {
			return wrapRuntime("state save", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	r.logCycleSummary(stackHealth, transitions)
	r.recordMetrics(stackHealth, transitions)
	for _, change := range transitions {
		var event *zerolog.Event
		switch change.CurrentStatus {
		case health.StatusFailed:
			event = r.logger.Error()
		case health.StatusDegraded:
			event = r.logger.Warn()
		default:
			event = r.logger.Info()
		}

		event = event.
			Str("service", change.Name).
			Str("previous_status", string(change.PreviousStatus)).
			Str("current_status", string(change.CurrentStatus)).
			Strs("reasons", change.Reasons)

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
		event = event.Str("stack_name", r.stackKey())
		event.Msg("service transition detected")
	}

	if r.notifier != nil && len(transitions) > 0 {
		if err := r.notifier.Notify(ctx, r.stackKey(), transitions); err != nil {
			r.logger.Error().Err(err).Msg("failed to send notifications")
		}
	}

	return nil
}

func (r *Runner) recordMetrics(stackHealth health.StackHealth, transitions []transition.ServiceTransition) {
	if r.metrics == nil {
		return
	}

	okCount := 0
	degradedCount := 0
	failedCount := 0
	for _, service := range stackHealth.Services {
		switch service.Status {
		case health.StatusOK:
			okCount++
		case health.StatusDegraded:
			degradedCount++
		case health.StatusFailed:
			failedCount++
		}
	}

	stack := r.stackKey()
	r.metrics.SetServicesTotal(stack, "ok", okCount)
	r.metrics.SetServicesTotal(stack, "degraded", degradedCount)
	r.metrics.SetServicesTotal(stack, "failed", failedCount)

	for _, change := range transitions {
		severity := strings.ToLower(string(change.CurrentStatus))
		if severity == "" {
			severity = "unknown"
		}
		r.metrics.IncAlertsTotal(stack, severity)
	}
}

func (r *Runner) stabilizeTransitions(prev *state.StackSnapshot, current health.StackHealth) (map[string]health.ServiceHealth, []transition.ServiceTransition) {
	stabilization := r.alertStabilizationCycles
	if stabilization <= 0 {
		stabilization = 1
	}

	prevServices := map[string]health.ServiceHealth{}
	if prev != nil && prev.Services != nil {
		prevServices = prev.Services
	}
	firstRun := prev == nil || len(prevServices) == 0

	updatedServices := make(map[string]health.ServiceHealth, len(current.Services))
	eligibleServices := make(map[string]health.ServiceHealth)

	for name, service := range current.Services {
		prevService, hadPrev := prevServices[name]
		consecutive := 1
		if hadPrev && prevService.Status == service.Status {
			if prevService.ConsecutiveCycles > 0 {
				consecutive = prevService.ConsecutiveCycles + 1
			} else {
				consecutive = 2
			}
		}

		lastNotified := prevService.LastNotifiedStatus
		if lastNotified == "" && hadPrev {
			lastNotified = prevService.Status
		}

		service.ConsecutiveCycles = consecutive
		service.LastNotifiedStatus = lastNotified

		shouldNotify := false
		if firstRun {
			shouldNotify = service.Status != health.StatusOK
		} else if service.Status != lastNotified {
			shouldNotify = stabilization <= 1 || consecutive >= stabilization
		}

		if shouldNotify {
			eligibleServices[name] = service
			service.LastNotifiedStatus = service.Status
		}

		updatedServices[name] = service
	}

	transitions := transition.DetectServiceTransitions(prev, health.StackHealth{
		Status:   current.Status,
		Services: eligibleServices,
	})
	return updatedServices, transitions
}

func (r *Runner) logCycleSummary(stackHealth health.StackHealth, transitions []transition.ServiceTransition) {
	okCount := 0
	degradedCount := 0
	failedCount := 0

	for _, service := range stackHealth.Services {
		switch service.Status {
		case health.StatusOK:
			okCount++
		case health.StatusDegraded:
			degradedCount++
		case health.StatusFailed:
			failedCount++
		}
	}

	r.logger.Info().
		Str("stack_name", r.stackKey()).
		Str("fingerprint", r.composeHash).
		Int("services_evaluated", len(stackHealth.Services)).
		Int("services_ok", okCount).
		Int("services_degraded", degradedCount).
		Int("services_failed", failedCount).
		Int("transitions", len(transitions)).
		Msg("health evaluation summary")
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
