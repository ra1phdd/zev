package httpclient

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewWithProxyUsesHTTPSConnect(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte("secure through proxy")); err != nil {
			t.Errorf("write upstream response: %v", err)
		}
	}))
	t.Cleanup(upstream.Close)

	connects := make(chan string, 1)
	proxy := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "CONNECT required", http.StatusMethodNotAllowed)
			return
		}
		connects <- r.Host
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Error("HTTPS proxy does not support hijacking")
			return
		}
		conn, rw, err := hijacker.Hijack()
		if err != nil {
			t.Errorf("hijack proxy connection: %v", err)
			return
		}
		defer func() {
			if err := conn.Close(); err != nil {
				t.Errorf("close proxy connection: %v", err)
			}
		}()
		if _, err := rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			t.Errorf("write CONNECT response: %v", err)
			return
		}
		if err := rw.Flush(); err != nil {
			t.Errorf("flush CONNECT response: %v", err)
			return
		}
		target, err := (&net.Dialer{}).DialContext(r.Context(), "tcp", strings.TrimPrefix(upstream.URL, "https://"))
		if err != nil {
			t.Errorf("connect test upstream: %v", err)
			return
		}
		defer func() {
			if err := target.Close(); err != nil {
				t.Errorf("close test upstream connection: %v", err)
			}
		}()
		go func() {
			if _, err := io.Copy(target, rw.Reader); err != nil {
				return
			}
		}()
		if _, err := io.Copy(conn, target); err != nil {
			t.Errorf("relay upstream response: %v", err)
		}
	}))
	t.Cleanup(proxy.Close)

	client, err := NewWithProxy(nil, proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	headerTransport, ok := client.Transport.(*headersTransport)
	if !ok {
		t.Fatalf("client transport type = %T", client.Transport)
	}
	retries, ok := headerTransport.base.(*retryTransport)
	if !ok {
		t.Fatalf("retry transport type = %T", headerTransport.base)
	}
	transport, ok := retries.base.(*http.Transport)
	if !ok {
		t.Fatalf("base transport type = %T", retries.base)
	}
	roots := x509.NewCertPool()
	roots.AddCert(proxy.Certificate())
	roots.AddCert(upstream.Certificate())
	transport.TLSClientConfig = &tls.Config{RootCAs: roots}

	targetURL := strings.Replace(upstream.URL, "localhost", "127.0.0.1", 1)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, targetURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if string(body) != "secure through proxy" {
		t.Fatalf("HTTPS response = %q", body)
	}
	if got, want := <-connects, strings.TrimPrefix(targetURL, "https://"); got != want {
		t.Fatalf("CONNECT target = %q, want %q", got, want)
	}
}
