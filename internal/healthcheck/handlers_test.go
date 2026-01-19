package healthcheck

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthHandlerHealthy(t *testing.T) {
	tracker := NewTracker()
	tracker.RecordCycle(150*time.Millisecond, 1)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	handler := HealthHandler(tracker, 5*time.Second)
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.LastCycleTime == nil {
		t.Fatalf("expected last cycle time to be set")
	}
	if payload.StacksEvaluated != 1 {
		t.Fatalf("expected stacks evaluated 1, got %d", payload.StacksEvaluated)
	}
	if payload.CycleDurationMS != 150 {
		t.Fatalf("expected duration 150ms, got %d", payload.CycleDurationMS)
	}
}

func TestHealthHandlerUnhealthyWhenStale(t *testing.T) {
	tracker := NewTracker()
	tracker.RecordCycle(10*time.Millisecond, 1)
	tracker.lastCycle = time.Now().Add(-10 * time.Second)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	handler := HealthHandler(tracker, 3*time.Second)
	handler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestReadyHandler(t *testing.T) {
	tracker := NewTracker()

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	handler := ReadyHandler(tracker)
	handler(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 before ready, got %d", rec.Code)
	}

	tracker.RecordCycle(5*time.Millisecond, 1)
	rec = httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 after ready, got %d", rec.Code)
	}
}
