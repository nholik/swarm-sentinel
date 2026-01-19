package swarm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestDockerClientPingSuccess(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_ping" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	t.Cleanup(server.Close)

	client, err := NewDockerClient(server.URL, 2*time.Second, TLSConfig{}, zerolog.Nop())
	if err != nil {
		t.Fatalf("NewDockerClient error: %v", err)
	}

	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("Ping error: %v", err)
	}
}

func TestDockerClientPingFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	client, err := NewDockerClient(server.URL, 2*time.Second, TLSConfig{}, zerolog.Nop())
	if err != nil {
		t.Fatalf("NewDockerClient error: %v", err)
	}

	if err := client.Ping(context.Background()); err == nil {
		t.Fatal("expected Ping error, got nil")
	}
}

func TestNewDockerClient_TLSMissingCerts(t *testing.T) {
	t.Parallel()

	_, err := NewDockerClient("http://example.com", time.Second, TLSConfig{
		Enabled: true,
		Verify:  true,
		CAFile:  "/tmp/ca.pem",
	}, zerolog.Nop())
	if err == nil {
		t.Fatal("expected error for missing cert/key")
	}
}

func TestNewDockerClient_TLSMissingCAWhenVerify(t *testing.T) {
	t.Parallel()

	_, err := NewDockerClient("http://example.com", time.Second, TLSConfig{
		Enabled:  true,
		Verify:   true,
		CertFile: "/tmp/cert.pem",
		KeyFile:  "/tmp/key.pem",
	}, zerolog.Nop())
	if err == nil {
		t.Fatal("expected error for missing CA")
	}
}

func TestNormalizeDockerHost(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		host       string
		tlsEnabled bool
		want       string
		wantErr    bool
	}{
		{
			name:       "http host",
			host:       "http://proxy:2375",
			tlsEnabled: false,
			want:       "tcp://proxy:2375",
		},
		{
			name:       "https host requires tls",
			host:       "https://proxy:2375",
			tlsEnabled: false,
			wantErr:    true,
		},
		{
			name:       "https host with tls",
			host:       "https://proxy:2375",
			tlsEnabled: true,
			want:       "tcp://proxy:2375",
		},
		{
			name:       "unix host with tls",
			host:       "unix:///var/run/docker.sock",
			tlsEnabled: true,
			wantErr:    true,
		},
		{
			name:       "raw host",
			host:       "tcp://proxy:2375",
			tlsEnabled: false,
			want:       "tcp://proxy:2375",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeDockerHost(tc.host, tc.tlsEnabled)
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
				t.Fatalf("unexpected host: %s", got)
			}
		})
	}
}
