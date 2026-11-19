package httpclient

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	allProxyEnv = "ALL_PROXY"
	noProxyEnv  = "NO_PROXY"
)

var proxyEnvKeys = []string{allProxyEnv, "all_proxy", "HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", noProxyEnv, "no_proxy"}

func TestNewWithProxyUsesHTTPProxy(t *testing.T) {
	t.Parallel()

	requestURIs := make(chan string, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestURIs <- r.RequestURI
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(proxy.Close)

	client, err := NewWithProxy(nil, proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://model.example/v1/chat/completions", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	requestURI := <-requestURIs
	if requestURI != "http://model.example/v1/chat/completions" {
		t.Fatalf("proxy request URI = %q", requestURI)
	}
}

func TestConfigureProxyValidatesSupportedURLs(t *testing.T) {
	t.Parallel()

	for _, proxyURL := range []string{"socks5://127.0.0.1:1080", "socks5h://127.0.0.1:1080", "http://127.0.0.1:8080", "https://127.0.0.1:8443"} {
		t.Run(proxyURL, func(t *testing.T) {
			transport := &http.Transport{}
			if err := ConfigureProxy(transport, proxyURL); err != nil {
				t.Fatalf("ConfigureProxy(%q): %v", proxyURL, err)
			}
			if strings.HasPrefix(proxyURL, "socks5://") {
				if transport.Proxy != nil || transport.DialContext == nil {
					t.Fatalf("local-resolution SOCKS5 transport has proxy=%t dialer=%t", transport.Proxy != nil, transport.DialContext != nil)
				}
				return
			}
			request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.com", nil)
			if err != nil {
				t.Fatal(err)
			}
			configured, err := transport.Proxy(request)
			if err != nil || configured == nil || configured.Scheme != proxyURL[:strings.IndexByte(proxyURL, ':')] {
				t.Fatalf("configured proxy = %v, %v", configured, err)
			}
		})
	}

	for _, proxyURL := range []string{"socks5://host", "ftp://host:21", "http://host:8080/path", "http://host:8080?token=x", "http://host:70000"} {
		if err := ConfigureProxy(&http.Transport{}, proxyURL); err == nil {
			t.Errorf("ConfigureProxy(%q) unexpectedly succeeded", proxyURL)
		}
	}
}

func TestResolveProxyExplicitValueOverridesEnvironment(t *testing.T) { //nolint:paralleltest // t.Setenv requires a non-parallel test.
	for _, key := range proxyEnvKeys {
		t.Setenv(key, "invalid://default")
	}

	got, err := ResolveProxy("socks5h://proxy.example:1080")
	if err != nil {
		t.Fatal(err)
	}
	if got != "socks5h://proxy.example:1080" {
		t.Fatalf("ResolveProxy() = %q", got)
	}
}

func TestResolveProxyEnvironmentPrecedence(t *testing.T) {
	for _, key := range proxyEnvKeys {
		t.Setenv(key, "")
	}
	t.Setenv("HTTP_PROXY", "http://http-proxy.example:8080")
	t.Setenv("HTTPS_PROXY", "socks5h://secure-proxy.example:1080")
	t.Setenv(allProxyEnv, "https://all-proxy.example:8443")
	t.Setenv(noProxyEnv, "*")

	got, err := ResolveProxy("")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://all-proxy.example:8443" {
		t.Fatalf("ResolveProxy() = %q, want ALL_PROXY", got)
	}
}

func TestNewWithProxyUsesEnvironmentWithoutNoProxyBypass(t *testing.T) {
	for _, key := range proxyEnvKeys {
		t.Setenv(key, "")
	}
	requestURIs := make(chan string, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestURIs <- r.RequestURI
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(proxy.Close)
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv(noProxyEnv, "*")

	client, err := NewWithProxy(nil, "")
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://model.example/v1/chat/completions", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if got := <-requestURIs; got != "http://model.example/v1/chat/completions" {
		t.Fatalf("proxy request URI = %q", got)
	}
}
