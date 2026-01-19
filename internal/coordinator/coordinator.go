package coordinator

import (
	"context"
	"sync"

	"github.com/nholik/swarm-sentinel/internal/compose"
	"github.com/nholik/swarm-sentinel/internal/config"
	"github.com/nholik/swarm-sentinel/internal/healthcheck"
	"github.com/nholik/swarm-sentinel/internal/metrics"
	"github.com/nholik/swarm-sentinel/internal/notify"
	"github.com/nholik/swarm-sentinel/internal/runner"
	"github.com/nholik/swarm-sentinel/internal/state"
	"github.com/nholik/swarm-sentinel/internal/swarm"
	"github.com/rs/zerolog"
)

// Coordinator manages multiple Runner instances, one per stack.
// It spawns runners in parallel and waits for context cancellation.
type Coordinator struct {
	logger                   zerolog.Logger
	cfg                      config.Config
	mappings                 []config.StackMapping
	swarmClient              swarm.Client
	stateStore               state.Store
	stateMu                  *sync.Mutex
	notifier                 notify.Notifier
	cycleTracker             *healthcheck.Tracker
	metrics                  *metrics.Metrics
	alertStabilizationCycles int
	runners                  map[string]*runner.Runner
	runnerErrors             map[string]error
	cancel                   context.CancelFunc
	done                     chan struct{}
	mu                       sync.RWMutex
}

// Option customizes coordinator behavior.
type Option func(*Coordinator)

// New constructs a Coordinator with the given configuration and stack mappings.
func New(logger zerolog.Logger, cfg config.Config, mappings []config.StackMapping, swarmClient swarm.Client, opts ...Option) *Coordinator {
	coord := &Coordinator{
		logger:       logger,
		cfg:          cfg,
		mappings:     mappings,
		swarmClient:  swarmClient,
		runners:      make(map[string]*runner.Runner),
		runnerErrors: make(map[string]error),
	}

	for _, opt := range opts {
		opt(coord)
	}

	return coord
}

// WithStateStore sets the shared state store for all runners.
func WithStateStore(store state.Store, lock *sync.Mutex) Option {
	return func(c *Coordinator) {
		c.stateStore = store
		c.stateMu = lock
	}
}

// WithNotifier enables transition notifications for each runner.
func WithNotifier(notifier notify.Notifier) Option {
	return func(c *Coordinator) {
		c.notifier = notifier
	}
}

// WithCycleTracker shares a cycle tracker with all runners.
func WithCycleTracker(tracker *healthcheck.Tracker) Option {
	return func(c *Coordinator) {
		c.cycleTracker = tracker
	}
}

// WithMetrics shares metrics collectors with all runners.
func WithMetrics(metricsCollector *metrics.Metrics) Option {
	return func(c *Coordinator) {
		c.metrics = metricsCollector
	}
}

// WithAlertStabilizationCycles applies alert stabilization to all runners.
func WithAlertStabilizationCycles(cycles int) Option {
	return func(c *Coordinator) {
		c.alertStabilizationCycles = cycles
	}
}

// Run starts all runners in parallel and blocks until context is canceled.
// Returns nil on clean shutdown; logs any per-runner errors internally.
func (c *Coordinator) Run(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	c.mu.Lock()
	c.cancel = cancel
	c.done = done
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.cancel = nil
		c.done = nil
		c.mu.Unlock()
		close(done)
	}()

	c.logger.Info().
		Int("stacks", len(c.mappings)).
		Msg("starting coordinator")

	// Spawn all runners in parallel
	var wg sync.WaitGroup
	for _, mapping := range c.mappings {
		wg.Add(1)
		go c.spawnRunner(runCtx, &wg, mapping)
	}

	// Wait for all runners to exit (via context cancellation or error)
	wg.Wait()
	c.logger.Info().Msg("all runners stopped")

	// Report any errors (use write lock since runners have finished writing)
	c.mu.Lock()
	for stack, err := range c.runnerErrors {
		if err != nil {
			c.logger.Error().Err(err).Str("stack", stack).Msg("runner error")
		}
	}
	c.mu.Unlock()

	return nil
}

// Stop cancels all running runners and waits for shutdown.
func (c *Coordinator) Stop() {
	c.mu.RLock()
	cancel := c.cancel
	done := c.done
	c.mu.RUnlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

// spawnRunner creates and runs a single Runner for the given stack mapping.
func (c *Coordinator) spawnRunner(ctx context.Context, wg *sync.WaitGroup, mapping config.StackMapping) {
	defer wg.Done()

	stackLogger := c.logger.With().Str("stack", mapping.Name).Logger()

	// Determine timeout: per-stack override or global default
	timeout := c.cfg.ComposeTimeout
	if mapping.Timeout > 0 {
		timeout = mapping.Timeout
	}

	// Create HTTP fetcher for this stack's compose URL
	fetcher, err := compose.NewHTTPFetcher(mapping.ComposeURL, timeout, 0)
	if err != nil {
		stackLogger.Error().Err(err).Msg("failed to initialize compose fetcher")
		c.recordError(mapping.Name, err)
		return
	}

	// Create runner for this stack
	opts := []runner.Option{
		runner.WithComposeFetcher(fetcher),
		runner.WithSwarmClient(c.swarmClient),
		runner.WithStackName(mapping.Name),
	}
	if c.stateStore != nil {
		opts = append(opts, runner.WithStateStore(c.stateStore, c.stateMu))
	}
	if c.notifier != nil {
		opts = append(opts, runner.WithNotifier(c.notifier))
	}
	if c.cycleTracker != nil {
		opts = append(opts, runner.WithCycleTracker(c.cycleTracker))
	}
	if c.metrics != nil {
		opts = append(opts, runner.WithMetrics(c.metrics))
	}
	if c.alertStabilizationCycles > 0 {
		opts = append(opts, runner.WithAlertStabilizationCycles(c.alertStabilizationCycles))
	}
	if len(c.mappings) > 0 {
		opts = append(opts, runner.WithStacksEvaluated(len(c.mappings)))
	}

	r := runner.New(
		stackLogger,
		c.cfg.PollInterval,
		opts...,
	)

	c.mu.Lock()
	c.runners[mapping.Name] = r
	c.mu.Unlock()

	stackLogger.Info().Msg("runner started")

	// Run until context is canceled or error occurs
	if err := r.Run(ctx); err != nil {
		stackLogger.Error().Err(err).Msg("runner exited with error")
		c.recordError(mapping.Name, err)
	} else {
		stackLogger.Info().Msg("runner exited cleanly")
	}
}

// recordError records a per-stack error for later reporting.
func (c *Coordinator) recordError(stackName string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.runnerErrors[stackName] = err
}

// GetRunners returns a copy of the runners map for testing.
func (c *Coordinator) GetRunners() map[string]*runner.Runner {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]*runner.Runner, len(c.runners))
	for k, v := range c.runners {
		result[k] = v
	}
	return result
}
