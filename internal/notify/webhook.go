package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"text/template"
	"time"

	"github.com/nholik/swarm-sentinel/internal/transition"
	"github.com/rs/zerolog"
)

const defaultWebhookTemplate = `{"stack":"{{ .Stack }}","transitions":{{ toJson .Transitions }}}`

// WebhookPayload is the template context for webhook notifications.
type WebhookPayload struct {
	Stack       string
	Transitions []transition.ServiceTransition
	GeneratedAt time.Time
}

// WebhookNotifier sends transition notifications to a generic webhook.
type WebhookNotifier struct {
	logger   zerolog.Logger
	template *template.Template
	poster   *httpPoster
}

// NewWebhookNotifier creates a webhook notifier with the provided template.
func NewWebhookNotifier(logger zerolog.Logger, webhookURL string, tmpl string) (*WebhookNotifier, error) {
	if webhookURL == "" {
		return nil, nil
	}
	if tmpl == "" {
		tmpl = defaultWebhookTemplate
	}

	parsed, err := template.New("webhook").Funcs(template.FuncMap{
		"toJson": func(v any) (string, error) {
			encoded, err := json.Marshal(v)
			if err != nil {
				return "", err
			}
			return string(encoded), nil
		},
	}).Parse(tmpl)
	if err != nil {
		return nil, fmt.Errorf("parse webhook template: %w", err)
	}

	return &WebhookNotifier{
		logger:   logger,
		template: parsed,
		poster:   newHTTPPoster(logger, "webhook", webhookURL, "application/json", defaultTiming),
	}, nil
}

// Notify implements Notifier.
func (n *WebhookNotifier) Notify(ctx context.Context, stack string, transitions []transition.ServiceTransition) error {
	if len(transitions) == 0 || n == nil {
		return nil
	}

	stackName := stack
	if stackName == "" {
		stackName = "default"
	}

	if err := n.poster.waitForRateLimit(ctx, stackName); err != nil {
		return err
	}

	payload := WebhookPayload{
		Stack:       stackName,
		Transitions: transitions,
		GeneratedAt: time.Now().UTC(),
	}

	var buf bytes.Buffer
	if err := n.template.Execute(&buf, payload); err != nil {
		return fmt.Errorf("render webhook template: %w", err)
	}

	if err := n.poster.postWithRetry(ctx, buf.Bytes()); err != nil {
		return err
	}

	n.logger.Debug().
		Str("stack", stackName).
		Int("transitions", len(transitions)).
		Msg("webhook notification sent")

	return nil
}
