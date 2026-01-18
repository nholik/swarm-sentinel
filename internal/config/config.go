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
	envPollInterval       = "SS_POLL_INTERVAL"
	envComposeURL         = "SS_COMPOSE_URL"
	envComposeTimeout     = "SS_COMPOSE_TIMEOUT"
	envComposeMappingFile = "SS_COMPOSE_MAPPING_FILE"
	envSlackWebhookURL    = "SS_SLACK_WEBHOOK_URL"
	envDockerProxyURL     = "SS_DOCKER_PROXY_URL"
	envDockerAPITimeout   = "SS_DOCKER_API_TIMEOUT"
	envStackName          = "SS_STACK_NAME"
	envDockerTLSCA        = "SS_DOCKER_TLS_CA"
	envDockerTLSCert      = "SS_DOCKER_TLS_CERT"
	envDockerTLSKey       = "SS_DOCKER_TLS_KEY"
	envDockerTLSVerify    = "SS_DOCKER_TLS_VERIFY"
	envLogLevel           = "SS_LOG_LEVEL"

	envDockerTLSVerifyCompat = "DOCKER_TLS_VERIFY"
	envDockerCertPathCompat  = "DOCKER_CERT_PATH"
)

const (
	defaultSwarmConfigPath = "/run/configs/compose-mapping.yaml"
	defaultSwarmSecretPath = "/run/secrets/compose-mapping.yaml"
	defaultLocalPath       = "./compose-mapping.yaml"
)

const (
	defaultPollInterval     = 30 * time.Second
	defaultComposeTimeout   = 10 * time.Second
	defaultDockerAPITimeout = 30 * time.Second
	defaultDockerProxyURL   = "http://localhost:2375"
	defaultLogLevel         = "info"
)

// Config describes runtime configuration loaded from the environment.
type Config struct {
	PollInterval     time.Duration
	ComposeTimeout   time.Duration
	DockerAPITimeout time.Duration
	ComposeURL       string
	SlackWebhookURL  string
	DockerProxyURL   string
	StackName        string
	DockerTLSEnabled bool
	DockerTLSVerify  bool
	DockerTLSCA      string
	DockerTLSCert    string
	DockerTLSKey     string
	LogLevel         string
}

// Load reads configuration from environment variables and a local .env file if present.
// Existing environment variables take precedence over values in .env.
func Load() (Config, error) {
	if err := loadDotEnvIfPresent(".env"); err != nil {
		return Config{}, err
	}

	cfg := Config{
		PollInterval:     defaultPollInterval,
		ComposeTimeout:   defaultComposeTimeout,
		DockerAPITimeout: defaultDockerAPITimeout,
		DockerProxyURL:   defaultDockerProxyURL,
		LogLevel:         defaultLogLevel,
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

	if value, ok := lookupTrimmed(envDockerAPITimeout); ok {
		timeout, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("invalid %s: %w", envDockerAPITimeout, err)
		}
		if timeout <= 0 {
			return Config{}, fmt.Errorf("%s must be greater than zero", envDockerAPITimeout)
		}
		cfg.DockerAPITimeout = timeout
	}

	if value, ok := lookupTrimmed(envLogLevel); ok {
		cfg.LogLevel = value
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

	// DockerTLSEnabled is auto-detected: true if TLS verification is requested,
	// or if any TLS certificate paths are provided. This allows users to enable
	// TLS by simply setting certificate paths without an explicit flag.
	cfg.DockerTLSEnabled = cfg.DockerTLSVerify ||
		cfg.DockerTLSCA != "" ||
		cfg.DockerTLSCert != "" ||
		cfg.DockerTLSKey != ""

	mappingPath, err := FindMappingFile()
	if err != nil {
		return Config{}, err
	}

	if mappingPath != "" && cfg.ComposeURL != "" {
		return Config{}, fmt.Errorf("SS_COMPOSE_URL and compose mapping file are mutually exclusive: %s", mappingPath)
	}
	if mappingPath == "" && cfg.ComposeURL == "" {
		return Config{}, errors.New("SS_COMPOSE_URL is required when no compose mapping file is present")
	}

	if cfg.ComposeURL != "" {
		if err := validateHTTPURL(cfg.ComposeURL, "SS_COMPOSE_URL"); err != nil {
			return Config{}, err
		}
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

func validateHTTPURL(value, name string) error {
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("invalid %s: %w", name, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("invalid %s: must include scheme and host", name)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("invalid %s: must be http or https URL", name)
	}
	return nil
}

// FindMappingFile attempts to locate a compose mapping file in order of precedence:
// 1. Explicit SS_COMPOSE_MAPPING_FILE env var (error if specified but not found)
// 2. /run/configs/compose-mapping.yaml (Swarm config mount)
// 3. /run/secrets/compose-mapping.yaml (Swarm secret mount)
// 4. ./compose-mapping.yaml (local development)
// Returns empty string if no mapping file is found (single-stack mode).
func FindMappingFile() (string, error) {
	// 1. Explicit env var
	if path, ok := lookupTrimmed(envComposeMappingFile); ok {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
		return "", fmt.Errorf("SS_COMPOSE_MAPPING_FILE specified but not found: %s", path)
	}

	// 2. Swarm config mount (standard)
	if _, err := os.Stat(defaultSwarmConfigPath); err == nil {
		return defaultSwarmConfigPath, nil
	}

	// 3. Swarm secret mount (alternative)
	if _, err := os.Stat(defaultSwarmSecretPath); err == nil {
		return defaultSwarmSecretPath, nil
	}

	// 4. Local development
	if _, err := os.Stat(defaultLocalPath); err == nil {
		return defaultLocalPath, nil
	}

	// No mapping file found (single-stack mode)
	return "", nil
}
