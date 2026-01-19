package notify

import (
	"context"

	"github.com/nholik/swarm-sentinel/internal/transition"
)

// Notifier delivers transition alerts to external systems.
type Notifier interface {
	Notify(ctx context.Context, stack string, transitions []transition.ServiceTransition) error
}
