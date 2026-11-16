package httpclient

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestRetryAfterBounds(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		value string
		want  time.Duration
	}{
		{"0", 4 * time.Second},
		{"1", 4 * time.Second},
		{"invalid", 4 * time.Second},
		{"-1", 4 * time.Second},
		{"120", 30 * time.Second},
		{"9223372036854775807", 30 * time.Second},
	} {
		t.Run(tt.value, func(t *testing.T) {
			t.Parallel()
			resp := &http.Response{Header: http.Header{"Retry-After": {tt.value}}}
			if got := retryDelay(resp, 3); got != tt.want {
				t.Fatalf("retry delay = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestRetryAfterHTTPDate(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		resp := &http.Response{Header: http.Header{"Retry-After": {time.Now().Add(12 * time.Second).UTC().Format(http.TimeFormat)}}}
		if got := retryDelay(resp, 1); got != 12*time.Second {
			t.Fatalf("delay = %s, want 12s", got)
		}
	})
}

func TestRetryLogsDoNotLeakSecrets(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	attempts := 0
	secret := "SENSITIVE_TEST_VALUE"
	transport := newRetryTransport(roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		attempts++
		return nil, &url.Error{Op: "Post", URL: "https://example.com/?key=" + secret, Err: io.ErrUnexpectedEOF}
	}), slog.New(slog.NewTextHandler(&logs, nil)), func(_ context.Context, _ time.Duration) error { return nil })
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://example.com/private/"+secret+"?key="+secret, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := transport.RoundTrip(req)
	closeTestResponse(t, resp)
	if err == nil || attempts != 4 {
		t.Fatalf("attempts=%d error=%v", attempts, err)
	}
	if strings.Contains(logs.String(), secret) {
		t.Fatalf("logs leaked a secret: %s", logs.String())
	}
	if !strings.Contains(logs.String(), "HTTP request failed after retries") {
		t.Fatalf("missing exhaustion log: %s", logs.String())
	}
}

func TestPermanentTransportErrorsAreNotRetried(t *testing.T) {
	t.Parallel()
	attempts := 0
	cause := errors.New("unsupported protocol scheme")
	transport := newRetryTransport(roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		attempts++
		return nil, cause
	}), slog.New(slog.NewTextHandler(io.Discard, nil)), func(_ context.Context, _ time.Duration) error { return nil })
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := transport.RoundTrip(req)
	closeTestResponse(t, resp)
	if !errors.Is(err, cause) || attempts != 1 {
		t.Fatalf("attempts=%d error=%v", attempts, err)
	}
}

func TestBackoffDeadline(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
		t.Cleanup(cancel)
		start := time.Now()
		if err := sleepWithContext(ctx, time.Hour); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("error=%v", err)
		}
		if elapsed := time.Since(start); elapsed != 500*time.Millisecond {
			t.Fatalf("elapsed=%s", elapsed)
		}
	})
}

func closeTestResponse(t *testing.T, resp *http.Response) {
	t.Helper()
	if resp != nil && resp.Body != nil {
		if err := resp.Body.Close(); err != nil {
			t.Error(err)
		}
	}
}
