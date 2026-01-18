package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/nholik/swarm-sentinel/internal/compose"
	"github.com/nholik/swarm-sentinel/internal/config"
	"github.com/nholik/swarm-sentinel/internal/logging"
	"github.com/nholik/swarm-sentinel/internal/runner"
	"github.com/nholik/swarm-sentinel/internal/swarm"
)

func main() {
	logger := logging.New()
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to load config")
	}

	logger.Info().
		Str("compose_url", cfg.ComposeURL).
		Dur("compose_timeout", cfg.ComposeTimeout).
		Str("docker_proxy_url", cfg.DockerProxyURL).
		Str("stack_name", cfg.StackName).
		Bool("docker_tls_enabled", cfg.DockerTLSEnabled).
		Dur("poll_interval", cfg.PollInterval).
		Str("slack_webhook", secretStatus(cfg.SlackWebhookURL)).
		Msg("config loaded")

	logger.Info().Msg("swarm-sentinel starting")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	swarmClient, err := swarm.NewDockerClient(cfg.DockerProxyURL, 0, swarm.TLSConfig{
		Enabled:  cfg.DockerTLSEnabled,
		Verify:   cfg.DockerTLSVerify,
		CAFile:   cfg.DockerTLSCA,
		CertFile: cfg.DockerTLSCert,
		KeyFile:  cfg.DockerTLSKey,
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initialize docker client")
	}
	if err := swarmClient.Ping(ctx); err != nil {
		logger.Fatal().Err(err).Msg("docker api unreachable")
	}

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
	)
	if err := r.Run(ctx); err != nil {
		logger.Fatal().Err(err).Msg("runner exited with error")
	}
}

func secretStatus(value string) string {
	if value == "" {
		return "unset"
	}
	return "set"
}
