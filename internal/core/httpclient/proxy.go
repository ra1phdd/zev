package httpclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	xproxy "golang.org/x/net/proxy"
)

// Resolver resolves hostnames for locally-resolving SOCKS5 proxies.
type Resolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

// ResolveProxy returns an explicit proxy URL or the first configured default
// from ALL_PROXY, HTTPS_PROXY, and HTTP_PROXY (uppercase before lowercase).
// NO_PROXY is intentionally ignored so a selected proxy cannot be bypassed.
func ResolveProxy(explicit string) (string, error) {
	proxyURL := explicit
	if proxyURL == "" {
		for _, key := range []string{"ALL_PROXY", "all_proxy", "HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
			if value := strings.TrimSpace(os.Getenv(key)); value != "" {
				proxyURL = value
				break
			}
		}
	}
	if proxyURL == "" {
		return "", nil
	}
	parsed, err := parseProxyURL(proxyURL)
	if err != nil {
		return "", err
	}
	return parsed.String(), nil
}

// ConfigureProxy validates proxyURL and configures transport to use it. An
// empty value selects the proxy environment defaults. A non-empty URL must
// include a host and port and use socks5, socks5h, http, or https.
// socks5 resolves destination names locally; socks5h resolves them at the proxy.
func ConfigureProxy(transport *http.Transport, proxyURL string) error {
	return ConfigureProxyWithResolver(transport, proxyURL, net.DefaultResolver, nil)
}

// ConfigureProxyWithResolver is ConfigureProxy with an injected destination
// resolver and optional IP policy for locally resolved SOCKS5 destinations.
func ConfigureProxyWithResolver(transport *http.Transport, proxyURL string, resolver Resolver, allowIP func(net.IP) bool) error {
	if transport == nil {
		return errors.New("HTTP transport is nil")
	}
	resolved, err := ResolveProxy(proxyURL)
	if err != nil {
		return err
	}
	transport.Proxy = nil
	if resolved == "" {
		return nil
	}
	parsed, err := parseProxyURL(resolved)
	if err != nil {
		return err
	}
	if parsed.Scheme != "socks5" {
		transport.Proxy = http.ProxyURL(parsed)
		return nil
	}
	return configureSOCKS5Dialer(transport, parsed, resolver, allowIP)
}

func configureSOCKS5Dialer(transport *http.Transport, parsed *url.URL, resolver Resolver, allowIP func(net.IP) bool) error {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	var auth *xproxy.Auth
	if parsed.User != nil {
		password, _ := parsed.User.Password()
		auth = &xproxy.Auth{User: parsed.User.Username(), Password: password}
	}
	dialer, err := xproxy.SOCKS5("tcp", parsed.Host, auth, nil)
	if err != nil {
		return fmt.Errorf("initialize SOCKS5 proxy dialer: %w", err)
	}
	contextDialer, ok := dialer.(xproxy.ContextDialer)
	if !ok {
		return errors.New("SOCKS5 proxy dialer does not support context cancellation")
	}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		ips, port, err := resolveSOCKS5Destination(ctx, resolver, address, allowIP)
		if err != nil {
			return nil, err
		}
		var lastErr error
		for _, ip := range ips {
			conn, err := contextDialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		return nil, fmt.Errorf("connect through SOCKS5 proxy: %w", lastErr)
	}
	return nil
}

func resolveSOCKS5Destination(ctx context.Context, resolver Resolver, address string, allowIP func(net.IP) bool) ([]net.IP, string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, "", fmt.Errorf("split SOCKS5 destination: %w", err)
	}
	ips, err := destinationIPs(ctx, resolver, host)
	if err != nil {
		return nil, "", fmt.Errorf("resolve SOCKS5 destination: %w", err)
	}
	if allowIP != nil {
		for _, ip := range ips {
			if !allowIP(ip) {
				return nil, "", errors.New("SOCKS5 destination is private or reserved")
			}
		}
	}
	return ips, port, nil
}

func destinationIPs(ctx context.Context, resolver Resolver, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	addresses, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("look up destination addresses: %w", err)
	}
	if len(addresses) == 0 {
		return nil, errors.New("destination has no IP addresses")
	}
	ips := make([]net.IP, 0, len(addresses))
	for _, address := range addresses {
		ips = append(ips, address.IP)
	}
	return ips, nil
}

func parseProxyURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" || parsed.Port() == "" || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.Opaque != "" || (parsed.Path != "" && parsed.Path != "/") {
		return nil, errors.New("proxy URL must include a host and port without a path, query, or fragment")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "socks5" && scheme != "socks5h" && scheme != "http" && scheme != "https" {
		return nil, errors.New("proxy URL scheme must be socks5, socks5h, http, or https")
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port < 1 || port > 65535 {
		return nil, errors.New("proxy URL port is invalid")
	}
	parsed.Scheme = scheme
	return parsed, nil
}
