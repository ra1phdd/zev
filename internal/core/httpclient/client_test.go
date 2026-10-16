package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNew(t *testing.T) {
	t.Parallel()

	headers := map[string]string{
		"User-Agent": "custom-agent",
		"X-Custom":   "custom-value",
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
		ua := r.Header.Get("User-Agent")
		if ua != "zev/1.0" {
			t.Errorf("expected User-Agent 'zev/1.0', got '%s'", ua)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
}

func TestCustomHeaders(t *testing.T) {
	t.Parallel()

	headers := map[string]string{
		"User-Agent":    "custom-agent/2.0",
		"X-Custom":      "custom-value",
		"Authorization": "Bearer token123",
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
	defer server.Close()

	client := New(headers)
	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
}

func TestHeaderOverride(t *testing.T) {
	t.Parallel()

	headers := map[string]string{
		"User-Agent": "base-agent/1.0",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")

		if ua != "base-agent/1.0" {
			t.Errorf("expected base User-Agent 'base-agent/1.0', got '%s'", ua)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(headers)
	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)

	req.Header.Set("User-Agent", "override-agent/1.0")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
}
