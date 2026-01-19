package notify

import (
	"context"

	"github.com/nholik/swarm-sentinel/internal/transition"
	"github.com/rs/zerolog"
)

// NoopNotifier drops notifications.
type NoopNotifier struct {
	logger zerolog.Logger
	reason string
}

// NewNoop returns a notifier that logs once and does nothing thereafter.
func NewNoop(logger zerolog.Logger, reason string) *NoopNotifier {
	if reason != "" {
		logger.Info().Msg(reason)
	}
	return &NoopNotifier{logger: logger, reason: reason}
}

// Notify implements Notifier.
func (n *NoopNotifier) Notify(_ context.Context, _ string, _ []transition.ServiceTransition) error {
	return nil
}
