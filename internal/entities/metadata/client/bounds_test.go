package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/entities/metadata/service"
)

const (
	validCompletion   = `{"choices":[{"message":{"content":"generated"}}]}`
	testGenerated     = "generated"
	testPrivateAPIKey = "private-api-key"
	testAPIKey        = "secret"
)

func TestNewRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		change    func(*Config)
		nilClient bool
	}{
		{name: "nil client", nilClient: true},
		{name: "unsupported provider", change: func(cfg *Config) { cfg.Provider = "private-provider" }},
		{name: "empty provider", change: func(cfg *Config) { cfg.Provider = "" }},
		{name: "empty model", change: func(cfg *Config) { cfg.Model = "" }},
		{name: "blank model", change: func(cfg *Config) { cfg.Model = " \t\n" }},
		{name: "empty URL", change: func(cfg *Config) { cfg.BaseURL = "" }},
		{name: "relative URL", change: func(cfg *Config) { cfg.BaseURL = "/v1" }},
		{name: "missing host", change: func(cfg *Config) { cfg.BaseURL = "https:///v1" }},
		{name: "unsupported scheme", change: func(cfg *Config) { cfg.BaseURL = "ftp://private.example/v1" }},
		{name: "URL userinfo", change: func(cfg *Config) { cfg.BaseURL = "https://user:secret@private.example/v1" }},
		{name: "URL query", change: func(cfg *Config) { cfg.BaseURL = "https://private.example/v1?token=secret" }},
		{name: "empty URL query", change: func(cfg *Config) { cfg.BaseURL = "https://private.example/v1?" }},
		{name: "URL fragment", change: func(cfg *Config) { cfg.BaseURL = "https://private.example/v1#secret" }},
		{name: "opaque URL", change: func(cfg *Config) { cfg.BaseURL = "https:private.example" }},
		{name: "malformed escape", change: func(cfg *Config) { cfg.BaseURL = "https://private.example/%zz" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := Config{Provider: ProviderOpenAICompatible, BaseURL: "https://private.example/v1", Model: testModel, APIKey: testPrivateAPIKey}
			if tc.change != nil {
				tc.change(&cfg)
			}
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Error("constructor performed HTTP I/O")
				return nil, service.ErrUnavailable
			})}
			if tc.nilClient {
				client = nil
			}
			got, err := New(cfg, client)
			if err == nil || got != nil {
				t.Fatalf("New = %#v, %v; want nil and error", got, err)
			}
			for _, secret := range []string{"private-provider", "private.example", testPrivateAPIKey, testAPIKey} {
				if strings.Contains(err.Error(), secret) {
					t.Errorf("configuration leaked: %v", err)
				}
			}
		})
	}
}

func TestReadCompletionResponseClosesAndClassifies(t *testing.T) {
	t.Parallel()
	failure := errors.New("private body failure")
	for _, tc := range []struct {
		name     string
		status   int
		body     string
		readErr  error
		closeErr error
		want     string
		wantErr  error
		wantRead bool
	}{
		{name: "success", status: http.StatusOK, body: validCompletion, want: testGenerated, wantRead: true},
		{name: "last success status", status: http.StatusMultipleChoices - 1, body: validCompletion, want: testGenerated, wantRead: true},
		{name: "informational", status: http.StatusContinue, body: validCompletion, wantErr: service.ErrUnavailable},
		{name: "redirect", status: http.StatusMultipleChoices, body: validCompletion, wantErr: service.ErrUnavailable},
		{name: "server failure", status: http.StatusInternalServerError, body: "private payload", wantErr: service.ErrUnavailable},
		{name: "status close failure", status: http.StatusUnauthorized, body: validCompletion, closeErr: failure, wantErr: service.ErrUnavailable},
		{name: "partial read failure", status: http.StatusOK, body: validCompletion, readErr: failure, wantErr: service.ErrUnavailable, wantRead: true},
		{name: "close failure", status: http.StatusOK, body: validCompletion, closeErr: failure, wantErr: service.ErrUnavailable, wantRead: true},
		{name: "read and close failure", status: http.StatusOK, body: validCompletion, readErr: failure, closeErr: failure, wantErr: service.ErrUnavailable, wantRead: true},
		{name: "malformed JSON", status: http.StatusOK, body: "private payload", wantErr: service.ErrInvalidResponse, wantRead: true},
		{name: "invalid UTF8", status: http.StatusOK, body: `{"choices":[{"message":{"content":"` + string([]byte{0xff}) + `"}}]}`, wantErr: service.ErrInvalidResponse, wantRead: true},
		{name: "exact size", status: http.StatusOK, body: validCompletion + strings.Repeat(" ", service.MaxGenerationBytes-len(validCompletion)), want: testGenerated, wantRead: true},
		{name: "size plus one", status: http.StatusOK, body: validCompletion + strings.Repeat(" ", service.MaxGenerationBytes-len(validCompletion)+1), wantErr: service.ErrInvalidResponse, wantRead: true},
		{name: "bounded read", status: http.StatusOK, body: strings.Repeat("x", 2*service.MaxGenerationBytes), wantErr: service.ErrInvalidResponse, wantRead: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reader := strings.NewReader(tc.body)
			body := &trackedResponseBody{reader: reader, readErr: tc.readErr, closeErr: tc.closeErr}
			c := &Client{provider: ProviderOpenAICompatible}
			got, err := c.readCompletionResponse(t.Context(), &http.Response{StatusCode: tc.status, Body: body})
			if got != tc.want || !errors.Is(err, tc.wantErr) {
				t.Errorf("readCompletionResponse = %q, %v; want %q, %v", got, err, tc.want, tc.wantErr)
			}
			if body.closes != 1 || (body.reads > 0) != tc.wantRead {
				t.Errorf("body reads=%d closes=%d; want read=%t and exactly one close", body.reads, body.closes, tc.wantRead)
			}
			wantBytes := 0
			if tc.wantRead {
				wantBytes = min(len(tc.body), service.MaxGenerationBytes+1)
			}
			if body.bytesRead != wantBytes {
				t.Errorf("read %d bytes, want %d", body.bytesRead, wantBytes)
			}
			if err != nil && (strings.Contains(err.Error(), "private") || errors.Is(err, failure)) {
				t.Errorf("response error exposed body failure: %v", err)
			}
		})
	}
}

