package runner

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nholik/swarm-sentinel/internal/compose"
	"github.com/nholik/swarm-sentinel/internal/health"
	"github.com/nholik/swarm-sentinel/internal/state"
	"github.com/nholik/swarm-sentinel/internal/swarm"
	"github.com/nholik/swarm-sentinel/internal/transition"
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

func TestRunner_Run_LogsRuntimeErrors(t *testing.T) {
	cases := []struct {
		name string
		op   string
	}{
		{name: "compose fetch", op: "compose fetch"},
		{name: "swarm actual state", op: "swarm actual state"},
	}

	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			ticker := &fakeTicker{ch: make(chan time.Time, 1)}
			runCalls := make(chan struct{}, 2)
			var buffer bytes.Buffer
			logger := zerolog.New(&buffer)

			r := New(logger, time.Second,
				WithTickerFactory(func(time.Duration) Ticker {
					return ticker
				}),
				WithRunOnce(func(context.Context) error {
					runCalls <- struct{}{}
					return &RuntimeError{Op: testCase.op, Err: errors.New("boom")}
				}),
			)

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})

			go func() {
				_ = r.Run(ctx)
				close(done)
			}()

			if !waitForCalls(runCalls, 1, time.Second) {
				t.Fatalf("expected initial run call")
			}
			cancel()

			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatalf("runner did not stop after cancel")
			}

			logs := buffer.String()
			if !strings.Contains(logs, "\"runtime_error\":true") {
				t.Fatalf("expected runtime_error field in logs, got %s", logs)
			}
			if !strings.Contains(logs, testCase.op) {
				t.Fatalf("expected operation in logs, got %s", logs)
			}
		})
	}
}

type memoryStateStore struct {
	state state.State
}

func (m *memoryStateStore) Load(context.Context) (state.State, error) {
	if m.state.Stacks == nil {
		m.state.Stacks = map[string]state.StackSnapshot{}
	}
	return m.state, nil
}

func (m *memoryStateStore) Save(_ context.Context, st state.State) error {
	m.state = st
	return nil
}

type recordingNotifier struct {
	calls [][]transition.ServiceTransition
}

func (n *recordingNotifier) Notify(_ context.Context, _ string, transitions []transition.ServiceTransition) error {
	n.calls = append(n.calls, transitions)
	return nil
}

func TestRunner_AlertStabilizationDelaysNotification(t *testing.T) {
	store := &memoryStateStore{}
	notifier := &recordingNotifier{}

	r := New(zerolog.Nop(), time.Second,
		WithStateStore(store, &sync.Mutex{}),
		WithNotifier(notifier),
		WithAlertStabilizationCycles(2),
	)

	desired := compose.DesiredState{
		Services: map[string]compose.DesiredService{
			"api": {Image: "app:v1", Mode: "replicated", Replicas: 2},
		},
	}

	r.lastDesiredState = &desired
	r.lastActualState = &swarm.ActualState{
		Services: map[string]swarm.ActualService{
			"api": {Name: "api", Image: "app:v1", DesiredReplicas: 2, RunningReplicas: 2},
		},
	}

	if err := r.evaluateAndPersist(context.Background()); err != nil {
		t.Fatalf("initial evaluate: %v", err)
	}

	r.lastActualState = &swarm.ActualState{
		Services: map[string]swarm.ActualService{
			"api": {Name: "api", Image: "app:v1", DesiredReplicas: 2, RunningReplicas: 1},
		},
	}

	if err := r.evaluateAndPersist(context.Background()); err != nil {
		t.Fatalf("first degraded evaluate: %v", err)
	}
	if len(notifier.calls) != 0 {
		t.Fatalf("expected no notifications yet, got %d", len(notifier.calls))
	}

	if err := r.evaluateAndPersist(context.Background()); err != nil {
		t.Fatalf("second degraded evaluate: %v", err)
	}
	if len(notifier.calls) != 1 {
		t.Fatalf("expected one notification, got %d", len(notifier.calls))
	}
	if notifier.calls[0][0].CurrentStatus != health.StatusDegraded {
		t.Fatalf("expected degraded transition, got %s", notifier.calls[0][0].CurrentStatus)
	}
}

