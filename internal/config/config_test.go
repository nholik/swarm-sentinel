package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad_ValidationAndDefaults(t *testing.T) {
	cases := []struct {
		name        string
		env         map[string]string
		mappingFile string
		wantErr     bool
		want        Config
	}{
		{
			name:    "missing compose url",
			env:     map[string]string{},
			wantErr: true,
		},
		{
			name: "defaults applied",
			env: map[string]string{
				envComposeURL: "https://example.com/compose.yml",
			},
			want: Config{
				PollInterval:             defaultPollInterval,
				ComposeTimeout:           defaultComposeTimeout,
				DockerAPITimeout:         defaultDockerAPITimeout,
				ComposeURL:               "https://example.com/compose.yml",
				DockerProxyURL:           defaultDockerProxyURL,
				StackName:                "",
				DockerTLSEnabled:         false,
				DockerTLSVerify:          false,
				LogLevel:                 defaultLogLevel,
				StatePath:                defaultStatePath,
				AlertStabilizationCycles: defaultAlertStabilizationCycles,
				HealthPort:               defaultHealthPort,
				MetricsPort:              defaultMetricsPort,
				DryRun:                   false,
			},
		},
		{
			name:        "mapping file without compose url",
			env:         map[string]string{},
			mappingFile: "stacks:\n  - name: prod\n    compose_url: https://example.com/compose.yml\n",
			want: Config{
				PollInterval:             defaultPollInterval,
				ComposeTimeout:           defaultComposeTimeout,
				DockerAPITimeout:         defaultDockerAPITimeout,
				DockerProxyURL:           defaultDockerProxyURL,
				StackName:                "",
				DockerTLSEnabled:         false,
				DockerTLSVerify:          false,
				LogLevel:                 defaultLogLevel,
				StatePath:                defaultStatePath,
				AlertStabilizationCycles: defaultAlertStabilizationCycles,
				HealthPort:               defaultHealthPort,
				MetricsPort:              defaultMetricsPort,
				DryRun:                   false,
			},
		},
		{
			name:        "compose url and mapping file",
			env:         map[string]string{envComposeURL: "https://example.com/compose.yml"},
			mappingFile: "stacks:\n  - name: prod\n    compose_url: https://example.com/compose.yml\n",
			wantErr:     true,
		},
		{
			name: "invalid poll interval",
			env: map[string]string{
				envComposeURL:   "https://example.com/compose.yml",
				envPollInterval: "nope",
			},
			wantErr: true,
		},
		{
			name: "invalid compose timeout",
			env: map[string]string{
				envComposeURL:     "https://example.com/compose.yml",
				envComposeTimeout: "nope",
			},
			wantErr: true,
		},
		{
			name: "zero compose timeout",
			env: map[string]string{
				envComposeURL:     "https://example.com/compose.yml",
				envComposeTimeout: "0s",
			},
			wantErr: true,
		},
		{
			name: "zero poll interval",
			env: map[string]string{
				envComposeURL:   "https://example.com/compose.yml",
				envPollInterval: "0s",
			},
			wantErr: true,
		},
		{
			name: "negative poll interval",
			env: map[string]string{
				envComposeURL:   "https://example.com/compose.yml",
				envPollInterval: "-5s",
			},
			wantErr: true,
		},
		{
			name: "invalid compose url missing scheme",
			env: map[string]string{
				envComposeURL: "example.com/compose.yml",
			},
			wantErr: true,
		},
		{
			name: "invalid compose url scheme",
			env: map[string]string{
				envComposeURL: "ftp://example.com/compose.yml",
			},
			wantErr: true,
		},
		{
			name: "invalid docker proxy url",
			env: map[string]string{
				envComposeURL:     "https://example.com/compose.yml",
				envDockerProxyURL: "not-a-url",
			},
			wantErr: true,
		},
		{
			name: "invalid slack webhook url",
			env: map[string]string{
				envComposeURL:      "https://example.com/compose.yml",
				envSlackWebhookURL: "not-a-url",
			},
			wantErr: true,
		},
		{
			name: "valid slack webhook url",
			env: map[string]string{
				envComposeURL:      "https://example.com/compose.yml",
				envSlackWebhookURL: "https://hooks.slack.com/services/T00/B00/XXX",
			},
			want: Config{
				PollInterval:             defaultPollInterval,
				ComposeTimeout:           defaultComposeTimeout,
				DockerAPITimeout:         defaultDockerAPITimeout,
				ComposeURL:               "https://example.com/compose.yml",
				DockerProxyURL:           defaultDockerProxyURL,
				StackName:                "",
				DockerTLSEnabled:         false,
				DockerTLSVerify:          false,
				SlackWebhookURL:          "https://hooks.slack.com/services/T00/B00/XXX",
				LogLevel:                 defaultLogLevel,
				StatePath:                defaultStatePath,
				AlertStabilizationCycles: defaultAlertStabilizationCycles,
				HealthPort:               defaultHealthPort,
				MetricsPort:              defaultMetricsPort,
				DryRun:                   false,
			},
		},
		{
			name: "custom docker proxy and poll interval",
			env: map[string]string{
				envComposeURL:     "https://example.com/compose.yml",
				envPollInterval:   "45s",
				envDockerProxyURL: "http://proxy:2375",
			},
			want: Config{
				PollInterval:             45 * time.Second,
				ComposeTimeout:           defaultComposeTimeout,
				DockerAPITimeout:         defaultDockerAPITimeout,
				ComposeURL:               "https://example.com/compose.yml",
				DockerProxyURL:           "http://proxy:2375",
				StackName:                "",
				DockerTLSEnabled:         false,
				DockerTLSVerify:          false,
				LogLevel:                 defaultLogLevel,
				StatePath:                defaultStatePath,
				AlertStabilizationCycles: defaultAlertStabilizationCycles,
				HealthPort:               defaultHealthPort,
				MetricsPort:              defaultMetricsPort,
				DryRun:                   false,
			},
		},
		{
			name: "stack name set",
			env: map[string]string{
				envComposeURL: "https://example.com/compose.yml",
				envStackName:  "prod",
			},
			want: Config{
				PollInterval:             defaultPollInterval,
				ComposeTimeout:           defaultComposeTimeout,
				DockerAPITimeout:         defaultDockerAPITimeout,
				ComposeURL:               "https://example.com/compose.yml",
				DockerProxyURL:           defaultDockerProxyURL,
				StackName:                "prod",
				DockerTLSEnabled:         false,
				DockerTLSVerify:          false,
				LogLevel:                 defaultLogLevel,
				StatePath:                defaultStatePath,
				AlertStabilizationCycles: defaultAlertStabilizationCycles,
				HealthPort:               defaultHealthPort,
				MetricsPort:              defaultMetricsPort,
				DryRun:                   false,
			},
		},
		{
			name: "tls verify missing certs",
			env: map[string]string{
				envComposeURL:      "https://example.com/compose.yml",
				envDockerTLSVerify: "true",
			},
			wantErr: true,
		},
		{
			name: "tls verify with certs",
			env: map[string]string{
				envComposeURL:      "https://example.com/compose.yml",
				envDockerTLSVerify: "true",
				envDockerTLSCA:     "/tmp/ca.pem",
				envDockerTLSCert:   "/tmp/cert.pem",
				envDockerTLSKey:    "/tmp/key.pem",
			},
			want: Config{
				PollInterval:             defaultPollInterval,
				ComposeTimeout:           defaultComposeTimeout,
				DockerAPITimeout:         defaultDockerAPITimeout,
				ComposeURL:               "https://example.com/compose.yml",
				DockerProxyURL:           defaultDockerProxyURL,
				StackName:                "",
				DockerTLSEnabled:         true,
				DockerTLSVerify:          true,
				DockerTLSCA:              "/tmp/ca.pem",
				DockerTLSCert:            "/tmp/cert.pem",
				DockerTLSKey:             "/tmp/key.pem",
				LogLevel:                 defaultLogLevel,
				StatePath:                defaultStatePath,
				AlertStabilizationCycles: defaultAlertStabilizationCycles,
				HealthPort:               defaultHealthPort,
				MetricsPort:              defaultMetricsPort,
				DryRun:                   false,
			},
		},
		{
			name: "tls verify with docker cert path",
			env: map[string]string{
				envComposeURL:            "https://example.com/compose.yml",
				envDockerTLSVerifyCompat: "1",
				envDockerCertPathCompat:  "/tmp/certs",
			},
			want: Config{
				PollInterval:             defaultPollInterval,
				ComposeTimeout:           defaultComposeTimeout,
				DockerAPITimeout:         defaultDockerAPITimeout,
				ComposeURL:               "https://example.com/compose.yml",
				DockerProxyURL:           defaultDockerProxyURL,
				StackName:                "",
				DockerTLSEnabled:         true,
				DockerTLSVerify:          true,
				DockerTLSCA:              "/tmp/certs/ca.pem",
				DockerTLSCert:            "/tmp/certs/cert.pem",
				DockerTLSKey:             "/tmp/certs/key.pem",
				LogLevel:                 defaultLogLevel,
				StatePath:                defaultStatePath,
				AlertStabilizationCycles: defaultAlertStabilizationCycles,
				HealthPort:               defaultHealthPort,
				MetricsPort:              defaultMetricsPort,
				DryRun:                   false,
			},
		},
		{
			name: "tls enabled without verify",
			env: map[string]string{
				envComposeURL:    "https://example.com/compose.yml",
				envDockerTLSCert: "/tmp/cert.pem",
				envDockerTLSKey:  "/tmp/key.pem",
			},
			want: Config{
				PollInterval:             defaultPollInterval,
				ComposeTimeout:           defaultComposeTimeout,
				DockerAPITimeout:         defaultDockerAPITimeout,
				ComposeURL:               "https://example.com/compose.yml",
				DockerProxyURL:           defaultDockerProxyURL,
				StackName:                "",
				DockerTLSEnabled:         true,
				DockerTLSVerify:          false,
				DockerTLSCert:            "/tmp/cert.pem",
				DockerTLSKey:             "/tmp/key.pem",
				LogLevel:                 defaultLogLevel,
				StatePath:                defaultStatePath,
				AlertStabilizationCycles: defaultAlertStabilizationCycles,
				HealthPort:               defaultHealthPort,
				MetricsPort:              defaultMetricsPort,
				DryRun:                   false,
			},
		},
		{
			name: "custom docker api timeout",
			env: map[string]string{
				envComposeURL:       "https://example.com/compose.yml",
				envDockerAPITimeout: "60s",
			},
			want: Config{
				PollInterval:             defaultPollInterval,
				ComposeTimeout:           defaultComposeTimeout,
				DockerAPITimeout:         60 * time.Second,
				ComposeURL:               "https://example.com/compose.yml",
				DockerProxyURL:           defaultDockerProxyURL,
				StackName:                "",
				DockerTLSEnabled:         false,
				DockerTLSVerify:          false,
				LogLevel:                 defaultLogLevel,
				StatePath:                defaultStatePath,
				AlertStabilizationCycles: defaultAlertStabilizationCycles,
				HealthPort:               defaultHealthPort,
				MetricsPort:              defaultMetricsPort,
				DryRun:                   false,
			},
		},
		{
			name: "invalid docker api timeout",
			env: map[string]string{
				envComposeURL:       "https://example.com/compose.yml",
				envDockerAPITimeout: "nope",
			},
			wantErr: true,
		},
		{
			name: "zero docker api timeout",
			env: map[string]string{
				envComposeURL:       "https://example.com/compose.yml",
				envDockerAPITimeout: "0s",
			},
			wantErr: true,
		},
		{
			name: "custom log level",
			env: map[string]string{
				envComposeURL: "https://example.com/compose.yml",
				envLogLevel:   "debug",
			},
			want: Config{
				PollInterval:             defaultPollInterval,
				ComposeTimeout:           defaultComposeTimeout,
				DockerAPITimeout:         defaultDockerAPITimeout,
				ComposeURL:               "https://example.com/compose.yml",
				DockerProxyURL:           defaultDockerProxyURL,
				StackName:                "",
				DockerTLSEnabled:         false,
				DockerTLSVerify:          false,
				LogLevel:                 "debug",
				StatePath:                defaultStatePath,
				AlertStabilizationCycles: defaultAlertStabilizationCycles,
				HealthPort:               defaultHealthPort,
				MetricsPort:              defaultMetricsPort,
				DryRun:                   false,
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			restoreDir := mustChdir(t, tmpDir)
			defer restoreDir()

			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			if tc.mappingFile != "" {
				mappingPath := filepath.Join(tmpDir, "compose-mapping.yaml")
				if err := os.WriteFile(mappingPath, []byte(tc.mappingFile), 0o600); err != nil {
					t.Fatalf("write mapping file: %v", err)
				}
				t.Setenv(envComposeMappingFile, mappingPath)
			}

			got, err := Load()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tc.want {
				t.Fatalf("unexpected config: %+v", got)
			}
		})
	}
}

