package notify

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nholik/swarm-sentinel/internal/health"
	"github.com/nholik/swarm-sentinel/internal/transition"
	"github.com/rs/zerolog"
)

func TestWebhookNotifierTemplateRendering(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		body = string(data)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier, err := NewWebhookNotifier(zerolog.Nop(), server.URL, `{"stack":"{{ .Stack }}","count":{{ len .Transitions }}}`)
	if err != nil {
		t.Fatalf("NewWebhookNotifier error: %v", err)
	}

	transitions := []transition.ServiceTransition{
		{Name: "api", CurrentStatus: health.StatusFailed},
	}

	if err := notifier.Notify(context.Background(), "alpha", transitions); err != nil {
		t.Fatalf("Notify error: %v", err)
	}

	if !strings.Contains(body, `"stack":"alpha"`) {
		t.Fatalf("expected stack in payload, got %s", body)
	}
	if !strings.Contains(body, `"count":1`) {
		t.Fatalf("expected count in payload, got %s", body)
	}
}

func TestWebhookNotifierRetriesOnServerError(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&calls, 1)
		if count <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier, err := NewWebhookNotifier(zerolog.Nop(), server.URL, "")
	if err != nil {
		t.Fatalf("NewWebhookNotifier error: %v", err)
	}
	notifier.poster.timing.backoffInitial = time.Millisecond
	notifier.poster.timing.backoffMax = 2 * time.Millisecond
	notifier.poster.timing.backoffMaxElapsed = 20 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	if err := notifier.Notify(ctx, "alpha", []transition.ServiceTransition{{Name: "api", CurrentStatus: health.StatusFailed}}); err != nil {
		t.Fatalf("expected retry to succeed, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("expected 3 attempts, got %d", got)
	}
}

func TestWebhookNotifierInvalidTemplate(t *testing.T) {
	_, err := NewWebhookNotifier(zerolog.Nop(), "http://example.com", "{{")
	if err == nil {
		t.Fatalf("expected template error")
	}
}
