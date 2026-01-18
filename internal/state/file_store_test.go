package state

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nholik/swarm-sentinel/internal/health"
	"github.com/rs/zerolog"
)

func TestFileStore_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "state.json")
	store := NewFileStore(path, zerolog.Nop())

	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	state := State{
		Stacks: map[string]StackSnapshot{
			"prod": {
				DesiredFingerprint: "abc123",
				EvaluatedAt:        now,
				Services: map[string]health.ServiceHealth{
					"api": {
						Name:   "api",
						Status: health.StatusDegraded,
						Reasons: []string{
							"replicas running 1/2",
						},
					},
				},
			},
			"staging": {
				DesiredFingerprint: "def456",
				EvaluatedAt:        now.Add(time.Minute),
				Services: map[string]health.ServiceHealth{
					"web": {
						Name:   "web",
						Status: health.StatusOK,
					},
				},
			},
		},
	}

	if err := store.Save(context.Background(), state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	loaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}

	if len(loaded.Stacks) != len(state.Stacks) {
		t.Fatalf("expected %d stacks, got %d", len(state.Stacks), len(loaded.Stacks))
	}

	if loaded.Stacks["prod"].DesiredFingerprint != "abc123" {
		t.Fatalf("unexpected prod fingerprint: %s", loaded.Stacks["prod"].DesiredFingerprint)
	}
	if loaded.Stacks["staging"].DesiredFingerprint != "def456" {
		t.Fatalf("unexpected staging fingerprint: %s", loaded.Stacks["staging"].DesiredFingerprint)
	}
	if loaded.Stacks["prod"].EvaluatedAt.IsZero() {
		t.Fatalf("expected evaluated time to be set")
	}
	if loaded.Stacks["prod"].Services["api"].Status != health.StatusDegraded {
		t.Fatalf("unexpected service status: %s", loaded.Stacks["prod"].Services["api"].Status)
	}
}

func TestFileStore_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "missing.json")
	store := NewFileStore(path, zerolog.Nop())

	state, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}

	if len(state.Stacks) != 0 {
		t.Fatalf("expected empty state, got %v", state.Stacks)
	}
}

func TestFileStore_CorruptFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "state.json")
	store := NewFileStore(path, zerolog.Nop())

	if err := os.WriteFile(path, []byte("{not-json"), 0o600); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}

	state, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}

	if len(state.Stacks) != 0 {
		t.Fatalf("expected empty state, got %v", state.Stacks)
	}
}

func TestFileStore_MultiStackSeparation(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "nested", "state.json")
	store := NewFileStore(path, zerolog.Nop())

	state := State{
		Stacks: map[string]StackSnapshot{
			"alpha": {DesiredFingerprint: "alpha"},
			"beta":  {DesiredFingerprint: "beta"},
		},
	}

	if err := store.Save(context.Background(), state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	loaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}

	if loaded.Stacks["alpha"].DesiredFingerprint != "alpha" {
		t.Fatalf("unexpected alpha fingerprint: %s", loaded.Stacks["alpha"].DesiredFingerprint)
	}
	if loaded.Stacks["beta"].DesiredFingerprint != "beta" {
		t.Fatalf("unexpected beta fingerprint: %s", loaded.Stacks["beta"].DesiredFingerprint)
	}
}
