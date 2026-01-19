package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/nholik/swarm-sentinel/internal/healthcheck"
	"github.com/nholik/swarm-sentinel/internal/metrics"
	"github.com/rs/zerolog"
)

const shutdownTimeout = 5 * time.Second

// Start launches health and metrics HTTP servers as configured.
func Start(ctx context.Context, logger zerolog.Logger, pollInterval time.Duration, tracker *healthcheck.Tracker, metricsCollector *metrics.Metrics, healthPort, metricsPort int) {
	if healthPort == 0 && metricsPort == 0 {
		return
	}

	if healthPort > 0 && metricsPort > 0 && healthPort == metricsPort {
		mux := http.NewServeMux()
		registerHealthRoutes(mux, tracker, pollInterval)
		registerMetricsRoute(mux, metricsCollector)
		startServer(ctx, logger, mux, healthPort, "health/metrics")
		return
	}

	if healthPort > 0 {
		mux := http.NewServeMux()
		registerHealthRoutes(mux, tracker, pollInterval)
		startServer(ctx, logger, mux, healthPort, "health")
	}

	if metricsPort > 0 {
		mux := http.NewServeMux()
		registerMetricsRoute(mux, metricsCollector)
		startServer(ctx, logger, mux, metricsPort, "metrics")
	}
}

func registerHealthRoutes(mux *http.ServeMux, tracker *healthcheck.Tracker, pollInterval time.Duration) {
	mux.HandleFunc("/healthz", healthcheck.HealthHandler(tracker, pollInterval))
	mux.HandleFunc("/readyz", healthcheck.ReadyHandler(tracker))
}

func registerMetricsRoute(mux *http.ServeMux, metricsCollector *metrics.Metrics) {
	if metricsCollector == nil {
		return
	}
	mux.Handle("/metrics", metricsCollector.Handler())
}

func startServer(ctx context.Context, logger zerolog.Logger, handler http.Handler, port int, label string) {
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info().Str("server", label).Int("port", port).Msg("http server starting")
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error().Err(err).Str("server", label).Int("port", port).Msg("http server failed")
		}
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error().Err(err).Str("server", label).Int("port", port).Msg("http server shutdown failed")
		}
	}()
}
