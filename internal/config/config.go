package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const (
	envPollInterval    = "SS_POLL_INTERVAL"
	envComposeURL      = "SS_COMPOSE_URL"
	envSlackWebhookURL = "SS_SLACK_WEBHOOK_URL"
	envDockerProxyURL  = "SS_DOCKER_PROXY_URL"
)

const (
	defaultPollInterval   = 30 * time.Second
	defaultDockerProxyURL = "http://localhost:2375"
)

// Config describes runtime configuration loaded from the environment.
type Config struct {
	PollInterval    time.Duration
	ComposeURL      string
	SlackWebhookURL string
	DockerProxyURL  string
}

// Load reads configuration from environment variables and a local .env file if present.
// Existing environment variables take precedence over values in .env.
func Load() (Config, error) {
	if err := loadDotEnvIfPresent(".env"); err != nil {
		return Config{}, err
	}

	cfg := Config{
		PollInterval:   defaultPollInterval,
		DockerProxyURL: defaultDockerProxyURL,
	}

	if value, ok := lookupTrimmed(envPollInterval); ok {
		interval, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("invalid %s: %w", envPollInterval, err)
		}
		if interval <= 0 {
			return Config{}, fmt.Errorf("%s must be greater than zero", envPollInterval)
		}
		cfg.PollInterval = interval
	}

	if value, ok := lookupTrimmed(envComposeURL); ok {
		cfg.ComposeURL = value
	}

	if value, ok := lookupTrimmed(envSlackWebhookURL); ok {
		cfg.SlackWebhookURL = value
	}

	if value, ok := lookupTrimmed(envDockerProxyURL); ok {
		cfg.DockerProxyURL = value
	}

	if cfg.ComposeURL == "" {
		return Config{}, errors.New("SS_COMPOSE_URL is required")
	}

	if err := validateURL(cfg.ComposeURL, "SS_COMPOSE_URL"); err != nil {
		return Config{}, err
	}

	if err := validateURL(cfg.DockerProxyURL, "SS_DOCKER_PROXY_URL"); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func lookupTrimmed(key string) (string, bool) {
	value, ok := os.LookupEnv(key)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(value), true
}

func loadDotEnvIfPresent(path string) error {
	err := godotenv.Load(path)
	if err == nil {
		return nil
	}

	var pathErr *os.PathError
	if errors.As(err, &pathErr) && errors.Is(pathErr.Err, os.ErrNotExist) {
		return nil
	}

	return err
}

func validateURL(value, name string) error {
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("invalid %s: %w", name, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("invalid %s: must include scheme and host", name)
	}
	return nil
}
