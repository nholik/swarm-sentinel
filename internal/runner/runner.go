package runner

import (
	"context"
	"errors"
	"time"

	"github.com/nholik/swarm-sentinel/internal/compose"
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
	composeETag      string
	composeHash      string
	lastDesiredState *compose.DesiredState
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
	if r.composeFetcher == nil {
		return nil
	}

	result, err := r.composeFetcher.Fetch(ctx, r.composeETag)
	if err != nil {
		return err
	}

	if result.ETag != "" {
		r.composeETag = result.ETag
	}

	if result.NotModified {
		r.logger.Debug().Msg("compose unchanged")
		return nil
	}

	fingerprint, err := compose.Fingerprint(result.Body)
	if err != nil {
		return err
	}
	if fingerprint == r.composeHash {
		r.logger.Debug().Msg("compose fingerprint unchanged")
		return nil
	}
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

	return nil
}
