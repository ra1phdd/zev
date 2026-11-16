package httpclient

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRetryTransportRetriesRateLimitAndServerErrors(t *testing.T) {
	t.Parallel()
	var attempts int
	var delays []time.Duration
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		if err := req.Body.Close(); err != nil {
			t.Fatal(err)
		}
		if string(body) != "request" {
			t.Errorf("body = %q", body)
		}
		statuses := []int{429, 500, 502, 200}
		return &http.Response{StatusCode: statuses[attempts-1], Header: make(http.Header), Body: io.NopCloser(strings.NewReader("response"))}, nil
	})
	transport := newRetryTransport(base, logger, func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return nil
	})
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://example.com", strings.NewReader("request"))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := resp.Body.Close(); err != nil {
			t.Error(err)
		}
	})
	if resp.StatusCode != http.StatusOK || attempts != 4 {
		t.Fatalf("status = %d, attempts = %d", resp.StatusCode, attempts)
	}
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
	if len(delays) != len(want) {
		t.Fatalf("delays = %v", delays)
	}
	for i := range want {
		if delays[i] != want[i] {
			t.Errorf("delay[%d] = %s, want %s", i, delays[i], want[i])
		}
	}
	for _, want := range []string{"retrying HTTP request", "status=429", "status=500", "status=502"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("logs missing %q: %s", want, logs.String())
		}
	}
}

func TestRetryTransportUsesRetryAfterHeader(t *testing.T) {
	t.Parallel()
	var delays []time.Duration
	attempts := 0
	transport := newRetryTransport(roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		attempts++
		resp := &http.Response{StatusCode: http.StatusTooManyRequests, Header: make(http.Header), Body: http.NoBody}
		if attempts == 1 {
			resp.Header.Set("Retry-After", "7")
		} else {
			resp.StatusCode = http.StatusOK
		}
		return resp, nil
	}), slog.Default(), func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return nil
	})
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := resp.Body.Close(); err != nil {
			t.Error(err)
		}
	})
	if resp.StatusCode != http.StatusOK || len(delays) != 1 || delays[0] != 7*time.Second {
		t.Fatalf("status=%d delays=%v", resp.StatusCode, delays)
	}
}

func TestRetryTransportStopsAfterThreeRetries(t *testing.T) {
	t.Parallel()
	attempts, delays := 0, 0
	transport := newRetryTransport(roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		attempts++
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("last failure"))}, nil
	}), slog.New(slog.NewTextHandler(io.Discard, nil)), func(_ context.Context, _ time.Duration) error {
		delays++
		return nil
	})
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := resp.Body.Close(); err != nil {
			t.Error(err)
		}
	})
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 4 || delays != 3 || resp.StatusCode != http.StatusServiceUnavailable || string(body) != "last failure" {
		t.Fatalf("attempts=%d delays=%d status=%d body=%q", attempts, delays, resp.StatusCode, body)
	}
}

func TestRetryTransportDoesNotRetryClientErrors(t *testing.T) {
	t.Parallel()
	for _, status := range []int{200, 400, 401, 403, 404, 422} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			attempts := 0
			transport := newRetryTransport(roundTripFunc(func(_ *http.Request) (*http.Response, error) {
				attempts++
				return &http.Response{StatusCode: status, Body: http.NoBody}, nil
			}), slog.Default(), func(_ context.Context, _ time.Duration) error {
				t.Fatal("unexpected retry")
				return nil
			})
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com", nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := transport.RoundTrip(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := resp.Body.Close(); err != nil {
					t.Error(err)
				}
			}()
			if attempts != 1 {
				t.Fatalf("attempts = %d", attempts)
			}
		})
	}
}

func TestRetryTransportRespectsContextCancellationDuringBackoff(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	attempts := 0
	transport := newRetryTransport(roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		attempts++
		cancel()
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: http.NoBody}, nil
	}), slog.Default(), sleepWithContext)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := transport.RoundTrip(req)
	if resp != nil {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}
	if !errors.Is(err, context.Canceled) || attempts != 1 {
		t.Fatalf("error = %v, attempts = %d", err, attempts)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
