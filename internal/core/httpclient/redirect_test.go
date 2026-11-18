package httpclient

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const authorizationHeader = "Authorization"

func TestCrossOriginRedirectDifferentPortIsBlockedBeforeContact(t *testing.T) {
	t.Parallel()
	var targetRequests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetRequests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(target.Close)

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", target.URL+"/secret-target")
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(source.Close)

	const secret = "Bearer redirect-secret"
	client := New(map[string]string{authorizationHeader: secret})
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, source.URL+"/start", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	closeTestResponse(t, resp)
	if !errors.Is(err, ErrCrossOriginRedirect) {
		t.Fatalf("error = %v, want ErrCrossOriginRedirect", err)
	}
	if got := targetRequests.Load(); got != 0 {
		t.Fatalf("cross-origin target requests = %d, want 0", got)
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), target.URL) {
		t.Fatalf("redirect error leaked sensitive details: %v", err)
	}
}

func TestCrossOriginRedirectDifferentHostIsBlockedBeforeContact(t *testing.T) {
	t.Parallel()
	var targetRequests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetRequests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(target.Close)
	targetURL := strings.Replace(target.URL, "127.0.0.1", "localhost", 1) + "/secret-target"

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", targetURL)
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(source.Close)

	client := New(map[string]string{authorizationHeader: "Bearer redirect-secret"})
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, source.URL+"/start", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	closeTestResponse(t, resp)
	if !errors.Is(err, ErrCrossOriginRedirect) {
		t.Fatalf("error = %v, want ErrCrossOriginRedirect", err)
	}
	if got := targetRequests.Load(); got != 0 {
		t.Fatalf("cross-origin target requests = %d, want 0", got)
	}
}

func TestRedirectResponseRetainsConfiguredHeaders(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			if got := r.Header.Get(authorizationHeader); got != "Bearer same-origin" {
				t.Errorf("Authorization = %q, want configured header", got)
			}
			w.Header().Set("Location", "/next")
			w.WriteHeader(http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := New(map[string]string{authorizationHeader: "Bearer same-origin"})
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/start", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() { closeTestResponse(t, resp) })
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusFound)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("same-origin requests = %d, want 1", got)
	}
}

func TestSameOrigin307RedirectIsNotFollowed(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/start" {
			w.Header().Set("Location", "/redirected")
			w.WriteHeader(http.StatusTemporaryRedirect)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := New(map[string]string{authorizationHeader: "Bearer exact-request"})
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/start", strings.NewReader("body"))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() { closeTestResponse(t, resp) })
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusTemporaryRedirect)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("redirect requests = %d, want 1", got)
	}
}

func TestRedirectResponsesAreNotFollowed(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Location", r.URL.Path)
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(server.Close)

	client := New(nil)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/loop", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() { closeTestResponse(t, resp) })
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusFound)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("redirect requests = %d, want 1", got)
	}
}

func TestNewSnapshotsCallerHeaders(t *testing.T) {
	t.Parallel()
	headers := map[string]string{"X-Snapshot": "before"}
	client := New(headers)
	if _, ok := headers["User-Agent"]; ok {
		t.Fatal("New mutated caller headers with default User-Agent")
	}
	headers["X-Snapshot"] = "after"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Snapshot"); got != "before" {
			t.Errorf("X-Snapshot = %q, want snapshot value", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeTestResponse(t, resp) })
}
