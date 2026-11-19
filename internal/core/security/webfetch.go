package security

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ra1phdd/zev/internal/core/httpclient"
)

const (
	maxFetchURLBytes  = 2048
	maxFetchBodyBytes = 256 * 1024
	maxFetchRedirects = 3
	fetchTimeout      = 15 * time.Second
	httpsScheme       = "https"
)

// FetchResult contains bounded, redacted, untrusted public-web data.
type FetchResult struct {
	URL       string
	Status    int
	Body      string
	Truncated bool
	Security  ProbeAnnotation
}

type ipResolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

// Fetcher performs only bounded public HTTP GET requests. A zero Fetcher uses
// net.DefaultResolver and a transport that revalidates every dialed address.
type Fetcher struct {
	resolver ipResolver
	client   *http.Client
	Proxy    string
}

// Capability describes the boundary enforced by Fetch.
func (Fetcher) Capability() Capability {
	return Capability{
		ID:                  "tool.web_fetch.public_get",
		Owner:               SourceWebFetch,
		Effects:             []Effect{EffectNetwork},
		NetworkDestinations: []string{"public-http"},
		Enforcement:         EnforcementApplication,
	}
}

// Fetch retrieves one public HTTP(S) URL without cookies, ambient credentials,
// or uploads. It uses the configured proxy when present and validates redirects
// independently.
func (f Fetcher) Fetch(ctx context.Context, rawURL string) (FetchResult, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	resolver := f.resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	proxyURL, err := httpclient.ResolveProxy(f.Proxy)
	if err != nil {
		return FetchResult{}, fmt.Errorf("configure web fetch proxy: %w", err)
	}
	localResolution := proxyURL == "" || strings.HasPrefix(proxyURL, "socks5://")
	u, err := parsePublicURL(ctx, rawURL, resolver, localResolution)
	if err != nil {
		return FetchResult{}, err
	}
	client := f.client
	if client == nil {
		client, err = newPublicFetchClient(resolver, publicURLOrigin(u), proxyURL)
		if err != nil {
			return FetchResult{}, fmt.Errorf("configure web fetch proxy: %w", err)
		}
	} else {
		if proxyURL != "" {
			return FetchResult{}, errors.New("custom web fetch client cannot guarantee proxy routing")
		}
		// Keep test/in-process transports usable, but never inherit a caller's
		// redirect policy. Every redirect must pass the same destination checks.
		clientCopy := *client
		clientCopy.CheckRedirect = publicFetchRedirectPolicy(resolver, publicURLOrigin(u), localResolution)
		client = &clientCopy
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return FetchResult{}, fmt.Errorf("create web fetch request: %w", err)
	}
	req.Header.Set("Accept", "text/plain, text/html, application/json, application/xml;q=0.9, */*;q=0.1")
	req.Header.Set("User-Agent", "zev-public-fetch/1")
	resp, err := client.Do(req)
	if err != nil {
		return FetchResult{}, fmt.Errorf("web fetch request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			return
		}
	}()
	return readFetchResponse(resp)
}

func newPublicFetchClient(resolver ipResolver, initialOrigin, proxyURL string) (*http.Client, error) {
	transport := publicFetchTransport(resolver)
	if proxyURL != "" {
		// A selected proxy owns destination resolution, except socks5, whose
		// local-resolution dialer validates and pins public destination IPs.
		transport.DialContext = nil
		if err := httpclient.ConfigureProxyWithResolver(transport, proxyURL, resolver, isPublicIP); err != nil {
			return nil, fmt.Errorf("configure public fetch proxy: %w", err)
		}
	}
	return &http.Client{
		Transport:     transport,
		CheckRedirect: publicFetchRedirectPolicy(resolver, initialOrigin, proxyURL == "" || strings.HasPrefix(proxyURL, "socks5://")),
	}, nil
}

func publicFetchRedirectPolicy(resolver ipResolver, initialOrigin string, localResolution bool) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxFetchRedirects {
			return errors.New("web fetch redirect limit exceeded")
		}
		if _, err := parsePublicURL(req.Context(), req.URL.String(), resolver, localResolution); err != nil {
			return fmt.Errorf("validate web fetch redirect: %w", err)
		}
		if publicURLOrigin(req.URL) != initialOrigin {
			return errors.New("web fetch redirect changes origin")
		}
		return nil
	}
}

func publicURLOrigin(u *url.URL) string {
	port := u.Port()
	if port == "" {
		if u.Scheme == httpsScheme {
			port = "443"
		} else {
			port = "80"
		}
	}
	return strings.ToLower(u.Scheme) + "://" + net.JoinHostPort(strings.ToLower(u.Hostname()), port)
}

