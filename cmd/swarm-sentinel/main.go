package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/nholik/swarm-sentinel/internal/config"
	"github.com/nholik/swarm-sentinel/internal/logging"
	"github.com/nholik/swarm-sentinel/internal/runner"
)

func main() {
	logger := logging.New()
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to load config")
	}

	logger.Info().
		Str("compose_url", cfg.ComposeURL).
		Str("docker_proxy_url", cfg.DockerProxyURL).
		Dur("poll_interval", cfg.PollInterval).
		Str("slack_webhook", secretStatus(cfg.SlackWebhookURL)).
		Msg("config loaded")

	logger.Info().Msg("swarm-sentinel starting")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	r := runner.New(logger, cfg.PollInterval)
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
