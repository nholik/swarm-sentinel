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
				PollInterval:     defaultPollInterval,
				ComposeTimeout:   defaultComposeTimeout,
				DockerAPITimeout: defaultDockerAPITimeout,
				ComposeURL:       "https://example.com/compose.yml",
				DockerProxyURL:   defaultDockerProxyURL,
				StackName:        "",
				DockerTLSEnabled: false,
				DockerTLSVerify:  false,
				LogLevel:         defaultLogLevel,
				StatePath:        defaultStatePath,
			},
		},
		{
			name:        "mapping file without compose url",
			env:         map[string]string{},
			mappingFile: "stacks:\n  - name: prod\n    compose_url: https://example.com/compose.yml\n",
			want: Config{
				PollInterval:     defaultPollInterval,
				ComposeTimeout:   defaultComposeTimeout,
				DockerAPITimeout: defaultDockerAPITimeout,
				DockerProxyURL:   defaultDockerProxyURL,
				StackName:        "",
				DockerTLSEnabled: false,
				DockerTLSVerify:  false,
				LogLevel:         defaultLogLevel,
				StatePath:        defaultStatePath,
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
				PollInterval:     defaultPollInterval,
				ComposeTimeout:   defaultComposeTimeout,
				DockerAPITimeout: defaultDockerAPITimeout,
				ComposeURL:       "https://example.com/compose.yml",
				DockerProxyURL:   defaultDockerProxyURL,
				StackName:        "",
				DockerTLSEnabled: false,
				DockerTLSVerify:  false,
				SlackWebhookURL:  "https://hooks.slack.com/services/T00/B00/XXX",
				LogLevel:         defaultLogLevel,
				StatePath:        defaultStatePath,
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
				PollInterval:     45 * time.Second,
				ComposeTimeout:   defaultComposeTimeout,
				DockerAPITimeout: defaultDockerAPITimeout,
				ComposeURL:       "https://example.com/compose.yml",
				DockerProxyURL:   "http://proxy:2375",
				StackName:        "",
				DockerTLSEnabled: false,
				DockerTLSVerify:  false,
				LogLevel:         defaultLogLevel,
				StatePath:        defaultStatePath,
			},
		},
		{
			name: "stack name set",
			env: map[string]string{
				envComposeURL: "https://example.com/compose.yml",
				envStackName:  "prod",
			},
			want: Config{
				PollInterval:     defaultPollInterval,
				ComposeTimeout:   defaultComposeTimeout,
				DockerAPITimeout: defaultDockerAPITimeout,
				ComposeURL:       "https://example.com/compose.yml",
				DockerProxyURL:   defaultDockerProxyURL,
				StackName:        "prod",
				DockerTLSEnabled: false,
				DockerTLSVerify:  false,
				LogLevel:         defaultLogLevel,
				StatePath:        defaultStatePath,
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
				PollInterval:     defaultPollInterval,
				ComposeTimeout:   defaultComposeTimeout,
				DockerAPITimeout: defaultDockerAPITimeout,
				ComposeURL:       "https://example.com/compose.yml",
				DockerProxyURL:   defaultDockerProxyURL,
				StackName:        "",
				DockerTLSEnabled: true,
				DockerTLSVerify:  true,
				DockerTLSCA:      "/tmp/ca.pem",
				DockerTLSCert:    "/tmp/cert.pem",
				DockerTLSKey:     "/tmp/key.pem",
				LogLevel:         defaultLogLevel,
				StatePath:        defaultStatePath,
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
				PollInterval:     defaultPollInterval,
				ComposeTimeout:   defaultComposeTimeout,
				DockerAPITimeout: defaultDockerAPITimeout,
				ComposeURL:       "https://example.com/compose.yml",
				DockerProxyURL:   defaultDockerProxyURL,
				StackName:        "",
				DockerTLSEnabled: true,
				DockerTLSVerify:  true,
				DockerTLSCA:      "/tmp/certs/ca.pem",
				DockerTLSCert:    "/tmp/certs/cert.pem",
				DockerTLSKey:     "/tmp/certs/key.pem",
				LogLevel:         defaultLogLevel,
				StatePath:        defaultStatePath,
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
				PollInterval:     defaultPollInterval,
				ComposeTimeout:   defaultComposeTimeout,
				DockerAPITimeout: defaultDockerAPITimeout,
				ComposeURL:       "https://example.com/compose.yml",
				DockerProxyURL:   defaultDockerProxyURL,
				StackName:        "",
				DockerTLSEnabled: true,
				DockerTLSVerify:  false,
				DockerTLSCert:    "/tmp/cert.pem",
				DockerTLSKey:     "/tmp/key.pem",
				LogLevel:         defaultLogLevel,
				StatePath:        defaultStatePath,
			},
		},
		{
			name: "custom docker api timeout",
			env: map[string]string{
				envComposeURL:       "https://example.com/compose.yml",
				envDockerAPITimeout: "60s",
			},
			want: Config{
				PollInterval:     defaultPollInterval,
				ComposeTimeout:   defaultComposeTimeout,
				DockerAPITimeout: 60 * time.Second,
				ComposeURL:       "https://example.com/compose.yml",
				DockerProxyURL:   defaultDockerProxyURL,
				StackName:        "",
				DockerTLSEnabled: false,
				DockerTLSVerify:  false,
				LogLevel:         defaultLogLevel,
				StatePath:        defaultStatePath,
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
				PollInterval:     defaultPollInterval,
				ComposeTimeout:   defaultComposeTimeout,
				DockerAPITimeout: defaultDockerAPITimeout,
				ComposeURL:       "https://example.com/compose.yml",
				DockerProxyURL:   defaultDockerProxyURL,
				StackName:        "",
				DockerTLSEnabled: false,
				DockerTLSVerify:  false,
				LogLevel:         "debug",
				StatePath:        defaultStatePath,
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
