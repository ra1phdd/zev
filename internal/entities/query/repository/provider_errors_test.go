package repository_test

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"

	anthropicsdk "github.com/liushuangls/go-anthropic/v2"
	openaisdk "github.com/sashabaranov/go-openai"

	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/entities/query/repository"
	anthropicprovider "github.com/ra1phdd/zev/internal/entities/query/repository/anthropic"
	"github.com/ra1phdd/zev/internal/entities/query/repository/gemini"
	openaiprovider "github.com/ra1phdd/zev/internal/entities/query/repository/openai"
	"github.com/ra1phdd/zev/internal/entities/query/repository/opencode"
)

const (
	testOpenAI        = "OpenAI"
	testGemini        = "Gemini"
	testOpenCode      = "OpenCode"
	testHTTPS         = "https://"
	testPost          = "Post"
	testAPIKey        = "API key"
	testUnavailable   = "temporarily unavailable"
	testBilling       = "billing"
	testSettings      = "provider settings"
	testInvalid       = "invalid request"
	testTLSVerify     = "TLS certificate verification failed"
	testPrivatePrompt = "private " + "prompt"
	testPrivateHost   = "private." + "example"
	testSecretKey     = "secret" + "-key"
)

func newErrorTestProvider(t *testing.T, name, baseURL string, client *http.Client) domain.InferenceProvider {
	t.Helper()
	var p domain.InferenceProvider
	var err error
	switch name {
	case testOpenAI:
		p, err = openaiprovider.New(testSecretKey, "test-model", baseURL, client)
	case "Anthropic":
		p, err = anthropicprovider.New(testSecretKey, "test-model", baseURL, client)
	case testGemini:
		p, err = gemini.New(testSecretKey, "test-model", baseURL, client)
	case testOpenCode:
		p, err = opencode.New(testSecretKey, "test-model", baseURL, client)
	default:
		t.Fatalf("unknown test provider %q", name)
	}
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestProviderInvalidResponsesHideContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{name: testOpenAI, body: `{"choices":[{"message":{"content":"secret-key private prompt https://private.example"}}]}`},
		{name: "Anthropic", body: `{"content":[{"type":"text","text":"secret-key private prompt https://private.example"}]}`},
		{name: testGemini, body: `{"candidates":[{"content":{"parts":[{"text":"secret-key private prompt https://private.example"}]}}]}`},
		{name: testOpenCode, body: "data: " + `{"choices":[{"delta":{"content":"secret-key private prompt https://private.example"}}]}` + "\n\ndata: [DONE]\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := &http.Client{Transport: providerErrorTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tt.body)), Header: make(http.Header)}, nil
			})}
			p := newErrorTestProvider(t, tt.name, "https://private.example", client)
			for _, history := range []bool{false, true} {
				var got error
				if history {
					_, got = p.GetOptionsWithHistory(t.Context(), &domain.ConversationHistory{})
				} else {
					_, got = p.GetOptions(t.Context(), "prompt", "env")
				}
				if got == nil {
					t.Fatal("expected invalid response error")
				}
				for _, want := range []string{"invalid response", "model"} {
					if !strings.Contains(got.Error(), want) {
						t.Errorf("history=%t error=%q, want %q", history, got, want)
					}
				}
				for _, secret := range []string{testSecretKey, testPrivatePrompt, testHTTPS, testPrivateHost} {
					if strings.Contains(got.Error(), secret) {
						t.Errorf("history=%t error exposes %q: %s", history, secret, got)
					}
				}
				if syntaxErr, ok := errors.AsType[*json.SyntaxError](got); !ok || syntaxErr == nil {
					t.Error("original JSON syntax error lost")
				}
			}
		})
	}
}

func TestWrapResponseError(t *testing.T) {
	t.Parallel()

	cause := &json.UnmarshalTypeError{Value: testSecretKey, Field: testPrivatePrompt}
	got := repository.WrapResponseError("TestProvider", cause)
	if got == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"TestProvider", "invalid response", "model"} {
		if !strings.Contains(got.Error(), want) {
			t.Errorf("error = %q, want %q", got, want)
		}
	}
	for _, secret := range []string{testSecretKey, testPrivatePrompt} {
		if strings.Contains(got.Error(), secret) {
			t.Errorf("error exposes %q", secret)
		}
	}
	if !errors.Is(got, cause) {
		t.Error("original cause lost")
	}
	if typed, ok := errors.AsType[*json.UnmarshalTypeError](got); !ok || typed != cause {
		t.Error("original type lost")
	}
	if repository.WrapResponseError("TestProvider", nil) != nil {
		t.Error("nil cause must return nil")
	}
}

