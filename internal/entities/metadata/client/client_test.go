package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/entities/metadata/service"
)

const (
	testModel     = "metadata-model"
	testAliasKind = "alias"
	testEntity    = "deploy"
	testTemplate  = "echo"
)

func TestClientDescriptionRequestIsolatedAndFieldSpecific(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Method != http.MethodPost {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
			http.Error(w, "read request", http.StatusBadRequest)
			return
		}
		text := string(data)
		for _, forbidden := range []string{"old history", "HOME=/secret", "tool output", "unrelated.txt", "api-key-in-input"} {
			if strings.Contains(text, forbidden) {
				t.Errorf("request leaked %q: %s", forbidden, text)
			}
		}
		var request struct {
			Model    string                           `json:"model"`
			Messages []struct{ Role, Content string } `json:"messages"`
		}
		if err := json.Unmarshal(data, &request); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "decode request", http.StatusBadRequest)
			return
		}
		if request.Model != testModel || len(request.Messages) != 2 || request.Messages[0].Role != "system" || request.Messages[1].Role != "user" {
			t.Errorf("request envelope = %#v", request)
			http.Error(w, "request envelope", http.StatusBadRequest)
			return
		}
		if !strings.Contains(request.Messages[1].Content, `"entity_name":"deploy"`) || !strings.Contains(request.Messages[1].Content, `"template":"kubectl get {{.arg1}}"`) {
			t.Errorf("user message omitted allowed fields: %q", request.Messages[1].Content)
			http.Error(w, "user message", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		writeString(t, w, `{"choices":[{"message":{"content":"{\"description\":\"lists resources\"}"}}]}`)
	}))
	t.Cleanup(server.Close)
	c := newTestClient(t, server)
	got, err := c.GenerateDescription(t.Context(), service.DescriptionInput{
		EntityName: testEntity, EntityKind: testAliasKind, Template: "kubectl get {{.arg1}}",
		Arguments: []domain.Argument{{Name: "resource", Required: true}},
	})
	if err != nil || got != "lists resources" {
		t.Fatalf("GenerateDescription = %q, %v", got, err)
	}
}

func TestClientArgumentsResponseValidationAndFieldSpecificRequest(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		content string
		want    []domain.Argument
		wantErr error
	}{
		{name: "full values", content: `{"arguments":[{"name":"path","description":"file","required":true},{"name":"format","description":"output format","required":false}]}`, want: []domain.Argument{{Name: "path", Description: "file", Required: true}, {Name: "format", Description: "output format", Required: false}}},
		{name: "wrong field", content: `{"description":"wrong field"}`, wantErr: service.ErrInvalidResponse},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body, marshalErr := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": tc.content}}}})
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request chatRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Errorf("decode request: %v", err)
					http.Error(w, "decode request", http.StatusBadRequest)
					return
				}
				if request.Model != testModel || request.ResponseFormat.Type != "json_object" || len(request.Messages) != 2 {
					t.Errorf("request envelope = %#v", request)
					http.Error(w, "request envelope", http.StatusBadRequest)
					return
				}
				wantPrompt := "Return exactly one JSON object with no unknown fields, nulls, prose, or trailing data. The object must be {\"arguments\": array} with no more than 32 items; each item has name string, optional description string, and optional required boolean.\nfield=arguments\ninput=" + `{"entity_name":"backup","entity_kind":"script","body":"echo backup","description":"already supplied"}`
				if request.Messages[0].Role != "system" || request.Messages[1].Role != roleUser || request.Messages[1].Content != wantPrompt {
					t.Errorf("messages = %#v, want exact arguments prompt %q", request.Messages, wantPrompt)
				}
				w.Header().Set("Content-Type", "application/json")
				writeBytes(t, w, body)
			}))
			t.Cleanup(server.Close)
			got, err := newTestClient(t, server).GenerateArguments(t.Context(), service.ArgumentsInput{
				EntityName: "backup", EntityKind: "script", Body: "echo backup", Description: "already supplied",
			})
			if !errors.Is(err, tc.wantErr) || !slices.Equal(got, tc.want) {
				t.Fatalf("GenerateArguments = %#v, %v; want %#v, %v", got, err, tc.want, tc.wantErr)
			}
		})
	}
}

func TestClientRejectsOversizedInputAndResponse(t *testing.T) {
	t.Parallel()
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		writeString(t, w, strings.Repeat("x", service.MaxGenerationBytes+1))
	}))
	t.Cleanup(server.Close)
	c := newTestClient(t, server)
	input := service.ArgumentsInput{EntityName: "backup", EntityKind: "script", Body: strings.Repeat("x", service.MaxGenerationBytes), Description: "ok"}
	if _, err := c.GenerateArguments(t.Context(), input); err == nil {
		t.Fatal("oversized request was accepted")
	}
	if calls != 0 {
		t.Fatal("invalid request reached provider")
	}
	if _, err := c.GenerateDescription(t.Context(), service.DescriptionInput{EntityName: testEntity, EntityKind: testAliasKind, Template: "echo {{.arg1}}"}); err == nil {
		t.Fatal("oversized response was accepted")
	}
}

func TestClientHonorsCancellation(t *testing.T) {
	t.Parallel()
	entered := make(chan struct{})
	requestCanceled := make(chan struct{})
	release := make(chan struct{})
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		close(entered)
		select {
		case <-r.Context().Done():
			close(requestCanceled)
			return nil, r.Context().Err()
		case <-release:
			return nil, errors.New("transport cleanup")
		}
	})
	// An HTTP timeout must not substitute for caller cancellation.
	client, err := New(Config{Provider: ProviderOpenAICompatible, BaseURL: "https://example.test/v1", Model: testModel, APIKey: "secret"}, &http.Client{Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	result := make(chan error, 1)
	finished := make(chan struct{})
	t.Cleanup(func() {
		close(release)
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Error("cancellation request goroutine did not exit")
		}
	})
	go func() {
		defer close(finished)
		_, requestErr := client.GenerateDescription(ctx, service.DescriptionInput{EntityName: testEntity, EntityKind: testAliasKind, Template: testTemplate})
		result <- requestErr
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("transport was not entered")
	}
	cancel()
	select {
	case <-requestCanceled:
	case <-time.After(time.Second):
		t.Error("request context did not observe cancellation")
	}
	select {
	case err = <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context canceled", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("cancellation result exceeded independent timeout")
	}
}

func writeString(t *testing.T, w io.Writer, value string) {
	t.Helper()
	if _, err := io.WriteString(w, value); err != nil {
		t.Errorf("write response: %v", err)
	}
}

func writeBytes(t *testing.T, w io.Writer, value []byte) {
	t.Helper()
	if _, err := w.Write(value); err != nil {
		t.Errorf("write response: %v", err)
	}
}

func newTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	client := server.Client()
	client.Timeout = time.Second
	got, err := New(Config{Provider: ProviderOpenAICompatible, BaseURL: server.URL + "/v1", Model: testModel, APIKey: "secret"}, client)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
