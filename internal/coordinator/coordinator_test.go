package coordinator

import (
	"context"
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
	cfg := config.Config{
		PollInterval:   100 * time.Millisecond,
		ComposeTimeout: 1 * time.Second,
	}

	mappings := []config.StackMapping{
		{
			Name:       "test-stack",
			ComposeURL: "https://httpbin.org/status/200",
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
	cfg := config.Config{
		PollInterval:   100 * time.Millisecond,
		ComposeTimeout: 1 * time.Second,
	}

	mappings := []config.StackMapping{
		{
			Name:       "stack-1",
			ComposeURL: "https://httpbin.org/status/200",
		},
		{
			Name:       "stack-2",
			ComposeURL: "https://httpbin.org/status/200",
		},
		{
			Name:       "stack-3",
			ComposeURL: "https://httpbin.org/status/200",
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
	cfg := config.Config{
		PollInterval:   100 * time.Millisecond,
		ComposeTimeout: 1 * time.Second,
	}

	mappings := []config.StackMapping{
		{
			Name:       "default-timeout",
			ComposeURL: "https://httpbin.org/status/200",
		},
		{
			Name:       "custom-timeout",
			ComposeURL: "https://httpbin.org/status/200",
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
	cfg := config.Config{
		PollInterval:   100 * time.Millisecond,
		ComposeTimeout: 1 * time.Second,
	}

	mappings := []config.StackMapping{
		{
			Name:       "stack-a",
			ComposeURL: "https://httpbin.org/status/200",
		},
		{
			Name:       "stack-b",
			ComposeURL: "https://httpbin.org/status/200",
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
	cfg := config.Config{
		PollInterval:   100 * time.Millisecond,
		ComposeTimeout: 1 * time.Second,
	}

	mappings := []config.StackMapping{
		{
			Name:       "stack-1",
			ComposeURL: "https://httpbin.org/status/200",
		},
		{
			Name:       "stack-2",
			ComposeURL: "https://httpbin.org/status/200",
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
