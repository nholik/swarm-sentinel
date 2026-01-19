package notify

import (
	"context"
	"errors"
	"fmt"
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

func TestBuildSlackMessagesSingle(t *testing.T) {
	transitions := makeTransitions(2)

	messages := buildSlackMessages("alpha", transitions)
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	msg := messages[0]
	if !strings.Contains(msg.Text, "Stack alpha") {
		t.Fatalf("expected summary to include stack name, got %q", msg.Text)
	}
	if !strings.Contains(msg.Text, "2 service transition") {
		t.Fatalf("expected summary to include transition count, got %q", msg.Text)
	}
	if msg.Blocks == nil {
		t.Fatalf("expected blocks to be set")
	}
	if len(msg.Blocks.BlockSet) != slackReservedBlocks+2 {
		t.Fatalf("expected %d blocks, got %d", slackReservedBlocks+2, len(msg.Blocks.BlockSet))
	}
}

func TestBuildSlackMessagesChunking(t *testing.T) {
	total := slackMaxTransitions*2 + 3
	transitions := makeTransitions(total)

	messages := buildSlackMessages("beta", transitions)
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}

	for i, msg := range messages {
		if msg.Blocks == nil {
			t.Fatalf("message %d missing blocks", i)
		}
		if len(msg.Blocks.BlockSet) > slackMaxBlocks {
			t.Fatalf("message %d exceeds block limit: %d", i, len(msg.Blocks.BlockSet))
		}
		if !strings.Contains(msg.Text, fmt.Sprintf("part %d/3", i+1)) {
			t.Fatalf("message %d missing part marker: %q", i, msg.Text)
		}
		if !strings.Contains(msg.Text, fmt.Sprintf("%d service transition", total)) {
			t.Fatalf("message %d missing total count: %q", i, msg.Text)
		}
	}
}

func TestSlackNotifierRetriesOnServerError(t *testing.T) {
	t.Parallel()

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

	logger := zerolog.New(io.Discard)
	notifier := NewSlackNotifier(logger, server.URL,
		WithSlackTiming(time.Millisecond, 1, 5*time.Millisecond, 10*time.Millisecond, 50*time.Millisecond),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if err := notifier.Notify(ctx, "alpha", makeTransitions(1)); err != nil {
		t.Fatalf("expected retry to succeed, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("expected 3 attempts, got %d", got)
	}
}

func TestSlackNotifierRetryAfterError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	logger := zerolog.New(io.Discard)
	notifier := NewSlackNotifier(logger, server.URL,
		WithSlackTiming(time.Millisecond, 1, time.Millisecond, 2*time.Millisecond, 20*time.Millisecond),
	)
	slackNotifier, ok := notifier.(*SlackNotifier)
	if !ok {
		t.Fatalf("expected SlackNotifier, got %T", notifier)
	}

	err := slackNotifier.postOnce(context.Background(), []byte(`{}`))
	var retryAfterErr *retryAfterError
	if !errors.As(err, &retryAfterErr) {
		t.Fatalf("expected retry-after error, got %v", err)
	}
	if retryAfterErr.Duration != time.Second {
		t.Fatalf("expected 1s retry-after, got %s", retryAfterErr.Duration)
	}
}

func TestSlackNotifierRateLimitBlocks(t *testing.T) {
	t.Parallel()

	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	logger := zerolog.New(io.Discard)
	// Use 500ms rate interval to test rate limiting
	notifier := NewSlackNotifier(logger, server.URL,
		WithSlackTiming(500*time.Millisecond, 1, time.Millisecond, 2*time.Millisecond, 20*time.Millisecond),
	)

	if err := notifier.Notify(context.Background(), "alpha", makeTransitions(1)); err != nil {
		t.Fatalf("expected first notify to succeed, got %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := notifier.Notify(ctx, "alpha", makeTransitions(1))
	if err == nil {
		t.Fatalf("expected rate limit error, got nil")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected rate limit to block second call, got %d", got)
	}
}

func TestSlackNotifierClientErrorNotRetried(t *testing.T) {
	t.Parallel()

	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid_payload"))
	}))
	defer server.Close()

	logger := zerolog.New(io.Discard)
	notifier := NewSlackNotifier(logger, server.URL,
		WithSlackTiming(time.Millisecond, 1, time.Millisecond, 2*time.Millisecond, 20*time.Millisecond),
	)

	err := notifier.Notify(context.Background(), "alpha", makeTransitions(1))
	if err == nil {
		t.Fatalf("expected error for 400 response, got nil")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Fatalf("expected error to contain status code, got %v", err)
	}
	if !strings.Contains(err.Error(), "invalid_payload") {
		t.Fatalf("expected error to contain response body, got %v", err)
	}
	// 4xx errors should not be retried
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected exactly 1 call (no retries for 4xx), got %d", got)
	}
}

func TestSlackNotifierContextCancellation(t *testing.T) {
	t.Parallel()

	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		// Always return server error to trigger retry
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	logger := zerolog.New(io.Discard)
	notifier := NewSlackNotifier(logger, server.URL,
		WithSlackTiming(time.Millisecond, 1, 100*time.Millisecond, 200*time.Millisecond, 1*time.Second),
	)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel context after first call
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	err := notifier.Notify(ctx, "alpha", makeTransitions(1))
	if err == nil {
		t.Fatalf("expected context cancellation error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled error, got %v", err)
	}
}

func makeTransitions(count int) []transition.ServiceTransition {
	transitions := make([]transition.ServiceTransition, count)
	for i := 0; i < count; i++ {
		transitions[i] = transition.ServiceTransition{
			Name:           fmt.Sprintf("svc-%02d", i+1),
			PreviousStatus: health.StatusOK,
			CurrentStatus:  health.StatusFailed,
			Reasons:        []string{"missing service"},
		}
	}
	return transitions
}