func TestRunner_FirstRunNonOKImmediate(t *testing.T) {
	store := &memoryStateStore{}
	notifier := &recordingNotifier{}

	r := New(zerolog.Nop(), time.Second,
		WithStateStore(store, &sync.Mutex{}),
		WithNotifier(notifier),
		WithAlertStabilizationCycles(3),
	)

	desired := compose.DesiredState{
		Services: map[string]compose.DesiredService{
			"api": {Image: "app:v1", Mode: "replicated", Replicas: 2},
		},
	}
	r.lastDesiredState = &desired
	r.lastActualState = &swarm.ActualState{
		Services: map[string]swarm.ActualService{
			"api": {Name: "api", Image: "app:v1", DesiredReplicas: 2, RunningReplicas: 1},
		},
	}

	if err := r.evaluateAndPersist(context.Background()); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if len(notifier.calls) != 1 {
		t.Fatalf("expected immediate notification, got %d", len(notifier.calls))
	}
	if notifier.calls[0][0].CurrentStatus != health.StatusDegraded {
		t.Fatalf("expected degraded transition, got %s", notifier.calls[0][0].CurrentStatus)
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

type recordingFetcher struct {
	results []compose.FetchResult
	calls   []string
	idx     int
}

func (f *recordingFetcher) Fetch(ctx context.Context, previousETag string) (compose.FetchResult, error) {
	f.calls = append(f.calls, previousETag)
	if f.idx >= len(f.results) {
		return compose.FetchResult{}, nil
	}
	result := f.results[f.idx]
	f.idx++
	return result, nil
}

func TestRunner_RunOnce_UsesComposeFetcherETag(t *testing.T) {
	validCompose := []byte(`
services:
  web:
    image: nginx:latest
`)
	fetcher := &recordingFetcher{
		results: []compose.FetchResult{
			{Body: validCompose, ETag: "etag-1"},
			{NotModified: true, ETag: "etag-1"},
		},
	}

	r := New(zerolog.Nop(), time.Second,
		WithComposeFetcher(fetcher),
	)

	if err := r.RunOnce(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := r.RunOnce(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fetcher.calls) != 2 {
		t.Fatalf("expected 2 fetch calls, got %d", len(fetcher.calls))
	}
	if fetcher.calls[0] != "" {
		t.Fatalf("expected empty etag on first call, got %q", fetcher.calls[0])
	}
	if fetcher.calls[1] != "etag-1" {
		t.Fatalf("expected etag-1 on second call, got %q", fetcher.calls[1])
	}

	// Verify desired state was parsed
	if r.lastDesiredState == nil {
		t.Fatalf("expected lastDesiredState to be set")
	}
	if _, ok := r.lastDesiredState.Services["web"]; !ok {
		t.Fatalf("expected web service in desired state")
	}
}

type fakeSwarmClient struct {
	calls      int
	stackNames []string
	state      *swarm.ActualState
	err        error
}

func (f *fakeSwarmClient) Ping(ctx context.Context) error {
	_ = ctx
	return nil
}

func (f *fakeSwarmClient) Close() error {
	return nil
}

func (f *fakeSwarmClient) GetActualState(ctx context.Context, stackName string) (*swarm.ActualState, error) {
	_ = ctx
	f.calls++
	f.stackNames = append(f.stackNames, stackName)
	return f.state, f.err
}

func TestRunner_RunOnce_CollectsActualStateWhenComposeUnchanged(t *testing.T) {
	validCompose := []byte(`
services:
  web:
    image: nginx:latest
`)
	fetcher := &recordingFetcher{
		results: []compose.FetchResult{
			{Body: validCompose, ETag: "etag-1"},
			{NotModified: true, ETag: "etag-1"},
		},
	}
	swarmClient := &fakeSwarmClient{
		state: &swarm.ActualState{
			Services: map[string]swarm.ActualService{},
		},
	}

	r := New(zerolog.Nop(), time.Second,
		WithComposeFetcher(fetcher),
		WithSwarmClient(swarmClient),
		WithStackName("prod"),
	)

	if err := r.RunOnce(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := r.RunOnce(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if swarmClient.calls != 2 {
		t.Fatalf("expected 2 swarm calls, got %d", swarmClient.calls)
	}
	if len(swarmClient.stackNames) != 2 || swarmClient.stackNames[0] != "prod" || swarmClient.stackNames[1] != "prod" {
		t.Fatalf("expected stack name to be passed on every call")
	}
}
