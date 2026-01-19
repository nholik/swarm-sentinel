package notify

import (
	"context"

	"github.com/nholik/swarm-sentinel/internal/transition"
)

// MultiNotifier fans out notifications to multiple notifiers.
type MultiNotifier struct {
	notifiers []Notifier
}

// NewMultiNotifier creates a notifier that dispatches to all provided notifiers.
func NewMultiNotifier(notifiers ...Notifier) *MultiNotifier {
	filtered := make([]Notifier, 0, len(notifiers))
	for _, notifier := range notifiers {
		if notifier == nil {
			continue
		}
		filtered = append(filtered, notifier)
	}
	return &MultiNotifier{notifiers: filtered}
}

// Notify implements Notifier.
func (m *MultiNotifier) Notify(ctx context.Context, stack string, transitions []transition.ServiceTransition) error {
	var firstErr error
	for _, notifier := range m.notifiers {
		if notifier == nil {
			continue
		}
		if err := notifier.Notify(ctx, stack, transitions); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