func TestReadCompletionResponseRejectsMissingBody(t *testing.T) {
	t.Parallel()
	for _, resp := range []*http.Response{nil, {StatusCode: http.StatusOK}} {
		got, err := (&Client{}).readCompletionResponse(t.Context(), resp)
		if got != "" || !errors.Is(err, service.ErrUnavailable) {
			t.Errorf("readCompletionResponse = %q, %v; want unavailable", got, err)
		}
	}
}

func TestReadCompletionResponseMidReadCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	body := &trackedResponseBody{reader: io.LimitReader(strings.NewReader(validCompletion), int64(len(validCompletion)/2)), readErr: errors.New("private interrupted read"), cancel: cancel}
	got, err := (&Client{}).readCompletionResponse(ctx, &http.Response{StatusCode: http.StatusOK, Body: body})
	if got != "" || !errors.Is(err, context.Canceled) || errors.Is(err, service.ErrUnavailable) {
		t.Errorf("readCompletionResponse = %q, %v; want cancellation", got, err)
	}
	if body.reads != 2 || body.bytesRead != len(validCompletion)/2 || body.closes != 1 {
		t.Errorf("mid-read lifecycle = %#v", body)
	}
	if err != nil && strings.Contains(err.Error(), "private") {
		t.Errorf("cancellation leaked read failure: %v", err)
	}
}

type trackedResponseBody struct {
	reader    io.Reader
	readErr   error
	closeErr  error
	cancel    context.CancelFunc
	reads     int
	bytesRead int
	closes    int
}

func (b *trackedResponseBody) Read(p []byte) (int, error) {
	b.reads++
	if b.cancel != nil && b.reads > 1 {
		return 0, b.readErr
	}
	n, err := b.reader.Read(p)
	b.bytesRead += n
	if b.cancel != nil {
		b.cancel()
		return n, nil
	}
	if b.readErr != nil {
		return n, b.readErr
	}
	if errors.Is(err, io.EOF) {
		return n, io.EOF
	}
	if err != nil {
		return n, fmt.Errorf("read fixture: %w", err)
	}
	return n, nil
}

func (b *trackedResponseBody) Close() error {
	b.closes++
	return b.closeErr
}

func TestCompleteValidatesPromptAndEncodedRequestBounds(t *testing.T) {
	t.Parallel()
	c := &Client{provider: ProviderOpenAICompatible, model: testModel}
	empty, _, err := c.requestPayload("")
	if err != nil {
		t.Fatal(err)
	}
	maxPrompt := strings.Repeat("x", service.MaxGenerationBytes-len(empty))
	for _, tc := range []struct {
		name       string
		prompt     string
		nilContext bool
		wantErr    error
	}{
		{name: "exact encoded boundary", prompt: maxPrompt},
		{name: "encoded boundary plus one", prompt: maxPrompt + "x", wantErr: service.ErrInvalidInput},
		{name: "raw boundary", prompt: strings.Repeat("x", service.MaxGenerationBytes), wantErr: service.ErrInvalidInput},
		{name: "raw boundary plus one", prompt: strings.Repeat("x", service.MaxGenerationBytes+1), wantErr: service.ErrInvalidInput},
		{name: "escaping expands payload", prompt: strings.Repeat("<", service.MaxGenerationBytes/2), wantErr: service.ErrInvalidInput},
		{name: "invalid UTF8", prompt: string([]byte{0xff}), wantErr: service.ErrInvalidInput},
		{name: "nil context", prompt: "valid", nilContext: true, wantErr: service.ErrInvalidInput},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			client, newErr := New(Config{Provider: ProviderOpenAICompatible, BaseURL: "https://bounded.example/v1", Model: testModel}, &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				data, readErr := io.ReadAll(r.Body)
				if readErr != nil || len(data) != service.MaxGenerationBytes {
					t.Errorf("encoded request size = %d, %v; want %d", len(data), readErr, service.MaxGenerationBytes)
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(validCompletion))}, nil
			})})
			if newErr != nil {
				t.Fatal(newErr)
			}
			ctx := t.Context()
			if tc.nilContext {
				ctx = nil
			}
			got, completeErr := client.complete(ctx, tc.prompt)
			if !errors.Is(completeErr, tc.wantErr) {
				t.Errorf("complete error = %v, want %v", completeErr, tc.wantErr)
			}
			wantCalls, want := 0, ""
			if tc.wantErr == nil {
				wantCalls, want = 1, testGenerated
			}
			if calls != wantCalls || got != want {
				t.Errorf("complete = %q, calls=%d; want %q, calls=%d", got, calls, want, wantCalls)
			}
		})
	}
}
