package swarm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDockerClientPingSuccess(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_ping" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	t.Cleanup(server.Close)

	client, err := NewDockerClient(server.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("NewDockerClient error: %v", err)
	}

	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("Ping error: %v", err)
	}
}

func TestDockerClientPingFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	client, err := NewDockerClient(server.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("NewDockerClient error: %v", err)
	}

	if err := client.Ping(context.Background()); err == nil {
		t.Fatal("expected Ping error, got nil")
	}
}
