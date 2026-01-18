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

const defaultMaxBytes int64 = 5 << 20

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

// HTTPFetcher retrieves a compose file over HTTP.
type HTTPFetcher struct {
	url      string
	client   *http.Client
	maxBytes int64
}

// NewHTTPFetcher constructs an HTTPFetcher with the given URL and timeout.
func NewHTTPFetcher(url string, timeout time.Duration, maxBytes int64) (*HTTPFetcher, error) {
	if strings.TrimSpace(url) == "" {
		return nil, errors.New("compose url must not be empty")
	}
	if timeout <= 0 {
		return nil, errors.New("timeout must be greater than zero")
	}
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}

	return &HTTPFetcher{
		url: url,
		client: &http.Client{
			Timeout: timeout,
		},
		maxBytes: maxBytes,
	}, nil
}

// Fetch downloads the compose file, optionally using ETag caching.
func (f *HTTPFetcher) Fetch(ctx context.Context, previousETag string) (FetchResult, error) {
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

	if resp.StatusCode != http.StatusOK {
		return FetchResult{}, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	body, err := readWithLimit(resp.Body, f.maxBytes)
	if err != nil {
		return FetchResult{}, err
	}
	if len(body) == 0 {
		return FetchResult{}, errors.New("compose body is empty")
	}

	return FetchResult{
		Body:         body,
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	}, nil
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
