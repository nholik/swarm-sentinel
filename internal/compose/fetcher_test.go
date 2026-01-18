package compose

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPFetcher_Fetch_OK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", "etag-1")
		w.Header().Set("Last-Modified", "yesterday")
		_, _ = w.Write([]byte("compose: true\n"))
	}))
	defer server.Close()

	fetcher, err := NewHTTPFetcher(server.URL, time.Second, 1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, err := fetcher.Fetch(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.NotModified {
		t.Fatalf("expected fresh response")
	}
	if string(result.Body) != "compose: true\n" {
		t.Fatalf("unexpected body: %q", string(result.Body))
	}
	if result.ETag != "etag-1" {
		t.Fatalf("unexpected etag: %q", result.ETag)
	}
	if result.LastModified != "yesterday" {
		t.Fatalf("unexpected last-modified: %q", result.LastModified)
	}
}

func TestHTTPFetcher_Fetch_NotModified(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("If-None-Match"); got != "etag-1" {
			t.Fatalf("expected If-None-Match header, got %q", got)
		}
		w.Header().Set("ETag", "etag-1")
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	fetcher, err := NewHTTPFetcher(server.URL, time.Second, 1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, err := fetcher.Fetch(context.Background(), "etag-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.NotModified {
		t.Fatalf("expected not modified response")
	}
	if len(result.Body) != 0 {
		t.Fatalf("expected empty body")
	}
}

func TestHTTPFetcher_Fetch_RejectsEmptyBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()

	fetcher, err := NewHTTPFetcher(server.URL, time.Second, 1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = fetcher.Fetch(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "compose body is empty") {
		t.Fatalf("expected empty body error, got %v", err)
	}
}

func TestHTTPFetcher_Fetch_RejectsStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	fetcher, err := NewHTTPFetcher(server.URL, time.Second, 1024)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = fetcher.Fetch(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "unexpected status") {
		t.Fatalf("expected status error, got %v", err)
	}
}

func TestHTTPFetcher_Fetch_RejectsOversizeBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("abcdef"))
	}))
	defer server.Close()

	fetcher, err := NewHTTPFetcher(server.URL, time.Second, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = fetcher.Fetch(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected size error, got %v", err)
	}
}