func TestSDKProviderErrors(t *testing.T) {
	t.Parallel()

	const secret = "secret-key https://private.example raw prompt"
	tests := []struct {
		name  string
		cause error
		want  []string
	}{
		{name: "OpenAI authentication", cause: &openaisdk.APIError{HTTPStatusCode: 401, Message: secret}, want: []string{"HTTP 401", testAPIKey}},
		{name: "OpenAI request", cause: &openaisdk.RequestError{HTTPStatusCode: 503, Err: errors.New(secret), Body: []byte(secret)}, want: []string{"HTTP 503", testUnavailable}},
		{name: "OpenAI quota", cause: &openaisdk.APIError{HTTPStatusCode: 429, Code: "insufficient_quota", Message: secret}, want: []string{"HTTP 429", testBilling}},
		{name: "OpenAI no status", cause: &openaisdk.APIError{Message: secret}, want: []string{testSettings}},
		{name: "Anthropic request", cause: &anthropicsdk.RequestError{StatusCode: 402, Err: errors.New(secret), Body: []byte(secret)}, want: []string{"HTTP 402", testBilling}},
		{name: "Anthropic authentication", cause: &anthropicsdk.APIError{Type: anthropicsdk.ErrTypeAuthentication, Message: secret}, want: []string{testAPIKey}},
		{name: "Anthropic permission", cause: &anthropicsdk.APIError{Type: anthropicsdk.ErrTypePermission, Message: secret}, want: []string{"permission"}},
		{name: "Anthropic invalid request", cause: &anthropicsdk.APIError{Type: anthropicsdk.ErrTypeInvalidRequest, Message: secret}, want: []string{testInvalid}},
		{name: "Anthropic not found", cause: &anthropicsdk.APIError{Type: anthropicsdk.ErrTypeNotFound, Message: secret}, want: []string{"model and base URL"}},
		{name: "Anthropic too large", cause: &anthropicsdk.APIError{Type: anthropicsdk.ErrTypeTooLarge, Message: secret}, want: []string{"too large"}},
		{name: "Anthropic rate limit", cause: &anthropicsdk.APIError{Type: anthropicsdk.ErrTypeRateLimit, Message: secret}, want: []string{"rate limit"}},
		{name: "Anthropic server error", cause: &anthropicsdk.APIError{Type: anthropicsdk.ErrTypeApi, Message: secret}, want: []string{testUnavailable}},
		{name: "Anthropic overload", cause: &anthropicsdk.APIError{Type: anthropicsdk.ErrTypeOverloaded, Message: secret}, want: []string{testUnavailable}},
		{name: "Anthropic unknown", cause: &anthropicsdk.APIError{Type: anthropicsdk.ErrType(secret), Message: secret}, want: []string{testSettings}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := repository.WrapNetworkError("TestProvider", fmt.Errorf("outer secret-key: %w", tt.cause))
			if got == nil {
				t.Fatal("expected error")
			}
			for _, want := range append(tt.want, "TestProvider") {
				if !strings.Contains(got.Error(), want) {
					t.Errorf("error = %q, want %q", got, want)
				}
			}
			if !errors.Is(got, tt.cause) {
				t.Error("original SDK error identity lost")
			}
			openAIAPIError, isOpenAIAPI := errors.AsType[*openaisdk.APIError](tt.cause)
			openAIRequestError, isOpenAIRequest := errors.AsType[*openaisdk.RequestError](tt.cause)
			anthropicAPIError, isAnthropicAPI := errors.AsType[*anthropicsdk.APIError](tt.cause)
			anthropicRequestError, isAnthropicRequest := errors.AsType[*anthropicsdk.RequestError](tt.cause)
			switch {
			case isOpenAIAPI && openAIAPIError != nil:
				if typed, ok := errors.AsType[*openaisdk.APIError](got); !ok || typed == nil {
					t.Error("OpenAI API error type lost")
				}
			case isOpenAIRequest && openAIRequestError != nil:
				if typed, ok := errors.AsType[*openaisdk.RequestError](got); !ok || typed == nil {
					t.Error("OpenAI request error type lost")
				}
			case isAnthropicAPI && anthropicAPIError != nil:
				if typed, ok := errors.AsType[*anthropicsdk.APIError](got); !ok || typed == nil {
					t.Error("Anthropic API error type lost")
				}
			case isAnthropicRequest && anthropicRequestError != nil:
				if typed, ok := errors.AsType[*anthropicsdk.RequestError](got); !ok || typed == nil {
					t.Error("Anthropic request error type lost")
				}
			}
			for _, forbidden := range []string{testSecretKey, testHTTPS, testPrivateHost, "raw prompt"} {
				if strings.Contains(got.Error(), forbidden) {
					t.Errorf("error exposes %q: %s", forbidden, got)
				}
			}
		})
	}
}

