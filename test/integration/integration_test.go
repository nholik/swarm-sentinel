//go:build integration

package integration

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/nholik/swarm-sentinel/internal/compose"
	"github.com/nholik/swarm-sentinel/internal/logging"
	"github.com/nholik/swarm-sentinel/internal/swarm"
)

// TestIntegrationComposeAndSwarm verifies the integration between
// compose fetching and Swarm API access using real Docker.
//
// Prerequisites:
//   - Docker daemon running
//   - docker compose -f test/integration/docker-compose.yml up -d
//
// Run with: go test -tags=integration -v ./test/integration/...
func TestIntegrationComposeAndSwarm(t *testing.T) {
	// Skip if integration test environment is not set up
	composeServerURL := getEnv("TEST_COMPOSE_URL", "http://localhost:8888/healthy-stack.yml")
	dockerProxyURL := getEnv("TEST_DOCKER_PROXY_URL", "http://localhost:2375")

	// Verify compose server is reachable
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := checkEndpoint(ctx, composeServerURL); err != nil {
		t.Skipf("compose server not reachable (run docker compose up first): %v", err)
	}

	if err := checkEndpoint(ctx, dockerProxyURL+"/_ping"); err != nil {
		t.Skipf("docker proxy not reachable (run docker compose up first): %v", err)
	}

	// Test compose fetching
	t.Run("ComposeFetch", func(t *testing.T) {
		fetcher, err := compose.NewHTTPFetcher(composeServerURL, 10*time.Second, 0)
		if err != nil {
			t.Fatalf("create fetcher: %v", err)
		}

		result, err := fetcher.Fetch(context.Background(), "")
		if err != nil {
			t.Fatalf("fetch compose: %v", err)
		}

		if len(result.Body) == 0 {
			t.Fatal("expected non-empty compose body")
		}

		// Parse the compose file
		desired, err := compose.ParseDesiredState(context.Background(), result.Body)
		if err != nil {
			t.Fatalf("parse compose: %v", err)
		}

		if len(desired.Services) == 0 {
			t.Fatal("expected at least one service in compose")
		}

		t.Logf("Parsed %d services from compose", len(desired.Services))
	})

	// Test Docker API access
	t.Run("DockerPing", func(t *testing.T) {
		logger := logging.New()
		client, err := swarm.NewDockerClient(dockerProxyURL, 10*time.Second, swarm.TLSConfig{}, logger)
		if err != nil {
			t.Fatalf("create docker client: %v", err)
		}
		defer client.Close()

		if err := client.Ping(context.Background()); err != nil {
			t.Fatalf("docker ping: %v", err)
		}
	})

	// Test actual state collection
	t.Run("ActualState", func(t *testing.T) {
		logger := logging.New()
		client, err := swarm.NewDockerClient(dockerProxyURL, 10*time.Second, swarm.TLSConfig{}, logger)
		if err != nil {
			t.Fatalf("create docker client: %v", err)
		}
		defer client.Close()

		state, err := client.GetActualState(context.Background(), "")
		if err != nil {
			t.Fatalf("get actual state: %v", err)
		}

		t.Logf("Found %d services in Swarm", len(state.Services))
	})
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func checkEndpoint(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
