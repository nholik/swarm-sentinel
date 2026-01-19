package notify

import (
	"context"

	"github.com/nholik/swarm-sentinel/internal/transition"
	"github.com/rs/zerolog"
)

// DryRunNotifier logs transitions without sending notifications.
type DryRunNotifier struct {
	logger zerolog.Logger
	inner  Notifier
}

// NewDryRunNotifier returns a notifier that suppresses delivery and logs instead.
func NewDryRunNotifier(logger zerolog.Logger, inner Notifier) *DryRunNotifier {
	return &DryRunNotifier{logger: logger, inner: inner}
}

// Notify implements Notifier.
func (n *DryRunNotifier) Notify(_ context.Context, stack string, transitions []transition.ServiceTransition) error {
	for _, change := range transitions {
		n.logger.Info().
			Str("stack", stack).
			Str("service", change.Name).
			Str("previous_status", string(change.PreviousStatus)).
			Str("current_status", string(change.CurrentStatus)).
			Strs("reasons", change.Reasons).
			Msg("[DRY-RUN] Would notify")
	}
	return nil
}
