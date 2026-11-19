package httpclient

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

type staticResolver []net.IPAddr

func (r staticResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return r, nil
}

type failingResolver struct{}

func (failingResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return nil, io.ErrUnexpectedEOF
}

func TestSOCKS5ProxyDNSModes(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		scheme     string
		resolver   Resolver
		wantATYP   byte
		wantDomain string
		wantIP     net.IP
	}{
		{name: "local DNS", scheme: "socks5", resolver: staticResolver{{IP: net.ParseIP("93.184.216.34")}}, wantATYP: 1, wantIP: net.ParseIP("93.184.216.34")},
		{name: "proxy DNS", scheme: "socks5h", resolver: failingResolver{}, wantATYP: 3, wantDomain: "model.example"},
	} {
		t.Run(test.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				if _, err := w.Write([]byte("through socks")); err != nil {
					t.Errorf("write upstream response: %v", err)
				}
			}))
			t.Cleanup(upstream.Close)

			listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := listener.Close(); err != nil {
					t.Errorf("close SOCKS5 listener: %v", err)
				}
			})
			destination := make(chan []byte, 1)
			go serveSOCKS5Proxy(t, listener, upstream.Listener.Addr().String(), destination)

			transport := &http.Transport{}
			proxyURL := test.scheme + "://" + listener.Addr().String()
			if err := ConfigureProxyWithResolver(transport, proxyURL, test.resolver, nil); err != nil {
				t.Fatal(err)
			}
			client := &http.Client{Transport: transport}
			httpRequest, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://model.example/v1/chat/completions", nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := client.Do(httpRequest)
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
			if string(body) != "through socks" {
				t.Fatalf("response body = %q", body)
			}
			request := <-destination
			if request[0] != test.wantATYP {
				t.Fatalf("SOCKS address type = %d, want %d", request[0], test.wantATYP)
			}
			switch test.wantATYP {
			case 1:
				if !net.IP(request[1:5]).Equal(test.wantIP) {
					t.Fatalf("SOCKS destination IP = %v, want %v", net.IP(request[1:5]), test.wantIP)
				}
			case 3:
				size := int(request[1])
				if got := string(request[2 : 2+size]); got != test.wantDomain {
					t.Fatalf("SOCKS destination domain = %q, want %q", got, test.wantDomain)
				}
			}
		})
	}
}

func serveSOCKS5Proxy(t *testing.T, listener net.Listener, upstream string, destination chan<- []byte) {
	t.Helper()
	conn, err := listener.Accept()
	if err != nil {
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Errorf("close SOCKS5 connection: %v", err)
		}
	}()

	greeting := make([]byte, 2)
	if _, err := io.ReadFull(conn, greeting); err != nil {
		return
	}
	methods := make([]byte, int(greeting[1]))
	if _, err := io.ReadFull(conn, methods); err != nil {
		return
	}
	if _, err := conn.Write([]byte{5, 0}); err != nil {
		return
	}
	request := make([]byte, 4)
	if _, err := io.ReadFull(conn, request); err != nil {
		return
	}
	var address []byte
	switch request[3] {
	case 1:
		address = make([]byte, 4)
	case 3:
		var size [1]byte
		if _, err := io.ReadFull(conn, size[:]); err != nil {
			return
		}
		address = make([]byte, int(size[0])+1)
		address[0] = size[0]
		if _, err := io.ReadFull(conn, address[1:]); err != nil {
			return
		}
	case 4:
		address = make([]byte, 16)
	default:
		return
	}
	if request[3] != 3 {
		if _, err := io.ReadFull(conn, address); err != nil {
			return
		}
	}
	port := make([]byte, 2)
	if _, err := io.ReadFull(conn, port); err != nil {
		return
	}
	destination <- append([]byte{request[3]}, address...)
	if _, err := conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0}); err != nil {
		return
	}
	target, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", upstream)
	if err != nil {
		return
	}
	defer func() {
		if err := target.Close(); err != nil {
			t.Errorf("close SOCKS5 upstream: %v", err)
		}
	}()
	go func() {
		if _, err := io.Copy(target, conn); err != nil {
			return
		}
	}()
	if _, err := io.Copy(conn, target); err != nil {
		t.Errorf("relay SOCKS5 response: %v", err)
	}
}
