package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nholik/swarm-sentinel/internal/health"
	"github.com/nholik/swarm-sentinel/internal/transition"
	"github.com/rs/zerolog"
	"github.com/slack-go/slack"
)

const (
	slackMaxBlocks = 50
	// slackReservedBlocks accounts for header block + context block in each message
	slackReservedBlocks = 2
	slackMaxTransitions = slackMaxBlocks - slackReservedBlocks
)

type SlackNotifier struct {
	logger     zerolog.Logger
	webhookURL string
	timing     timingConfig
	poster     *httpPoster
}

// SlackOption customizes SlackNotifier behavior.
type SlackOption func(*SlackNotifier)

// WithSlackTiming overrides timing parameters (primarily for testing).
func WithSlackTiming(rateInterval time.Duration, rateBurst int, backoffInitial, backoffMax, backoffMaxElapsed time.Duration) SlackOption {
	return func(s *SlackNotifier) {
		s.timing.rateInterval = rateInterval
		s.timing.rateBurst = rateBurst
		s.timing.backoffInitial = backoffInitial
		s.timing.backoffMax = backoffMax
		s.timing.backoffMaxElapsed = backoffMaxElapsed
	}
}

// NewSlackNotifier creates a Slack notifier or a noop notifier when the webhook is empty.
func NewSlackNotifier(logger zerolog.Logger, webhookURL string, opts ...SlackOption) Notifier {
	if webhookURL == "" {
		return NewNoop(logger, "slack webhook not configured; notifications disabled")
	}

	notifier := &SlackNotifier{
		logger:     logger,
		webhookURL: webhookURL,
		timing:     defaultTiming,
	}

	for _, opt := range opts {
		opt(notifier)
	}

	notifier.poster = newHTTPPoster(logger, "slack", webhookURL, "application/json", notifier.timing)

	return notifier
}

// Notify implements Notifier.
func (n *SlackNotifier) Notify(ctx context.Context, stack string, transitions []transition.ServiceTransition) error {
	if len(transitions) == 0 {
		return nil
	}
	stackName := stack
	if stackName == "" {
		stackName = "default"
	}
	if err := n.poster.waitForRateLimit(ctx, stackName); err != nil {
		return err
	}

	messages := buildSlackMessages(stackName, transitions)
	for _, message := range messages {
		payload, err := json.Marshal(message)
		if err != nil {
			return fmt.Errorf("marshal slack payload: %w", err)
		}
		if err := n.poster.postWithRetry(ctx, payload); err != nil {
			return err
		}
	}

	n.logger.Debug().
		Str("stack", stackName).
		Int("transitions", len(transitions)).
		Int("messages", len(messages)).
		Msg("slack notification sent")

	return nil
}

func (n *SlackNotifier) postOnce(ctx context.Context, payload []byte) error {
	return n.poster.postOnce(ctx, payload)
}

func buildSlackMessages(stack string, transitions []transition.ServiceTransition) []slack.WebhookMessage {
	if len(transitions) == 0 {
		return nil
	}
	if slackMaxTransitions <= 0 {
		return []slack.WebhookMessage{buildSlackMessage(stack, transitions, len(transitions), 1, 1)}
	}

	total := len(transitions)
	chunkTotal := (total + slackMaxTransitions - 1) / slackMaxTransitions
	messages := make([]slack.WebhookMessage, 0, chunkTotal)

	for i := 0; i < total; i += slackMaxTransitions {
		end := i + slackMaxTransitions
		if end > total {
			end = total
		}
		partIndex := (i / slackMaxTransitions) + 1
		messages = append(messages, buildSlackMessage(stack, transitions[i:end], total, partIndex, chunkTotal))
	}
	return messages
}

func buildSlackMessage(stack string, transitions []transition.ServiceTransition, total int, partIndex int, partTotal int) slack.WebhookMessage {
	summary := fmt.Sprintf("Stack %s: %d service transition(s)", stack, total)
	if partTotal > 1 {
		summary = fmt.Sprintf("%s (part %d/%d)", summary, partIndex, partTotal)
	}
	header := slack.NewHeaderBlock(slack.NewTextBlockObject("plain_text", summary, false, false))
	contextElements := []slack.MixedElement{
		slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("Stack: *%s*", stack), false, false),
	}
	if partTotal > 1 {
		contextElements = append(contextElements, slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("Batch: %d/%d", partIndex, partTotal), false, false))
	}
	context := slack.NewContextBlock("", contextElements...)

	blocks := []slack.Block{header, context}
	for _, change := range transitions {
		blocks = append(blocks, buildTransitionBlock(change))
	}

	blockSet := slack.Blocks{BlockSet: blocks}
	return slack.WebhookMessage{
		Text:   summary,
		Blocks: &blockSet,
	}
}

func buildTransitionBlock(change transition.ServiceTransition) slack.Block {
	title := fmt.Sprintf("*%s*: `%s` → `%s`", change.Name, statusLabel(change.PreviousStatus), statusLabel(change.CurrentStatus))
	text := slack.NewTextBlockObject("mrkdwn", title, false, false)

	fields := make([]*slack.TextBlockObject, 0, 4)
	if len(change.Reasons) > 0 {
		fields = append(fields, slack.NewTextBlockObject("mrkdwn", "*Reasons:*\n"+strings.Join(change.Reasons, ", "), false, false))
	}
	if change.ReplicaChange != nil {
		fields = append(fields, slack.NewTextBlockObject("mrkdwn", formatReplicaChange(change.ReplicaChange), false, false))
	}
	if change.ImageChange != nil {
		fields = append(fields, slack.NewTextBlockObject("mrkdwn", formatImageChange(change.ImageChange), false, false))
	}
	if len(change.Drift) > 0 {
		fields = append(fields, slack.NewTextBlockObject("mrkdwn", formatDrift(change.Drift), false, false))
	}

	return slack.NewSectionBlock(text, fields, nil)
}

func formatReplicaChange(change *transition.ReplicaChange) string {
	return fmt.Sprintf("*Replicas:*\nDesired %d (Δ %d), Running %d (Δ %d)",
		change.CurrentDesired, change.DesiredDelta, change.CurrentRunning, change.RunningDelta)
}

func formatImageChange(change *transition.ImageChange) string {
	desired := change.CurrentDesired
	if desired == "" {
		desired = "unknown"
	}
	actual := change.CurrentActual
	if actual == "" {
		actual = "unknown"
	}
	return fmt.Sprintf("*Image:*\nDesired `%s`\nActual `%s`", desired, actual)
}

func formatDrift(drift []health.DriftDetail) string {
	parts := make([]string, 0, len(drift))
	for _, detail := range drift {
		if detail.Resource != "" && detail.Name != "" {
			parts = append(parts, fmt.Sprintf("%s %s/%s", detail.Kind, detail.Resource, detail.Name))
			continue
		}
		if detail.Name != "" {
			parts = append(parts, fmt.Sprintf("%s %s", detail.Kind, detail.Name))
			continue
		}
		parts = append(parts, string(detail.Kind))
	}
	return "*Drift:*\n• " + strings.Join(parts, "\n• ")
}

func statusLabel(status health.ServiceStatus) string {
	if status == "" {
		return "UNKNOWN"
	}
	return string(status)
}
