package client

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ra1phdd/zev/internal/entities/metadata/service"
)

func TestClientClassifiesMalformedSuccessfulResponsesAsInvalid(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		`not-json`,
		`{}`,
		`{"choices":[]}`,
		`{"choices":[{"message":{"content":""}}]}`,
		`{"choices":[{"message":{"content":"ignored"}}],"choices":[{"message":{"content":"{\"description\":\"looks valid\"}"}}]}`,
		`{"choices":[{"message":{"content":"{\"description\":\"valid\"}"}}]} {}`,
	} {
		t.Run(body, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { writeString(t, w, body) }))
			t.Cleanup(server.Close)
			_, err := newTestClient(t, server).GenerateDescription(t.Context(), service.DescriptionInput{EntityName: testEntity, EntityKind: testAliasKind, Template: testTemplate})
			if !errors.Is(err, service.ErrInvalidResponse) {
				t.Fatalf("error = %v, want invalid response", err)
			}
		})
	}
}

func TestClientClassifiesMalformedProviderEnvelopesAsInvalid(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		provider string
		body     string
	}{
		{name: ProviderOpenAICompatible, provider: ProviderOpenAICompatible, body: `{}`},
		{name: providerGemini, provider: providerGemini, body: "{\u0022candidates\u0022:[]}"},
		{name: providerAnthropic, provider: providerAnthropic, body: "{\u0022content\u0022:[]}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { writeString(t, w, tc.body) }))
			t.Cleanup(server.Close)
			client, err := New(Config{Provider: tc.provider, BaseURL: server.URL, Model: testModel}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.GenerateDescription(t.Context(), service.DescriptionInput{EntityName: testEntity, EntityKind: testAliasKind, Template: testTemplate})
			if !errors.Is(err, service.ErrInvalidResponse) {
				t.Fatalf("provider=%s error=%v, want invalid response", tc.provider, err)
			}
		})
	}
}

func TestClientClassifiesTransportFailuresAsUnavailable(t *testing.T) {
	t.Parallel()
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("transport failed")
	})}
	c, err := New(Config{Provider: ProviderOpenAICompatible, BaseURL: "https://example.test/v1", Model: testModel}, client)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.GenerateDescription(t.Context(), service.DescriptionInput{EntityName: testEntity, EntityKind: testAliasKind, Template: testTemplate})
	if !errors.Is(err, service.ErrUnavailable) {
		t.Fatalf("error = %v, want unavailable", err)
	}
}
