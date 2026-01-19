package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/nholik/swarm-sentinel/internal/compose"
	"github.com/nholik/swarm-sentinel/internal/config"
	"github.com/nholik/swarm-sentinel/internal/coordinator"
	"github.com/nholik/swarm-sentinel/internal/healthcheck"
	"github.com/nholik/swarm-sentinel/internal/logging"
	"github.com/nholik/swarm-sentinel/internal/metrics"
	"github.com/nholik/swarm-sentinel/internal/notify"
	"github.com/nholik/swarm-sentinel/internal/runner"
	"github.com/nholik/swarm-sentinel/internal/server"
	"github.com/nholik/swarm-sentinel/internal/state"
	"github.com/nholik/swarm-sentinel/internal/swarm"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		// Use default logger for config load errors since we don't have log level yet
		logger := logging.New()
		logger.Fatal().Err(err).Msg("failed to load config")
	}

	logger := logging.NewWithLevel(cfg.LogLevel)

	logger.Info().
		Str("compose_url", cfg.ComposeURL).
		Dur("compose_timeout", cfg.ComposeTimeout).
		Str("docker_proxy_url", cfg.DockerProxyURL).
		Dur("docker_api_timeout", cfg.DockerAPITimeout).
		Str("stack_name", cfg.StackName).
		Bool("docker_tls_enabled", cfg.DockerTLSEnabled).
		Dur("poll_interval", cfg.PollInterval).
		Int("alert_stabilization_cycles", cfg.AlertStabilizationCycles).
		Str("log_level", cfg.LogLevel).
		Str("state_path", cfg.StatePath).
		Str("slack_webhook", secretStatus(cfg.SlackWebhookURL)).
		Str("webhook_url", secretStatus(cfg.WebhookURL)).
		Int("health_port", cfg.HealthPort).
		Int("metrics_port", cfg.MetricsPort).
		Bool("dry_run", cfg.DryRun).
		Msg("config loaded")

	logger.Info().Msg("swarm-sentinel starting")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	swarmClient, err := swarm.NewDockerClient(cfg.DockerProxyURL, cfg.DockerAPITimeout, swarm.TLSConfig{
		Enabled:  cfg.DockerTLSEnabled,
		Verify:   cfg.DockerTLSVerify,
		CAFile:   cfg.DockerTLSCA,
		CertFile: cfg.DockerTLSCert,
		KeyFile:  cfg.DockerTLSKey,
	}, logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initialize docker client")
	}
	defer func() {
		if err := swarmClient.Close(); err != nil {
			logger.Warn().Err(err).Msg("error closing docker client")
		}
	}()

	if err := swarmClient.Ping(ctx); err != nil {
		logger.Fatal().Err(err).Msg("docker api unreachable")
	}

	stateStore := state.NewFileStore(cfg.StatePath, logger)
	stateMu := &sync.Mutex{}

	var tracker *healthcheck.Tracker
	if cfg.HealthPort != 0 {
		tracker = healthcheck.NewTracker()
	}

	var metricsCollector *metrics.Metrics
	if cfg.MetricsPort != 0 {
		metricsCollector = metrics.New()
	}

	server.Start(ctx, logger, cfg.PollInterval, tracker, metricsCollector, cfg.HealthPort, cfg.MetricsPort)

	slackNotifier := notify.NewSlackNotifier(logger, cfg.SlackWebhookURL)
	webhookNotifier, err := notify.NewWebhookNotifier(logger, cfg.WebhookURL, cfg.WebhookTemplate)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initialize webhook notifier")
	}

	var notifier notify.Notifier = notify.NewMultiNotifier(slackNotifier, webhookNotifier)
	if cfg.DryRun {
		notifier = notify.NewDryRunNotifier(logger, notifier)
	}

	// Detect mode: multi-stack or single-stack
	mappingPath, err := config.FindMappingFile()
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to find mapping file")
	}

	if mappingPath != "" {
		// Mode 2: Multi-stack via mapping file
		mappings, err := config.LoadMappingFile(mappingPath)
		if err != nil {
			logger.Fatal().Err(err).Msg("failed to load mapping file")
		}

		logger.Info().
			Int("stacks", len(mappings)).
			Str("mapping_file", mappingPath).
			Msg("multi-stack mode")

		coord := coordinator.New(
			logger,
			cfg,
			mappings,
			swarmClient,
			coordinator.WithStateStore(stateStore, stateMu),
			coordinator.WithNotifier(notifier),
			coordinator.WithAlertStabilizationCycles(cfg.AlertStabilizationCycles),
			coordinator.WithCycleTracker(tracker),
			coordinator.WithMetrics(metricsCollector),
		)
		if err := coord.Run(ctx); err != nil {
			logger.Fatal().Err(err).Msg("coordinator exited with error")
		}
	} else {
		// Mode 1: Single-stack (backward compatible)
		if cfg.ComposeURL == "" {
			logger.Fatal().Msg("SS_COMPOSE_URL is required in single-stack mode")
		}

		logger.Info().
			Str("compose_url", cfg.ComposeURL).
			Str("stack_name", cfg.StackName).
			Msg("single-stack mode")

		composeFetcher, err := compose.NewHTTPFetcher(cfg.ComposeURL, cfg.ComposeTimeout, 0)
		if err != nil {
			logger.Fatal().Err(err).Msg("failed to initialize compose fetcher")
		}

		r := runner.New(
			logger,
			cfg.PollInterval,
			runner.WithComposeFetcher(composeFetcher),
			runner.WithSwarmClient(swarmClient),
			runner.WithStackName(cfg.StackName),
			runner.WithStateStore(stateStore, stateMu),
			runner.WithNotifier(notifier),
			runner.WithAlertStabilizationCycles(cfg.AlertStabilizationCycles),
			runner.WithCycleTracker(tracker),
			runner.WithMetrics(metricsCollector),
		)
		if err := r.Run(ctx); err != nil {
			logger.Fatal().Err(err).Msg("runner exited with error")
		}
	}
}

func secretStatus(value string) string {
	if value == "" {
		return "unset"
	}
	return "set"
}
