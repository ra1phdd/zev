package security

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetcherRoutesRequestsThroughConfiguredProxy(t *testing.T) {
	t.Parallel()

	requestURIs := make(chan string, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestURIs <- r.RequestURI
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("fetched through proxy")); err != nil {
			t.Errorf("write proxy response: %v", err)
		}
	}))
	t.Cleanup(proxy.Close)

	fetcher := Fetcher{
		resolver: fixtureResolver{publicHost: {{IP: net.ParseIP("93.184.216.34")}}},
		Proxy:    proxy.URL,
	}
	result, err := fetcher.Fetch(t.Context(), "http://example.com/document")
	if err != nil {
		t.Fatal(err)
	}
	requestURI := <-requestURIs
	if requestURI != "http://example.com/document" {
		t.Fatalf("proxy request URI = %q", requestURI)
	}
	if result.Body != "fetched through proxy" {
		t.Fatalf("response body = %q", result.Body)
	}
}
