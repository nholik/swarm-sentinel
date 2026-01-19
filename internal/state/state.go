package state

import (
	"context"
	"time"

	"github.com/nholik/swarm-sentinel/internal/health"
)

// StackSnapshot captures the persisted health state for a stack.
type StackSnapshot struct {
	DesiredFingerprint string                          `json:"desired_fingerprint"`
	Services           map[string]health.ServiceHealth `json:"services"`
	EvaluatedAt        time.Time                       `json:"evaluated_at"`
}

// CurrentStateVersion is the current schema version for the state file.
const CurrentStateVersion = 1

// State stores snapshots for all stacks.
type State struct {
	Version int                      `json:"version"`
	Stacks  map[string]StackSnapshot `json:"stacks"`
}

// Store defines the interface for persisting state.
type Store interface {
	Load(ctx context.Context) (State, error)
	Save(ctx context.Context, state State) error
}
