package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const (
	envPollInterval    = "SS_POLL_INTERVAL"
	envComposeURL      = "SS_COMPOSE_URL"
	envComposeTimeout  = "SS_COMPOSE_TIMEOUT"
	envSlackWebhookURL = "SS_SLACK_WEBHOOK_URL"
	envDockerProxyURL  = "SS_DOCKER_PROXY_URL"
	envStackName       = "SS_STACK_NAME"
	envDockerTLSCA     = "SS_DOCKER_TLS_CA"
	envDockerTLSCert   = "SS_DOCKER_TLS_CERT"
	envDockerTLSKey    = "SS_DOCKER_TLS_KEY"
	envDockerTLSVerify = "SS_DOCKER_TLS_VERIFY"

	envDockerTLSVerifyCompat = "DOCKER_TLS_VERIFY"
	envDockerCertPathCompat  = "DOCKER_CERT_PATH"
)

const (
	defaultPollInterval   = 30 * time.Second
	defaultComposeTimeout = 10 * time.Second
	defaultDockerProxyURL = "http://localhost:2375"
)

// Config describes runtime configuration loaded from the environment.
type Config struct {
	PollInterval     time.Duration
	ComposeTimeout   time.Duration
	ComposeURL       string
	SlackWebhookURL  string
	DockerProxyURL   string
	StackName        string
	DockerTLSEnabled bool
	DockerTLSVerify  bool
	DockerTLSCA      string
	DockerTLSCert    string
	DockerTLSKey     string
}

// Load reads configuration from environment variables and a local .env file if present.
// Existing environment variables take precedence over values in .env.
func Load() (Config, error) {
	if err := loadDotEnvIfPresent(".env"); err != nil {
		return Config{}, err
	}

	cfg := Config{
		PollInterval:   defaultPollInterval,
		ComposeTimeout: defaultComposeTimeout,
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

	if value, ok := lookupTrimmed(envComposeTimeout); ok {
		timeout, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("invalid %s: %w", envComposeTimeout, err)
		}
		if timeout <= 0 {
			return Config{}, fmt.Errorf("%s must be greater than zero", envComposeTimeout)
		}
		cfg.ComposeTimeout = timeout
	}

	if value, ok := lookupTrimmed(envSlackWebhookURL); ok {
		cfg.SlackWebhookURL = value
	}

	if value, ok := lookupTrimmed(envDockerProxyURL); ok {
		cfg.DockerProxyURL = value
	}

	if value, ok := lookupTrimmed(envStackName); ok {
		cfg.StackName = value
	}

	tlsVerify, tlsVerifySet, err := lookupBool(envDockerTLSVerify)
	if err != nil {
		return Config{}, err
	}
	if !tlsVerifySet {
		tlsVerify, _, err = lookupBool(envDockerTLSVerifyCompat)
		if err != nil {
			return Config{}, err
		}
	}
	cfg.DockerTLSVerify = tlsVerify

	if value, ok := lookupTrimmed(envDockerTLSCA); ok {
		cfg.DockerTLSCA = value
	}
	if value, ok := lookupTrimmed(envDockerTLSCert); ok {
		cfg.DockerTLSCert = value
	}
	if value, ok := lookupTrimmed(envDockerTLSKey); ok {
		cfg.DockerTLSKey = value
	}

	if dockerCertPath, ok := lookupTrimmed(envDockerCertPathCompat); ok {
		if cfg.DockerTLSCA == "" {
			cfg.DockerTLSCA = filepath.Join(dockerCertPath, "ca.pem")
		}
		if cfg.DockerTLSCert == "" {
			cfg.DockerTLSCert = filepath.Join(dockerCertPath, "cert.pem")
		}
		if cfg.DockerTLSKey == "" {
			cfg.DockerTLSKey = filepath.Join(dockerCertPath, "key.pem")
		}
	}

	cfg.DockerTLSEnabled = cfg.DockerTLSVerify ||
		cfg.DockerTLSCA != "" ||
		cfg.DockerTLSCert != "" ||
		cfg.DockerTLSKey != ""

	if cfg.ComposeURL == "" {
		return Config{}, errors.New("SS_COMPOSE_URL is required")
	}

	if err := validateURL(cfg.ComposeURL, "SS_COMPOSE_URL"); err != nil {
		return Config{}, err
	}

	if err := validateURL(cfg.DockerProxyURL, "SS_DOCKER_PROXY_URL"); err != nil {
		return Config{}, err
	}

	if cfg.DockerTLSEnabled {
		missing := []string{}
		if cfg.DockerTLSCert == "" {
			missing = append(missing, envDockerTLSCert)
		}
		if cfg.DockerTLSKey == "" {
			missing = append(missing, envDockerTLSKey)
		}
		if cfg.DockerTLSVerify && cfg.DockerTLSCA == "" {
			missing = append(missing, envDockerTLSCA)
		}
		if len(missing) > 0 {
			return Config{}, fmt.Errorf("docker tls enabled but missing %s", strings.Join(missing, ", "))
		}
	}

	dockerURL, _ := url.Parse(cfg.DockerProxyURL)
	if dockerURL != nil && dockerURL.Scheme == "https" && !cfg.DockerTLSEnabled {
		return Config{}, errors.New("https docker host requires TLS configuration")
	}

	if cfg.SlackWebhookURL != "" {
		if err := validateURL(cfg.SlackWebhookURL, "SS_SLACK_WEBHOOK_URL"); err != nil {
			return Config{}, err
		}
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

func lookupBool(key string) (bool, bool, error) {
	value, ok := lookupTrimmed(key)
	if !ok {
		return false, false, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, true, fmt.Errorf("invalid %s: %w", key, err)
	}
	return parsed, true, nil
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
