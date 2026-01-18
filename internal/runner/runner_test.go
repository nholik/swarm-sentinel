package runner

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

type fakeTicker struct {
	ch      chan time.Time
	stopped bool
	mu      sync.Mutex
}

func (t *fakeTicker) C() <-chan time.Time {
	return t.ch
}

func (t *fakeTicker) Stop() {
	t.mu.Lock()
	t.stopped = true
	t.mu.Unlock()
}

func (t *fakeTicker) Stopped() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stopped
}

func TestRunner_Run_TriggersRunOnceOnTicks(t *testing.T) {
	ticker := &fakeTicker{ch: make(chan time.Time, 2)}
	runCalls := make(chan struct{}, 2)

	r := New(zerolog.Nop(), time.Second,
		WithTickerFactory(func(time.Duration) Ticker {
			return ticker
		}),
		WithRunOnce(func(context.Context) error {
			runCalls <- struct{}{}
			return nil
		}),
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		_ = r.Run(ctx)
		close(done)
	}()

	ticker.ch <- time.Now()
	ticker.ch <- time.Now()

	if !waitForCalls(runCalls, 2, time.Second) {
		t.Fatalf("expected two run calls")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("runner did not stop after cancel")
	}

	if !ticker.Stopped() {
		t.Fatalf("expected ticker to be stopped")
	}
}

func TestRunner_Run_StopsOnContextCancel(t *testing.T) {
	ticker := &fakeTicker{ch: make(chan time.Time, 1)}

	r := New(zerolog.Nop(), time.Second,
		WithTickerFactory(func(time.Duration) Ticker {
			return ticker
		}),
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		_ = r.Run(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("runner did not stop after cancel")
	}

	if !ticker.Stopped() {
		t.Fatalf("expected ticker to be stopped")
	}
}

func TestRunner_Run_RejectsZeroPollInterval(t *testing.T) {
	r := New(zerolog.Nop(), 0)

	err := r.Run(context.Background())
	if err == nil {
		t.Fatalf("expected error for zero poll interval")
	}
}

func TestRunner_Run_ImmediateFirstRun(t *testing.T) {
	ticker := &fakeTicker{ch: make(chan time.Time, 1)}
	runCalls := make(chan struct{}, 2)

	r := New(zerolog.Nop(), time.Second,
		WithTickerFactory(func(time.Duration) Ticker {
			return ticker
		}),
		WithRunOnce(func(context.Context) error {
			runCalls <- struct{}{}
			return nil
		}),
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		_ = r.Run(ctx)
		close(done)
	}()

	// Should receive immediate first run without any tick
	if !waitForCalls(runCalls, 1, time.Second) {
		t.Fatalf("expected immediate first run")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("runner did not stop after cancel")
	}
}

func waitForCalls(ch <-chan struct{}, count int, timeout time.Duration) bool {
	deadline := time.After(timeout)
	for i := 0; i < count; i++ {
		select {
		case <-ch:
		case <-deadline:
			return false
		}
	}
	return true
}
