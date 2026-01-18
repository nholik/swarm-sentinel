package compose

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultMaxBytes   int64 = 5 << 20
	defaultMaxRetries       = 3
	defaultRetryDelay       = 1 * time.Second
	maxRetryDelay           = 30 * time.Second
)

// Fetcher retrieves the desired compose file.
type Fetcher interface {
	Fetch(ctx context.Context, previousETag string) (FetchResult, error)
}

// FetchResult contains the fetched compose bytes and response metadata.
type FetchResult struct {
	Body         []byte
	ETag         string
	LastModified string
	NotModified  bool
}

// HTTPFetcher retrieves a compose file over HTTP with configurable retry logic.
type HTTPFetcher struct {
	url        string
	client     *http.Client
	maxBytes   int64
	maxRetries int
	retryDelay time.Duration
}

// HTTPFetcherOption configures an HTTPFetcher.
type HTTPFetcherOption func(*HTTPFetcher)

// WithMaxRetries sets the maximum number of retry attempts for transient failures.
// Default is 3. Set to 0 to disable retries.
func WithMaxRetries(n int) HTTPFetcherOption {
	return func(f *HTTPFetcher) {
		if n >= 0 {
			f.maxRetries = n
		}
	}
}

// WithRetryDelay sets the initial delay between retry attempts.
// Subsequent retries use exponential backoff (delay * 2^attempt).
// Default is 1 second.
func WithRetryDelay(d time.Duration) HTTPFetcherOption {
	return func(f *HTTPFetcher) {
		if d > 0 {
			f.retryDelay = d
		}
	}
}

// NewHTTPFetcher constructs an HTTPFetcher with the given URL and timeout.
func NewHTTPFetcher(url string, timeout time.Duration, maxBytes int64, opts ...HTTPFetcherOption) (*HTTPFetcher, error) {
	if strings.TrimSpace(url) == "" {
		return nil, errors.New("compose url must not be empty")
	}
	if timeout <= 0 {
		return nil, errors.New("timeout must be greater than zero")
	}
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}

	f := &HTTPFetcher{
		url: url,
		client: &http.Client{
			Timeout: timeout,
		},
		maxBytes:   maxBytes,
		maxRetries: defaultMaxRetries,
		retryDelay: defaultRetryDelay,
	}

	for _, opt := range opts {
		opt(f)
	}

	return f, nil
}

// Fetch downloads the compose file, optionally using ETag caching.
// Transient network errors are retried with exponential backoff.
func (f *HTTPFetcher) Fetch(ctx context.Context, previousETag string) (FetchResult, error) {
	var lastErr error

	for attempt := 0; attempt <= f.maxRetries; attempt++ {
		if attempt > 0 {
			delay := f.calculateBackoff(attempt)
			select {
			case <-ctx.Done():
				return FetchResult{}, ctx.Err()
			case <-time.After(delay):
			}
		}

		result, err := f.doFetch(ctx, previousETag)
		if err == nil {
			return result, nil
		}

		// Don't retry on context cancellation or non-retryable errors
		if ctx.Err() != nil {
			return FetchResult{}, ctx.Err()
		}
		if !isRetryableError(err) {
			return FetchResult{}, err
		}

		lastErr = err
	}

	return FetchResult{}, fmt.Errorf("fetch failed after %d attempts: %w", f.maxRetries+1, lastErr)
}

func (f *HTTPFetcher) calculateBackoff(attempt int) time.Duration {
	delay := f.retryDelay
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay > maxRetryDelay {
			delay = maxRetryDelay
			break
		}
	}
	return delay
}

// isRetryableError determines if an error is transient and worth retrying.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Non-retryable errors should not be retried
	var nonRetryable *nonRetryableError
	if errors.As(err, &nonRetryable) {
		return false
	}

	// Network errors are generally retryable
	var netErr interface{ Timeout() bool }
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	// Server errors (5xx) are retryable
	errStr := err.Error()
	if strings.Contains(errStr, "server error:") {
		return true
	}

	// Check for specific error messages indicating transient failures
	transientPatterns := []string{
		"connection refused",
		"connection reset",
		"no such host",
		"temporary failure",
		"i/o timeout",
		"EOF",
	}
	for _, pattern := range transientPatterns {
		if strings.Contains(strings.ToLower(errStr), strings.ToLower(pattern)) {
			return true
		}
	}

	return false
}

func (f *HTTPFetcher) doFetch(ctx context.Context, previousETag string) (FetchResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.url, http.NoBody)
	if err != nil {
		return FetchResult{}, fmt.Errorf("create request: %w", err)
	}
	if previousETag != "" {
		req.Header.Set("If-None-Match", previousETag)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return FetchResult{}, fmt.Errorf("fetch compose: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return FetchResult{
			ETag:         resp.Header.Get("ETag"),
			LastModified: resp.Header.Get("Last-Modified"),
			NotModified:  true,
		}, nil
	}

	// Retry on server errors (5xx)
	if resp.StatusCode >= 500 && resp.StatusCode < 600 {
		return FetchResult{}, fmt.Errorf("server error: %s", resp.Status)
	}

	if resp.StatusCode != http.StatusOK {
		return FetchResult{}, &nonRetryableError{fmt.Errorf("unexpected status: %s", resp.Status)}
	}

	body, err := readWithLimit(resp.Body, f.maxBytes)
	if err != nil {
		return FetchResult{}, err
	}
	if len(body) == 0 {
		return FetchResult{}, &nonRetryableError{errors.New("compose body is empty")}
	}

	return FetchResult{
		Body:         body,
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	}, nil
}

// nonRetryableError wraps errors that should not be retried.
type nonRetryableError struct {
	err error
}

func (e *nonRetryableError) Error() string {
	return e.err.Error()
}

func (e *nonRetryableError) Unwrap() error {
	return e.err
}

func readWithLimit(r io.Reader, maxBytes int64) ([]byte, error) {
	limited := io.LimitReader(r, maxBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read compose: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("compose body exceeds %d bytes", maxBytes)
	}
	return body, nil
}
