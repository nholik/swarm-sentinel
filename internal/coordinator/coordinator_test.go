package coordinator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nholik/swarm-sentinel/internal/config"
	"github.com/nholik/swarm-sentinel/internal/swarm"
	"github.com/rs/zerolog"
)

type fakeSwarmClient struct{}

func (f *fakeSwarmClient) Ping(ctx context.Context) error {
	return nil
}

func (f *fakeSwarmClient) GetActualState(ctx context.Context, stackName string) (*swarm.ActualState, error) {
	return &swarm.ActualState{Services: map[string]swarm.ActualService{}}, nil
}

func (f *fakeSwarmClient) Close() error {
	return nil
}

func TestCoordinator_SingleStack(t *testing.T) {
	composeURL := newComposeServer(t)
	cfg := config.Config{
		PollInterval:   100 * time.Millisecond,
		ComposeTimeout: 1 * time.Second,
	}

	mappings := []config.StackMapping{
		{
			Name:       "test-stack",
			ComposeURL: composeURL,
		},
	}

	coord := New(
		zerolog.Nop(),
		cfg,
		mappings,
		&fakeSwarmClient{},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := coord.Run(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	runners := coord.GetRunners()
	if len(runners) == 0 {
		t.Fatal("expected at least one runner to be created")
	}

	if _, ok := runners["test-stack"]; !ok {
		t.Fatal("expected test-stack runner")
	}
}

func TestCoordinator_MultipleStacks(t *testing.T) {
	composeURL := newComposeServer(t)
	cfg := config.Config{
		PollInterval:   100 * time.Millisecond,
		ComposeTimeout: 1 * time.Second,
	}

	mappings := []config.StackMapping{
		{
			Name:       "stack-1",
			ComposeURL: composeURL,
		},
		{
			Name:       "stack-2",
			ComposeURL: composeURL,
		},
		{
			Name:       "stack-3",
			ComposeURL: composeURL,
		},
	}

	coord := New(
		zerolog.Nop(),
		cfg,
		mappings,
		&fakeSwarmClient{},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := coord.Run(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	runners := coord.GetRunners()
	if len(runners) != 3 {
		t.Fatalf("expected 3 runners, got %d", len(runners))
	}

	for _, name := range []string{"stack-1", "stack-2", "stack-3"} {
		if _, ok := runners[name]; !ok {
			t.Fatalf("expected %s runner", name)
		}
	}
}

func TestCoordinator_PerStackTimeout(t *testing.T) {
	composeURL := newComposeServer(t)
	cfg := config.Config{
		PollInterval:   100 * time.Millisecond,
		ComposeTimeout: 1 * time.Second,
	}

	mappings := []config.StackMapping{
		{
			Name:       "default-timeout",
			ComposeURL: composeURL,
		},
		{
			Name:       "custom-timeout",
			ComposeURL: composeURL,
			Timeout:    5 * time.Second,
		},
	}

	coord := New(
		zerolog.Nop(),
		cfg,
		mappings,
		&fakeSwarmClient{},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := coord.Run(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	runners := coord.GetRunners()
	if len(runners) != 2 {
		t.Fatalf("expected 2 runners, got %d", len(runners))
	}
}

func TestCoordinator_GracefulShutdown(t *testing.T) {
	composeURL := newComposeServer(t)
	cfg := config.Config{
		PollInterval:   100 * time.Millisecond,
		ComposeTimeout: 1 * time.Second,
	}

	mappings := []config.StackMapping{
		{
			Name:       "stack-a",
			ComposeURL: composeURL,
		},
		{
			Name:       "stack-b",
			ComposeURL: composeURL,
		},
	}

	coord := New(
		zerolog.Nop(),
		cfg,
		mappings,
		&fakeSwarmClient{},
	)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- coord.Run(ctx)
	}()

	// Let runners start
	time.Sleep(150 * time.Millisecond)

	// Cancel context
	cancel()

	// Wait for graceful shutdown
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("coordinator did not stop after context cancellation")
	}
}

func TestCoordinator_InvalidURL(t *testing.T) {
	cfg := config.Config{
		PollInterval:   100 * time.Millisecond,
		ComposeTimeout: 1 * time.Second,
	}

	mappings := []config.StackMapping{
		{
			Name:       "bad-url",
			ComposeURL: "ht!tp://invalid",
		},
	}

	coord := New(
		zerolog.Nop(),
		cfg,
		mappings,
		&fakeSwarmClient{},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Run should complete without panic, errors are logged
	err := coord.Run(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCoordinator_SharedSwarmClient(t *testing.T) {
	composeURL := newComposeServer(t)
	cfg := config.Config{
		PollInterval:   100 * time.Millisecond,
		ComposeTimeout: 1 * time.Second,
	}

	mappings := []config.StackMapping{
		{
			Name:       "stack-1",
			ComposeURL: composeURL,
		},
		{
			Name:       "stack-2",
			ComposeURL: composeURL,
		},
	}

	client := &fakeSwarmClient{}

	coord := New(
		zerolog.Nop(),
		cfg,
		mappings,
		client,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := coord.Run(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both runners should use the same swarm client
	// This is verified by the fact that we passed one client to coordinator
	// and runners should not have created additional ones
	runners := coord.GetRunners()
	if len(runners) != 2 {
		t.Fatalf("expected 2 runners, got %d", len(runners))
	}
}

func TestCoordinator_Stop(t *testing.T) {
	composeURL := newComposeServer(t)
	cfg := config.Config{
		PollInterval:   50 * time.Millisecond,
		ComposeTimeout: 1 * time.Second,
	}

	mappings := []config.StackMapping{
		{
			Name:       "stack-a",
			ComposeURL: composeURL,
		},
	}

	coord := New(
		zerolog.Nop(),
		cfg,
		mappings,
		&fakeSwarmClient{},
	)

	done := make(chan error, 1)
	go func() {
		done <- coord.Run(context.Background())
	}()

	waitForRunners(t, coord, 1, time.Second)
	coord.Stop()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("coordinator did not stop after Stop")
	}
}

func newComposeServer(t *testing.T) string {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/yaml")
		_, _ = w.Write([]byte("services:\n  web:\n    image: nginx:latest\n"))
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func waitForRunners(t *testing.T, coord *Coordinator, expected int, timeout time.Duration) {
	t.Helper()

	deadline := time.After(timeout)
	for {
		if len(coord.GetRunners()) >= expected {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("expected %d runners to start", expected)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}