func TestLoad_DotEnvAndEnvOverride(t *testing.T) {
	tmpDir := t.TempDir()
	restoreDir := mustChdir(t, tmpDir)
	defer restoreDir()

	dotenv := []byte(`
# example .env
SS_COMPOSE_URL=https://example.com/from-dotenv.yml
SS_SLACK_WEBHOOK_URL=https://hooks.slack.com/services/test
SS_DOCKER_PROXY_URL=http://dotenv:2375
`)

	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), dotenv, 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	t.Setenv(envComposeURL, "https://example.com/from-env.yml")
	t.Setenv(envDockerProxyURL, "http://env:2375")

	got, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.ComposeURL != "https://example.com/from-env.yml" {
		t.Fatalf("compose url did not prefer env: %s", got.ComposeURL)
	}
	if got.DockerProxyURL != "http://env:2375" {
		t.Fatalf("docker proxy url did not prefer env: %s", got.DockerProxyURL)
	}
	if got.SlackWebhookURL != "https://hooks.slack.com/services/test" {
		t.Fatalf("slack webhook url not loaded from .env: %s", got.SlackWebhookURL)
	}
	if got.PollInterval != defaultPollInterval {
		t.Fatalf("unexpected poll interval: %s", got.PollInterval)
	}
	if got.ComposeTimeout != defaultComposeTimeout {
		t.Fatalf("unexpected compose timeout: %s", got.ComposeTimeout)
	}
}

