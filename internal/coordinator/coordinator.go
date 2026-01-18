package coordinator

import (
	"context"
	"sync"

	"github.com/nholik/swarm-sentinel/internal/compose"
	"github.com/nholik/swarm-sentinel/internal/config"
	"github.com/nholik/swarm-sentinel/internal/runner"
	"github.com/nholik/swarm-sentinel/internal/swarm"
	"github.com/rs/zerolog"
)

// Coordinator manages multiple Runner instances, one per stack.
// It spawns runners in parallel and waits for context cancellation.
type Coordinator struct {
	logger       zerolog.Logger
	cfg          config.Config
	mappings     []config.StackMapping
	swarmClient  swarm.Client
	runners      map[string]*runner.Runner
	runnerErrors map[string]error
	mu           sync.RWMutex
}

// New constructs a Coordinator with the given configuration and stack mappings.
func New(logger zerolog.Logger, cfg config.Config, mappings []config.StackMapping, swarmClient swarm.Client) *Coordinator {
	return &Coordinator{
		logger:       logger,
		cfg:          cfg,
		mappings:     mappings,
		swarmClient:  swarmClient,
		runners:      make(map[string]*runner.Runner),
		runnerErrors: make(map[string]error),
	}
}

// Run starts all runners in parallel and blocks until context is canceled.
// Returns nil on clean shutdown; logs any per-runner errors internally.
func (c *Coordinator) Run(ctx context.Context) error {
	c.logger.Info().
		Int("stacks", len(c.mappings)).
		Msg("starting coordinator")

	// Spawn all runners in parallel
	var wg sync.WaitGroup
	for _, mapping := range c.mappings {
		wg.Add(1)
		go c.spawnRunner(ctx, &wg, mapping)
	}

	// Wait for all runners to exit (via context cancellation or error)
	wg.Wait()
	c.logger.Info().Msg("all runners stopped")

	// Report any errors
	c.mu.RLock()
	defer c.mu.RUnlock()
	for stack, err := range c.runnerErrors {
		if err != nil {
			c.logger.Error().Err(err).Str("stack", stack).Msg("runner error")
		}
	}

	return nil
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
	r := runner.New(
		stackLogger,
		c.cfg.PollInterval,
		runner.WithComposeFetcher(fetcher),
		runner.WithSwarmClient(c.swarmClient),
		runner.WithStackName(mapping.Name),
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