// SafeURLForDisclosure returns only the public origin. URL paths, queries, and
// fragments are not safe diagnostic fields because services commonly put tokens
// and object identifiers there.
func SafeURLForDisclosure(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Hostname() == "" || u.User != nil {
		return "[REDACTED_URL]"
	}
	u.User = nil
	u.Path, u.RawPath, u.RawQuery, u.Fragment = "", "", "", ""
	return u.String()
}

func readFetchResponse(resp *http.Response) (FetchResult, error) {
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return FetchResult{}, fmt.Errorf("web fetch returned HTTP status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBodyBytes+1))
	if err != nil {
		return FetchResult{}, fmt.Errorf("read web fetch response: %w", err)
	}
	truncated := len(data) > maxFetchBodyBytes
	if truncated {
		data = data[:maxFetchBodyBytes]
	}
	body := RedactSensitive(string(data))
	responseURL := ""
	if resp.Request != nil && resp.Request.URL != nil {
		responseURL = SafeURLForDisclosure(resp.Request.URL.String())
	}
	return FetchResult{
		URL:       responseURL,
		Status:    resp.StatusCode,
		Body:      body,
		Truncated: truncated,
		Security:  Probe(body, truncated),
	}, nil
}

func parsePublicURL(ctx context.Context, rawURL string, resolver ipResolver, localResolution bool) (*url.URL, error) {
	u, err := parsePublicURLSyntax(rawURL)
	if err != nil {
		return nil, err
	}
	if err := validatePublicURLDestination(ctx, u, resolver, localResolution); err != nil {
		return nil, err
	}
	return u, nil
}

func parsePublicURLSyntax(rawURL string) (*url.URL, error) {
	if len(rawURL) == 0 || len(rawURL) > maxFetchURLBytes {
		return nil, errors.New("web fetch URL is empty or too long")
	}
	u, err := url.ParseRequestURI(rawURL)
	if err != nil || !u.IsAbs() || u.Hostname() == "" || u.User != nil {
		return nil, errors.New("web fetch URL must be an absolute HTTP(S) URL without userinfo")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("web fetch URL scheme must be http or https")
	}
	if u.Port() != "" && ((u.Scheme == "http" && u.Port() != "80") || (u.Scheme == "https" && u.Port() != "443")) {
		return nil, errors.New("web fetch URL uses an unsupported port")
	}
	if strings.ContainsAny(u.Host, "\r\n") {
		return nil, errors.New("web fetch URL contains control characters")
	}
	if strings.Contains(u.Hostname(), "%") {
		return nil, errors.New("web fetch URL contains a scoped IP address")
	}
	return u, nil
}

func validatePublicURLDestination(ctx context.Context, u *url.URL, resolver ipResolver, localResolution bool) error {
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		if !isPublicIP(ip) {
			return errors.New("web fetch destination is private or reserved")
		}
		return nil
	}
	if !localResolution {
		return nil
	}
	addresses, err := resolver.LookupIPAddr(ctx, u.Hostname())
	if err != nil {
		return fmt.Errorf("resolve web fetch destination: %w", err)
	}
	if len(addresses) == 0 {
		return errors.New("web fetch destination has no addresses")
	}
	for _, address := range addresses {
		if !isPublicIP(address.IP) {
			return errors.New("web fetch destination is private or reserved")
		}
	}
	return nil
}

func publicFetchTransport(resolver ipResolver) *http.Transport {
	return &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, fmt.Errorf("split web fetch address: %w", err)
			}
			ips, err := resolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("resolve web fetch connection: %w", err)
			}
			dialer := net.Dialer{}
			var lastErr error
			for _, ip := range ips {
				if !isPublicIP(ip.IP) {
					continue
				}
				conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
				if dialErr == nil {
					return conn, nil
				}
				lastErr = dialErr
			}
			if lastErr != nil {
				return nil, fmt.Errorf("dial public web fetch destination: %w", lastErr)
			}
			return nil, errors.New("web fetch connection has no public address")
		},
	}
}

func isPublicIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return false
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return !reservedIPv6(ip)
	}
	first, second := ip4[0], ip4[1]
	if first == 0 || first >= 240 || first == 127 || first == 10 || first == 192 && (second == 0 || second == 168) || first == 172 && second >= 16 && second <= 31 || first == 169 && second == 254 || first == 100 && second >= 64 && second <= 127 || first == 198 && (second == 18 || second == 19 || second == 51) || first == 203 && second == 0 && ip4[2] == 113 {
		return false
	}
	return true
}

func reservedIPv6(ip net.IP) bool {
	for _, network := range []string{"2001:db8::/32", "2001:2::/48", "2001:10::/28", "2001:20::/28"} {
		_, subnet, err := net.ParseCIDR(network)
		if err == nil && subnet.Contains(ip) {
			return true
		}
	}
	return false
}
