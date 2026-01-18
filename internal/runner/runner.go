package runner

import (
	"context"
	"errors"
	"time"

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
	logger        zerolog.Logger
	pollInterval  time.Duration
	tickerFactory func(time.Duration) Ticker
	runOnce       func(context.Context) error
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

// New constructs a Runner with the given logger and poll interval.
func New(logger zerolog.Logger, pollInterval time.Duration, opts ...Option) *Runner {
	r := &Runner{
		logger:       logger,
		pollInterval: pollInterval,
		tickerFactory: func(d time.Duration) Ticker {
			return timeTicker{ticker: time.NewTicker(d)}
		},
		runOnce: func(context.Context) error {
			return nil
		},
	}

	for _, opt := range opts {
		opt(r)
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
