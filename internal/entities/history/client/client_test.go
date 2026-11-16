package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	embeddingModel = "embedding-model"
	exampleURL     = "http://example.test"
	testModel      = "model"
)

func validConfig(baseURL string) Config {
	return Config{
		Provider:   ProviderOpenAICompatible,
		BaseURL:    baseURL,
		Model:      embeddingModel,
		APIKey:     "embedding-secret",
		Dimensions: 3,
		MaxBytes:   64,
	}
}

func boundedHTTPClient() *http.Client { return &http.Client{Timeout: time.Second} }

func serverHTTPClient(server *httptest.Server) *http.Client {
	client := server.Client()
	client.Timeout = time.Second
	return client
}

func cleanupServer(t *testing.T, server *httptest.Server) {
	t.Helper()
	t.Cleanup(server.Close)
}

// roundTripFunc keeps timeout tests local and deterministic.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func writeTestResponse(t *testing.T, w io.Writer, body string) {
	t.Helper()
	if _, err := io.WriteString(w, body); err != nil {
		t.Errorf("write test response: %v", err)
	}
}

func TestEmbedSendsOpenAICompatibleRequest(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/embeddings" {
			t.Errorf("path = %s, want /v1/embeddings", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("content type = %q, want application/json", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer embedding-secret" {
			t.Errorf("authorization = %q, want bearer token", got)
		}
		var request struct {
			Model      string `json:"model"`
			Input      string `json:"input"`
			Dimensions int    `json:"dimensions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if request.Model != embeddingModel || request.Input != "find files" || request.Dimensions != 3 {
			t.Errorf("request = %+v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		writeTestResponse(t, w, `{"data":[{"embedding":[1,2,2]}]}`)
	}))
	cleanupServer(t, server)

	c, err := New(validConfig(server.URL+"/v1"), serverHTTPClient(server))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	got, err := c.Embed(context.Background(), "find files")
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}
	want := []float32{1, 2, 2}
	if len(got) != len(want) {
		t.Fatalf("vector length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("vector[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestNewRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cfg  Config
		cli  *http.Client
	}{
		{name: "nil HTTP client", cfg: validConfig(exampleURL), cli: nil},
		{name: "unbounded HTTP client", cfg: validConfig(exampleURL), cli: &http.Client{}},
		{name: "negative HTTP timeout", cfg: validConfig(exampleURL), cli: &http.Client{Timeout: -time.Second}},
		{name: "unsupported provider", cfg: Config{Provider: "other", BaseURL: exampleURL, Model: testModel, Dimensions: 1, MaxBytes: 1}, cli: boundedHTTPClient()},
		{name: "empty endpoint", cfg: Config{Provider: ProviderOpenAICompatible, Model: testModel, Dimensions: 1, MaxBytes: 1}, cli: boundedHTTPClient()},
		{name: "empty model", cfg: Config{Provider: ProviderOpenAICompatible, BaseURL: exampleURL, Dimensions: 1, MaxBytes: 1}, cli: boundedHTTPClient()},
		{name: "zero dimensions", cfg: Config{Provider: ProviderOpenAICompatible, BaseURL: exampleURL, Model: testModel, MaxBytes: 1}, cli: boundedHTTPClient()},
		{name: "negative max bytes", cfg: Config{Provider: ProviderOpenAICompatible, BaseURL: exampleURL, Model: testModel, Dimensions: 1, MaxBytes: -1}, cli: boundedHTTPClient()},
		{name: "ftp endpoint", cfg: Config{Provider: ProviderOpenAICompatible, BaseURL: "ftp://example.test", Model: testModel, Dimensions: 1, MaxBytes: 1}, cli: boundedHTTPClient()},
		{name: "endpoint credentials", cfg: Config{Provider: ProviderOpenAICompatible, BaseURL: "http://user:pass@example.test", Model: testModel, Dimensions: 1, MaxBytes: 1}, cli: boundedHTTPClient()},
		{name: "endpoint query", cfg: Config{Provider: ProviderOpenAICompatible, BaseURL: "http://example.test/v1?key=secret", Model: testModel, Dimensions: 1, MaxBytes: 1}, cli: boundedHTTPClient()},
		{name: "endpoint fragment", cfg: Config{Provider: ProviderOpenAICompatible, BaseURL: "http://example.test/v1#secret", Model: testModel, Dimensions: 1, MaxBytes: 1}, cli: boundedHTTPClient()},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.cfg, tt.cli)
			if err == nil {
				t.Fatal("New accepted invalid configuration")
			}
			for _, secret := range []string{"secret", tt.cfg.APIKey, tt.cfg.BaseURL} {
				if secret != "" && strings.Contains(err.Error(), secret) {
					t.Fatalf("error contains configuration secret/value %q: %v", secret, err)
				}
			}
		})
	}
}

func TestEmbedRejectsInvalidInputWithoutRequest(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		text string
	}{
		{name: "oversized", text: "secret text"},
		{name: "UTF-8 byte limit", text: "аб"},
		{name: "invalid UTF-8", text: string([]byte{0xff})},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			var called atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called.Store(true) }))
			cleanupServer(t, server)
			cfg := validConfig(server.URL)
			cfg.MaxBytes = 3
			c, err := New(cfg, serverHTTPClient(server))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := c.Embed(t.Context(), tt.text); err == nil {
				t.Fatal("Embed accepted invalid input")
			} else if strings.Contains(err.Error(), tt.text) {
				t.Fatalf("error leaked input text: %v", err)
			}
			if called.Load() {
				t.Fatal("invalid input reached provider")
			}
		})
	}
}

func TestEmbedRejectsInvalidProviderResponsesSafely(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body string
		code int
	}{
		{name: "status", body: `provider secret payload`, code: http.StatusBadGateway},
		{name: "malformed", body: `{"data":[{"embedding":[1]}`, code: http.StatusOK},
		{name: "no data", body: `{"data":[]}`, code: http.StatusOK},
		{name: "multiple vectors", body: `{"data":[{"embedding":[1]},{"embedding":[1]}]}`, code: http.StatusOK},
		{name: "wrong dimensions", body: `{"data":[{"embedding":[1,2]}]}`, code: http.StatusOK},
		{name: "nonfinite", body: `{"data":[{"embedding":[1,NaN,2]}]}`, code: http.StatusOK},
		{name: "zero norm", body: `{"data":[{"embedding":[0,0,0]}]}`, code: http.StatusOK},
		{name: "response too large", body: strings.Repeat("x", (1<<20)+1), code: http.StatusOK},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.code)
				writeTestResponse(t, w, tt.body)
			}))
			cleanupServer(t, server)
			cfg := validConfig(server.URL)
			c, err := New(cfg, serverHTTPClient(server))
			if err != nil {
				t.Fatal(err)
			}
			input := "input secret"
			_, err = c.Embed(context.Background(), input)
			if err == nil {
				t.Fatal("Embed accepted invalid provider response")
			}
			for _, secret := range []string{tt.body, cfg.APIKey, cfg.BaseURL, input, "502", "200"} {
				if len(secret) > 0 && strings.Contains(err.Error(), secret) {
					t.Fatalf("error leaked %q: %v", secret, err)
				}
			}
		})
	}
}

func TestEmbedHonorsContextTimeout(t *testing.T) {
	t.Parallel()
	httpClient := &http.Client{Timeout: time.Second, Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	c, err := New(validConfig("https://example.test"), httpClient)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	t.Cleanup(cancel)
	_, err = c.Embed(ctx, "input")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Embed error = %v, want context deadline exceeded", err)
	}
}

type canceledBody struct {
	ctx     context.Context
	started chan struct{}
	closed  bool
}

func (b *canceledBody) Read([]byte) (int, error) {
	close(b.started)
	<-b.ctx.Done()
	return 0, fmt.Errorf("read canceled response: %w", b.ctx.Err())
}

func (b *canceledBody) Close() error {
	b.closed = true
	return nil
}

func TestEmbedPreservesCancellationDuringBodyRead(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	body := &canceledBody{ctx: ctx, started: make(chan struct{})}
	httpClient := &http.Client{
		Timeout: time.Second,
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body, Request: r}, nil
		}),
	}
	c, err := New(validConfig(exampleURL), httpClient)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, embedErr := c.Embed(ctx, "input")
		result <- embedErr
	}()
	select {
	case <-body.started:
	case <-time.After(5 * time.Second):
		t.Fatal("response body read did not start")
	}
	cancel()
	select {
	case err = <-result:
	case <-time.After(5 * time.Second):
		t.Fatal("Embed did not stop after cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Embed error = %v, want context canceled", err)
	}
	if !body.closed {
		t.Fatal("response body was not closed")
	}
}

func TestProfileIDSeparatesEmbeddingProfilesNotCredentials(t *testing.T) {
	t.Parallel()
	base := validConfig("https://example.test/v1")
	withKey, err := New(base, boundedHTTPClient())
	if err != nil {
		t.Fatal(err)
	}
	base.APIKey = "different-secret"
	base.MaxBytes = 128
	base.BaseURL += "/"
	withoutKey, err := New(base, boundedHTTPClient())
	if err != nil {
		t.Fatal(err)
	}
	if withKey.ProfileID() != withoutKey.ProfileID() {
		t.Fatal("credentials, request limit, or trailing slash changed profile ID")
	}

	for name, changed := range map[string]Config{
		"endpoint":   func() Config { c := base; c.BaseURL = "https://other.example/v1"; return c }(),
		"model":      func() Config { c := base; c.Model = "other"; return c }(),
		"dimensions": func() Config { c := base; c.Dimensions = 4; return c }(),
	} {
		other, err := New(changed, boundedHTTPClient())
		if err != nil {
			t.Fatalf("New %s: %v", name, err)
		}
		if withKey.ProfileID() == other.ProfileID() {
			t.Errorf("changing %s did not change profile ID", name)
		}
	}
	if withKey.Model() != embeddingModel || withKey.Dimensions() != 3 || withKey.MaxBytes() != 64 {
		t.Fatal("client metadata accessors returned unexpected values")
	}
}

func TestEmbedHTTPClientTimeout(t *testing.T) {
	t.Parallel()
	for _, flushHeaders := range []bool{false, true} {
		name := "headers"
		if flushHeaders {
			name = "body"
		}
		t.Run(name, func(t *testing.T) {
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if _, err := io.Copy(io.Discard, r.Body); err != nil {
					t.Errorf("read request: %v", err)
					return
				}
				if flushHeaders {
					w.WriteHeader(http.StatusOK)
					if err := http.NewResponseController(w).Flush(); err != nil {
						t.Errorf("flush headers: %v", err)
						return
					}
				}
				select {
				case <-r.Context().Done():
				case <-release:
				}
			}))
			cleanupServer(t, server)
			t.Cleanup(func() { close(release) })
			httpClient := serverHTTPClient(server)
			httpClient.Timeout = 50 * time.Millisecond
			c, err := New(validConfig(server.URL), httpClient)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			t.Cleanup(cancel)
			_, err = c.Embed(ctx, "secret input")
			if !errors.Is(err, ErrUnavailable) {
				t.Fatalf("Embed error = %v, want unavailable", err)
			}
			if ctx.Err() != nil {
				t.Fatalf("caller timeout expired before HTTP timeout: %v", ctx.Err())
			}
		})
	}
}

func TestEmbedBoundsResponseRead(t *testing.T) {
	t.Parallel()
	for _, size := range []int{maxResponseBytes, maxResponseBytes + 1, maxResponseBytes + 1024} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			const vectorJSON = `{"data":[{"embedding":[1,2,2]}]}`
			body := strings.NewReader(vectorJSON + strings.Repeat(" ", size-len(vectorJSON)))
			httpClient := boundedHTTPClient()
			httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(body), Request: r}, nil
			})
			c, err := New(validConfig(exampleURL), httpClient)
			if err != nil {
				t.Fatal(err)
			}
			vector, err := c.Embed(t.Context(), "input")
			if size > maxResponseBytes {
				if !errors.Is(err, ErrUnavailable) || vector != nil {
					t.Fatalf("oversized response: vector = %v, error = %v", vector, err)
				}
			} else if err != nil || len(vector) != 3 {
				t.Fatalf("response at limit: vector = %v, error = %v", vector, err)
			}
			if read := size - body.Len(); read != min(size, maxResponseBytes+1) {
				t.Fatalf("read %d bytes, want %d", read, min(size, maxResponseBytes+1))
			}
		})
	}
}

func TestEmbedRejectsNilContext(t *testing.T) {
	t.Parallel()
	var called atomic.Bool
	httpClient := boundedHTTPClient()
	httpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		called.Store(true)
		return nil, errors.New("provider secret")
	})
	c, err := New(validConfig(exampleURL), httpClient)
	if err != nil {
		t.Fatal(err)
	}
	var ctx context.Context
	if _, err := c.Embed(ctx, "input"); err == nil {
		t.Fatal("Embed accepted nil context")
	}
	if called.Load() {
		t.Fatal("nil context reached provider")
	}
}

func TestEmbedRejectsFloat32Overflow(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeTestResponse(t, w, `{"data":[{"embedding":[1e+40,1,1]}]}`)
	}))
	cleanupServer(t, server)
	c, err := New(validConfig(server.URL), serverHTTPClient(server))
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Embed(context.Background(), "input")
	if err == nil {
		t.Fatal("Embed accepted float32 overflow response")
	}
}
