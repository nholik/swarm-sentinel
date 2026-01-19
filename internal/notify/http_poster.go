package notify

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog"
	"golang.org/x/time/rate"
)

const httpErrorBodyLimit = 1024

type timingConfig struct {
	timeout           time.Duration
	rateInterval      time.Duration
	rateBurst         int
	backoffMaxElapsed time.Duration
	backoffMax        time.Duration
	backoffInitial    time.Duration
}

var defaultTiming = timingConfig{
	timeout:           10 * time.Second,
	rateInterval:      1 * time.Second,
	rateBurst:         1,
	backoffMaxElapsed: 30 * time.Second,
	backoffMax:        10 * time.Second,
	backoffInitial:    1 * time.Second,
}

type httpPoster struct {
	logger      zerolog.Logger
	serviceName string
	webhookURL  string
	contentType string
	client      *retryablehttp.Client
	timing      timingConfig
	limiters    map[string]*rate.Limiter
	limiterMu   sync.Mutex
}

func newHTTPPoster(logger zerolog.Logger, serviceName, webhookURL, contentType string, timing timingConfig) *httpPoster {
	client := retryablehttp.NewClient()
	client.RetryMax = 0
	client.CheckRetry = func(_ context.Context, _ *http.Response, _ error) (bool, error) {
		return false, nil
	}
	client.Logger = nil
	client.HTTPClient = &http.Client{Timeout: timing.timeout}

	return &httpPoster{
		logger:      logger,
		serviceName: serviceName,
		webhookURL:  webhookURL,
		contentType: contentType,
		client:      client,
		timing:      timing,
		limiters:    make(map[string]*rate.Limiter),
	}
}

func (n *httpPoster) waitForRateLimit(ctx context.Context, stack string) error {
	limiter := n.getLimiter(stack)
	if limiter == nil {
		return nil
	}
	return limiter.Wait(ctx)
}

func (n *httpPoster) getLimiter(stack string) *rate.Limiter {
	n.limiterMu.Lock()
	defer n.limiterMu.Unlock()

	limiter, ok := n.limiters[stack]
	if ok {
		return limiter
	}
	limiter = rate.NewLimiter(rate.Every(n.timing.rateInterval), n.timing.rateBurst)
	n.limiters[stack] = limiter
	return limiter
}

func (n *httpPoster) postWithRetry(ctx context.Context, payload []byte) error {
	backoffCfg := backoff.NewExponentialBackOff()
	backoffCfg.InitialInterval = n.timing.backoffInitial
	backoffCfg.MaxInterval = n.timing.backoffMax
	backoffCfg.MaxElapsedTime = n.timing.backoffMaxElapsed
	backoffCfg.Reset()

	for {
		if err := n.postOnce(ctx, payload); err != nil {
			var retryAfter *retryAfterError
			if errors.As(err, &retryAfter) {
				if !sleepWithContext(ctx, retryAfter.Duration) {
					return ctx.Err()
				}
				continue
			}
			var retryable *retryableError
			if !errors.As(err, &retryable) {
				return err
			}
			wait := backoffCfg.NextBackOff()
			if wait == backoff.Stop {
				return err
			}
			if !sleepWithContext(ctx, wait) {
				return ctx.Err()
			}
			continue
		}
		return nil
	}
}

func (n *httpPoster) postOnce(ctx context.Context, payload []byte) error {
	reqCtx, cancel := context.WithTimeout(ctx, n.timing.timeout)
	defer cancel()

	req, err := retryablehttp.NewRequestWithContext(reqCtx, http.MethodPost, n.webhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build %s request: %w", n.serviceName, err)
	}
	req.Header.Set("Content-Type", n.contentType)

	resp, err := n.client.Do(req)
	if err != nil {
		return &retryableError{err: fmt.Errorf("%s request failed: %w", n.serviceName, err)}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, httpErrorBodyLimit))
	bodyText := strings.TrimSpace(string(body))

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		if wait, ok := parseRetryAfter(resp.Header.Get("Retry-After")); ok {
			return &retryAfterError{
				Duration: wait,
				err:      fmt.Errorf("%s rate limited: %s", n.serviceName, resp.Status),
			}
		}
		return &retryableError{err: fmt.Errorf("%s rate limited: %s", n.serviceName, resp.Status)}
	}
	if resp.StatusCode >= http.StatusInternalServerError {
		return &retryableError{err: fmt.Errorf("%s server error: %s", n.serviceName, resp.Status)}
	}
	if bodyText != "" {
		return fmt.Errorf("%s request failed: %s (%s)", n.serviceName, resp.Status, bodyText)
	}
	return fmt.Errorf("%s request failed: %s", n.serviceName, resp.Status)
}

func parseRetryAfter(value string) (time.Duration, bool) {
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return 0, false
		}
		return time.Duration(seconds) * time.Second, true
	}
	if when, err := http.ParseTime(value); err == nil {
		wait := time.Until(when)
		if wait <= 0 {
			return 0, false
		}
		return wait, true
	}
	return 0, false
}

func sleepWithContext(ctx context.Context, wait time.Duration) bool {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

type retryableError struct {
	err error
}

func (e *retryableError) Error() string {
	return e.err.Error()
}

type retryAfterError struct {
	Duration time.Duration
	err      error
}

func (e *retryAfterError) Error() string {
	return fmt.Sprintf("rate limited; retry after %s", e.Duration)
}

func (e *retryAfterError) Unwrap() error {
	return e.err
}