func mustChdir(t *testing.T, dir string) func() {
	t.Helper()
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	return func() {
		if err := os.Chdir(original); err != nil {
			t.Fatalf("restore dir: %v", err)
		}
	}
}

func TestLoad_SSRFProtection(t *testing.T) {
	cases := []struct {
		name       string
		composeURL string
		wantErr    bool
		errContain string
	}{
		{
			name:       "AWS metadata IPv4",
			composeURL: "http://169.254.169.254/latest/meta-data/",
			wantErr:    true,
			errContain: "cloud metadata endpoint",
		},
		{
			name:       "AWS metadata with path",
			composeURL: "http://169.254.169.254/latest/user-data",
			wantErr:    true,
			errContain: "cloud metadata endpoint",
		},
		{
			name:       "Google metadata",
			composeURL: "http://metadata.google.internal/computeMetadata/v1/",
			wantErr:    true,
			errContain: "cloud metadata endpoint",
		},
		{
			name:       "Google metadata alternate",
			composeURL: "http://metadata.goog/computeMetadata/v1/",
			wantErr:    true,
			errContain: "cloud metadata endpoint",
		},
		{
			name:       "Azure metadata",
			composeURL: "http://metadata.azure.com/metadata/instance",
			wantErr:    true,
			errContain: "cloud metadata endpoint",
		},
		{
			name:       "AWS ECS task metadata",
			composeURL: "http://169.254.170.2/v4/credentials",
			wantErr:    true,
			errContain: "metadata endpoint",
		},
		{
			name:       "link-local other",
			composeURL: "http://169.254.1.1/some/path",
			wantErr:    true,
			errContain: "link-local address",
		},
		{
			name:       "valid external URL",
			composeURL: "https://artifacts.example.com/compose.yml",
			wantErr:    false,
		},
		{
			name:       "valid internal URL",
			composeURL: "http://config-server:8080/compose.yml",
			wantErr:    false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			restoreDir := mustChdir(t, tmpDir)
			defer restoreDir()

			t.Setenv(envComposeURL, tc.composeURL)

			_, err := Load()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %s", tc.composeURL)
				}
				if tc.errContain != "" && !contains(err.Error(), tc.errContain) {
					t.Fatalf("expected error containing %q, got %v", tc.errContain, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestLoadSecretFromFile(t *testing.T) {
	t.Run("prefers _FILE over direct env var", func(t *testing.T) {
		tmpDir := t.TempDir()
		restoreDir := mustChdir(t, tmpDir)
		defer restoreDir()

		secretFile := filepath.Join(tmpDir, "slack-secret")
		if err := os.WriteFile(secretFile, []byte("https://hooks.slack.com/from-file\n"), 0o600); err != nil {
			t.Fatalf("write secret file: %v", err)
		}

		t.Setenv(envComposeURL, "https://example.com/compose.yml")
		t.Setenv(envSlackWebhookURL, "https://hooks.slack.com/from-env")
		t.Setenv(envSlackWebhookURL+"_FILE", secretFile)

		got, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got.SlackWebhookURL != "https://hooks.slack.com/from-file" {
			t.Fatalf("expected slack webhook from file, got: %s", got.SlackWebhookURL)
		}
	})

	t.Run("falls back to direct env var when file missing", func(t *testing.T) {
		tmpDir := t.TempDir()
		restoreDir := mustChdir(t, tmpDir)
		defer restoreDir()

		t.Setenv(envComposeURL, "https://example.com/compose.yml")
		t.Setenv(envSlackWebhookURL, "https://hooks.slack.com/from-env")
		t.Setenv(envSlackWebhookURL+"_FILE", "/nonexistent/path/secret")

		got, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got.SlackWebhookURL != "https://hooks.slack.com/from-env" {
			t.Fatalf("expected slack webhook from env fallback, got: %s", got.SlackWebhookURL)
		}
	})

	t.Run("trims whitespace from file content", func(t *testing.T) {
		tmpDir := t.TempDir()
		restoreDir := mustChdir(t, tmpDir)
		defer restoreDir()

		secretFile := filepath.Join(tmpDir, "webhook-secret")
		if err := os.WriteFile(secretFile, []byte("  https://webhook.example.com/hook  \n\n"), 0o600); err != nil {
			t.Fatalf("write secret file: %v", err)
		}

		t.Setenv(envComposeURL, "https://example.com/compose.yml")
		t.Setenv(envWebhookURL+"_FILE", secretFile)

		got, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got.WebhookURL != "https://webhook.example.com/hook" {
			t.Fatalf("expected trimmed webhook url, got: %q", got.WebhookURL)
		}
	})

	t.Run("uses direct env var when no _FILE set", func(t *testing.T) {
		tmpDir := t.TempDir()
		restoreDir := mustChdir(t, tmpDir)
		defer restoreDir()

		t.Setenv(envComposeURL, "https://example.com/compose.yml")
		t.Setenv(envWebhookURL, "https://webhook.example.com/direct")

		got, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got.WebhookURL != "https://webhook.example.com/direct" {
			t.Fatalf("expected webhook url from env, got: %s", got.WebhookURL)
		}
	})
}
