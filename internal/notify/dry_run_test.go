package notify

import (
	"context"
	"testing"

	"github.com/nholik/swarm-sentinel/internal/health"
	"github.com/nholik/swarm-sentinel/internal/transition"
	"github.com/rs/zerolog"
)

type countingNotifier struct {
	calls int
}

func (n *countingNotifier) Notify(context.Context, string, []transition.ServiceTransition) error {
	n.calls++
	return nil
}

func TestDryRunNotifierSuppressesDelivery(t *testing.T) {
	inner := &countingNotifier{}
	dryRun := NewDryRunNotifier(zerolog.Nop(), inner)

	transitions := []transition.ServiceTransition{
		{Name: "api", CurrentStatus: health.StatusFailed},
	}

	if err := dryRun.Notify(context.Background(), "alpha", transitions); err != nil {
		t.Fatalf("Notify error: %v", err)
	}
	if inner.calls != 0 {
		t.Fatalf("expected no notifier calls, got %d", inner.calls)
	}
}