func TestHTTPStatusError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status int
		want   string
	}{
		{status: 400, want: testInvalid},
		{status: 401, want: testAPIKey},
		{status: 402, want: testBilling},
		{status: 403, want: "permission"},
		{status: 404, want: "model and base URL"},
		{status: 408, want: "timed out"},
		{status: 409, want: "conflict"},
		{status: 413, want: "too large"},
		{status: 422, want: testInvalid},
		{status: 429, want: "rate limit"},
		{status: 500, want: testUnavailable},
		{status: 502, want: testUnavailable},
		{status: 503, want: testUnavailable},
		{status: 504, want: testUnavailable},
		{status: 529, want: testUnavailable},
		{status: 418, want: testSettings},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.status), func(t *testing.T) {
			t.Parallel()
			got := repository.HTTPStatusError("TestProvider", tt.status)
			if got == nil {
				t.Fatal("expected error")
			}
			for _, want := range []string{fmt.Sprintf("TestProvider HTTP %d", tt.status), tt.want} {
				if !strings.Contains(got.Error(), want) {
					t.Errorf("error = %q, want %q", got, want)
				}
			}
		})
	}
}

type providerErrorTransport func(*http.Request) (*http.Response, error)

func (f providerErrorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestHTTPProviderErrorsHideResponse(t *testing.T) {
	t.Parallel()

	for _, name := range []string{testGemini, testOpenCode} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			client := &http.Client{Transport: providerErrorTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader("secret-key private prompt https://private.example")), Header: make(http.Header)}, nil
			})}
			var p domain.InferenceProvider
			var err error
			if name == testGemini {
				p, err = gemini.New(testSecretKey, "test-model", "https://private.example", client)
			} else {
				p, err = opencode.New(testSecretKey, "test-model", "https://private.example", client)
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, history := range []bool{false, true} {
				var got error
				if history {
					_, got = p.GetOptionsWithHistory(t.Context(), &domain.ConversationHistory{})
				} else {
					_, got = p.GetOptions(t.Context(), "test prompt", "test context")
				}
				if got == nil {
					t.Fatal("expected error")
				}
				for _, want := range []string{name + " HTTP 401", testAPIKey, "zev setup"} {
					if !strings.Contains(got.Error(), want) {
						t.Errorf("history=%t error=%q, want %q", history, got, want)
					}
				}
				for _, secret := range []string{testSecretKey, testPrivatePrompt, testPrivateHost, testHTTPS} {
					if strings.Contains(got.Error(), secret) {
						t.Errorf("history=%t error exposes %q: %s", history, secret, got)
					}
				}
			}
		})
	}
}

func TestWrapNetworkErrorDiagnosis(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		cause error
		want  string
	}{
		{name: "reset", cause: syscall.ECONNRESET, want: "connection to TestProvider was reset"},
		{name: "broken pipe", cause: syscall.EPIPE, want: "connection to TestProvider was reset"},
		{name: "generic network timeout", cause: &net.OpError{Op: "read", Err: os.ErrDeadlineExceeded}, want: "TestProvider request timed out"},
		{name: "untrusted certificate", cause: x509.UnknownAuthorityError{}, want: testTLSVerify},
		{name: "hostname mismatch", cause: x509.HostnameError{}, want: testTLSVerify},
		{name: "expired certificate", cause: x509.CertificateInvalidError{}, want: testTLSVerify},
		{name: "certificate verification", cause: &tls.CertificateVerificationError{Err: errors.New(testSecretKey)}, want: testTLSVerify},
		{name: "TLS protocol", cause: tls.RecordHeaderError{Msg: testSecretKey}, want: "TLS connection failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cause := &url.Error{Op: testPost, URL: "https://user:secret-key@private.example/?key=secret-key", Err: tt.cause}
			got := repository.WrapNetworkError("TestProvider", fmt.Errorf("secret-key: %w", cause))
			if got == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(got.Error(), tt.want) {
				t.Errorf("error = %q, want %q", got, tt.want)
			}
			if !errors.Is(got, cause) {
				t.Error("original error identity lost")
			}
			if urlErr, ok := errors.AsType[*url.Error](got); !ok || urlErr != cause {
				t.Error("original URL error type lost")
			}
			for _, secret := range []string{testSecretKey, testPrivateHost, testHTTPS} {
				if strings.Contains(got.Error(), secret) {
					t.Errorf("error exposes %q: %s", secret, got)
				}
			}
		})
	}
}

func TestWrapNetworkErrorHidesDetails(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		cause error
	}{
		{name: "generic cause", cause: errors.New("secret-key https://private.example/body password")},
		{name: "DNS cause", cause: &net.DNSError{Err: "no such host", Name: "secret-key.private.example"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cause := tc.cause
			original := &url.Error{
				Op: testPost, URL: "https://user:password@private.example/?key=secret-key", Err: cause,
			}
			got := repository.WrapNetworkError(testGemini, original)
			if got == nil {
				t.Fatal("expected error")
			}
			for _, forbidden := range []string{testSecretKey, "password", testPrivateHost, testHTTPS} {
				if strings.Contains(got.Error(), forbidden) {
					t.Errorf("error exposes %q: %s", forbidden, got)
				}
			}
			if !errors.Is(got, cause) {
				t.Error("cause was lost")
			}
			unwrapped, ok := errors.AsType[*url.Error](got)
			if !ok || unwrapped != original {
				t.Error("url.Error was lost")
			}
		})
	}
}
