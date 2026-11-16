package httpclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestClientCancellationInterruptsWaitingHTTP(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	stopped := make(chan struct{})
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		close(started)
		<-r.Context().Done()
		close(stopped)
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		resp, requestErr := New(nil).Do(req)
		closeTestResponse(t, resp)
		done <- requestErr
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("HTTP request did not reach server")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Do() error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("HTTP request did not stop after cancellation")
	}
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("server did not observe request cancellation")
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("attempts = %d, want 1", got)
	}
}

func TestRetryCancellationInterruptsActiveBackoff(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)
		attempts := 0
		transport := newRetryTransport(roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			attempts++
			return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: http.NoBody}, nil
		}), nil, nil)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.com", nil)
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() {
			resp, requestErr := transport.RoundTrip(req)
			closeTestResponse(t, resp)
			done <- requestErr
		}()
		synctest.Wait()
		if attempts != 1 {
			t.Fatalf("attempts = %d, want one request waiting in backoff", attempts)
		}
		cancel()
		synctest.Wait()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("RoundTrip() error = %v, want context.Canceled", err)
			}
		default:
			t.Fatal("backoff did not stop after cancellation")
		}
		if attempts != 1 {
			t.Fatalf("attempts = %d, want no retry after cancellation", attempts)
		}
	})
}
