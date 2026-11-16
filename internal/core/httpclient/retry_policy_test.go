package httpclient

import (
	"bytes"
	"context"
	"crypto/x509"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRetryPolicyTransientFailures(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		status int
		cause  error
	}{
		{name: "request timeout", status: http.StatusRequestTimeout},
		{name: "conflict", status: http.StatusConflict},
		{name: "too early", status: http.StatusTooEarly},
		{name: "temporary DNS", cause: &net.DNSError{IsTemporary: true}},
		{name: "connection reset", cause: syscall.ECONNRESET},
		{name: "unexpected EOF", cause: io.ErrUnexpectedEOF},
		{name: "connection timeout", cause: &net.OpError{Op: "dial", Err: syscall.ETIMEDOUT}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			transport := newRetryTransport(roundTripFunc(func(_ *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					if tt.cause != nil {
						return nil, tt.cause
					}
					return &http.Response{StatusCode: tt.status, Body: http.NoBody}, nil
				}
				return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
			}), nil, func(_ context.Context, _ time.Duration) error { return nil })
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://example.com", nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := transport.RoundTrip(req)
			closeTestResponse(t, resp)
			if err != nil || resp == nil || resp.StatusCode != http.StatusOK || calls != 2 {
				t.Fatalf("response=%v error=%v calls=%d", resp, err, calls)
			}
		})
	}
}

func TestRetryPolicyPermanentFailures(t *testing.T) {
	t.Parallel()
	for _, cause := range []error{
		&net.DNSError{IsNotFound: true},
		x509.UnknownAuthorityError{},
		context.Canceled,
	} {
		if shouldRetry(nil, cause) {
			t.Errorf("permanent error %T must not be retried", cause)
		}
	}
}

func TestRetryLogExplainsFailure(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	calls := 0
	transport := newRetryTransport(roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusTooManyRequests, Body: http.NoBody}, nil
	}), slog.New(slog.NewTextHandler(&output, nil)), func(_ context.Context, _ time.Duration) error { return nil })
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := transport.RoundTrip(req)
	closeTestResponse(t, resp)
	if err != nil || calls != 4 {
		t.Fatalf("error=%v calls=%d", err, calls)
	}
	for _, want := range []string{"rate limit", "1/4", "1s", "attempts=4"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("missing %q in log: %s", want, output.String())
		}
	}
}
