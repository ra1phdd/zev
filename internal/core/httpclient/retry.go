package httpclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	maxRetries     = 3
	initialBackoff = time.Second
	maxBackoff     = 30 * time.Second
)

type retryTransport struct {
	base   http.RoundTripper
	logger *slog.Logger
	sleep  func(context.Context, time.Duration) error
}

func newRetryTransport(base http.RoundTripper, logger *slog.Logger, sleep func(context.Context, time.Duration) error) *retryTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	if sleep == nil {
		sleep = sleepWithContext
	}
	return &retryTransport{base: base, logger: logger, sleep: sleep}
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		if err := req.Context().Err(); err != nil {
			return nil, fmt.Errorf("request context ended: %w", err)
		}
		current, err := requestForAttempt(req, attempt)
		if err != nil {
			return nil, err
		}
		resp, err := t.base.RoundTrip(current)
		retryable := shouldRetry(resp, err)
		if !retryable || attempt == maxRetries || !replayable(req) {
			if retryable && attempt == maxRetries {
				t.logger.ErrorContext(req.Context(), "HTTP request failed after retries",
					"attempts", attempt+1,
					"max_attempts", maxRetries+1,
					"status", responseStatus(resp),
					"reason", retryReason(resp, err),
				)
			}
			return resp, wrapRoundTripError(err)
		}
		delay := retryDelay(resp, attempt+1)
		t.discardResponse(resp)
		// Do not log URLs, headers, bodies or raw errors: these can contain API keys.
		t.logger.WarnContext(req.Context(), "retrying HTTP request",
			"attempt", fmt.Sprintf("%d/%d", attempt+1, maxRetries+1),
			"status", responseStatus(resp),
			"reason", retryReason(resp, err),
			"delay", delay,
		)
		if err := t.sleep(req.Context(), delay); err != nil {
			return nil, fmt.Errorf("waiting before retry: %w", err)
		}
	}
}

func replayable(req *http.Request) bool {
	return req.Body == nil || req.Body == http.NoBody || req.GetBody != nil
}

func requestForAttempt(req *http.Request, attempt int) (*http.Request, error) {
	if attempt == 0 {
		return req, nil
	}
	clone := req.Clone(req.Context())
	if req.Body == nil || req.Body == http.NoBody {
		return clone, nil
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, fmt.Errorf("recreating request body: %w", err)
	}
	clone.Body = body
	return clone, nil
}

func (t *retryTransport) discardResponse(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	// Close without unbounded draining: a failing upstream may stall its body.
	if err := resp.Body.Close(); err != nil {
		t.logger.Debug("failed to close retry response body")
	}
}

func wrapRoundTripError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("HTTP round trip: %w", err)
}

func shouldRetry(resp *http.Response, err error) bool {
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.EPIPE) {
			return true
		}
		if dnsErr, ok := errors.AsType[*net.DNSError](err); ok {
			return dnsErr.IsTemporary || dnsErr.IsTimeout
		}
		if netErr, ok := errors.AsType[net.Error](err); ok {
			return netErr.Timeout()
		}
		return false
	}
	return resp != nil && (resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode == http.StatusConflict || resp.StatusCode == http.StatusTooEarly || resp.StatusCode == http.StatusTooManyRequests || (resp.StatusCode >= 500 && resp.StatusCode <= 599))
}

func retryReason(resp *http.Response, err error) string {
	if resp != nil {
		switch resp.StatusCode {
		case http.StatusRequestTimeout:
			return "request timeout"
		case http.StatusConflict:
			return "request conflict"
		case http.StatusTooEarly:
			return "request not ready"
		case http.StatusTooManyRequests:
			return "rate limit or quota"
		case http.StatusInternalServerError, http.StatusNotImplemented, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return "provider server error"
		default:
			if resp.StatusCode >= 500 && resp.StatusCode <= 599 {
				return "provider server error"
			}
		}
	}
	if err != nil {
		return "temporary network error"
	}
	return "temporary provider error"
}

func retryDelay(resp *http.Response, attempt int) time.Duration {
	delay := max(initialBackoff<<(attempt-1), retryAfter(resp))
	return min(delay, maxBackoff)
}

func retryAfter(resp *http.Response) time.Duration {
	if resp == nil {
		return 0
	}
	if after, ok := parseRetryAfter(resp.Header.Get("Retry-After")); ok {
		return after
	}
	return 0
}

func parseRetryAfter(value string) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.ParseUint(value, 10, 64); err == nil {
		const maxDuration = time.Duration(1<<63 - 1)
		if seconds > uint64(maxDuration/time.Second) {
			return maxDuration, true
		}
		return time.Duration(seconds) * time.Second, true
	}
	when, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	return max(time.Until(when), 0), true
}

func responseStatus(resp *http.Response) int {
	if resp == nil {
		return 0
	}
	return resp.StatusCode
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("retry canceled: %w", err)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("retry canceled: %w", ctx.Err())
	}
}
