package httpclient

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNew(t *testing.T) {
	t.Parallel()

	headers := map[string]string{
		userAgentHeader: "custom-agent",
		"X-Custom":      "custom-value",
	}

	client := New(headers)
	if client == nil {
		t.Fatal("expected non-nil client")
	}

	if client.Timeout == 0 {
		t.Error("expected timeout to be set")
	}
}

func TestNewWithNilHeaders(t *testing.T) {
	t.Parallel()

	client := New(nil)
	if client == nil {
		t.Fatal("expected non-nil client")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get(userAgentHeader)
		if ua != "zev/1.0" {
			t.Errorf("expected User-Agent 'zev/1.0', got '%s'", ua)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("close response body: %v", err)
		}
	})
}

func TestCustomHeaders(t *testing.T) {
	t.Parallel()

	headers := map[string]string{
		userAgentHeader:     "custom-agent/2.0",
		"X-Custom":          "custom-value",
		authorizationHeader: "Bearer token123",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for key, expectedValue := range headers {
			actualValue := r.Header.Get(key)
			if actualValue != expectedValue {
				t.Errorf("header %s: expected '%s', got '%s'", key, expectedValue, actualValue)
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	client := New(headers)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("close response body: %v", err)
		}
	})
}

func TestHeaderOverride(t *testing.T) {
	t.Parallel()

	headers := map[string]string{
		userAgentHeader: "base-agent/1.0",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get(userAgentHeader)

		if ua != "base-agent/1.0" {
			t.Errorf("expected base User-Agent 'base-agent/1.0', got '%s'", ua)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	client := New(headers)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	req.Header.Set(userAgentHeader, "override-agent/1.0")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	t.Cleanup(func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("close response body: %v", err)
		}
	})
}
