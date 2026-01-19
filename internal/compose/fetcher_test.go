package compose

import (
	"context"
	"errors"
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
	if err == nil {
		t.Fatal("expected error for bad status")
	}

	// Check that we can extract status code from FetchError
	var fetchErr *FetchError
	if errors.As(err, &fetchErr) {
		if fetchErr.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected status code %d, got %d", http.StatusBadRequest, fetchErr.StatusCode)
		}
		if fetchErr.IsRetryable() {
			t.Fatal("4xx errors should not be retryable")
		}
	} else {
		// Fallback to string matching for wrapped errors
		if !strings.Contains(err.Error(), "400") && !strings.Contains(err.Error(), "Bad Request") {
			t.Fatalf("expected status error, got %v", err)
		}
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

func TestHTTPFetcher_Fetch_RetriesOnServerError(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("success"))
	}))
	defer server.Close()

	fetcher, err := NewHTTPFetcher(server.URL, time.Second, 1024,
		WithMaxRetries(3),
		WithRetryDelay(10*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, err := fetcher.Fetch(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result.Body) != "success" {
		t.Fatalf("unexpected body: %q", result.Body)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestHTTPFetcher_Fetch_NoRetryOn4xx(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	fetcher, err := NewHTTPFetcher(server.URL, time.Second, 1024,
		WithMaxRetries(3),
		WithRetryDelay(10*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = fetcher.Fetch(context.Background(), "")
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt (no retry on 4xx), got %d", attempts)
	}
}

func TestHTTPFetcher_Fetch_RetriesExhausted(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	fetcher, err := NewHTTPFetcher(server.URL, time.Second, 1024,
		WithMaxRetries(2),
		WithRetryDelay(10*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = fetcher.Fetch(context.Background(), "")
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	if !strings.Contains(err.Error(), "after 3 attempts") {
		t.Fatalf("expected retry exhausted error, got: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts (1 initial + 2 retries), got %d", attempts)
	}
}

func TestHTTPFetcher_Fetch_RespectsContextCancellation(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	fetcher, err := NewHTTPFetcher(server.URL, time.Second, 1024,
		WithMaxRetries(10),
		WithRetryDelay(100*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after a short delay to allow one retry
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err = fetcher.Fetch(ctx, "")
	if err == nil {
		t.Fatal("expected context error")
	}
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

func TestHTTPFetcher_Fetch_NoRetriesWhenDisabled(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	fetcher, err := NewHTTPFetcher(server.URL, time.Second, 1024,
		WithMaxRetries(0),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = fetcher.Fetch(context.Background(), "")
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt (retries disabled), got %d", attempts)
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name      string
		errString string
		want      bool
	}{
		{"nil error", "", false},
		{"connection refused", "dial tcp: connection refused", true},
		{"connection reset", "read: connection reset by peer", true},
		{"no such host", "dial tcp: lookup example.com: no such host", true},
		{"EOF", "unexpected EOF", true},
		{"timeout", "i/o timeout", true},
		{"not found", "404 not found", false},
		{"bad request", "400 bad request", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			if tt.errString != "" {
				err = &testError{msg: tt.errString}
			}
			got := isRetryableError(err)
			if got != tt.want {
				t.Errorf("isRetryableError(%q) = %v, want %v", tt.errString, got, tt.want)
			}
		})
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
