package healthcheck

import (
	"encoding/json"
	"net/http"
	"time"
)

// HealthHandler serves /healthz responses.
func HealthHandler(tracker *Tracker, pollInterval time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := http.StatusServiceUnavailable
		snapshot := Snapshot{}
		if tracker != nil && tracker.Healthy(time.Now().UTC(), pollInterval) {
			status = http.StatusOK
			snapshot = tracker.Snapshot()
		} else if tracker != nil {
			snapshot = tracker.Snapshot()
		}
		writeJSON(w, status, snapshot)
	}
}

// ReadyHandler serves /readyz responses.
func ReadyHandler(tracker *Tracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := http.StatusServiceUnavailable
		snapshot := Snapshot{}
		if tracker != nil && tracker.Ready() {
			status = http.StatusOK
			snapshot = tracker.Snapshot()
		} else if tracker != nil {
			snapshot = tracker.Snapshot()
		}
		writeJSON(w, status, snapshot)
	}
}

func writeJSON(w http.ResponseWriter, status int, payload Snapshot) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
