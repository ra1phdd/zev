// Package httpclient provides HTTP clients with headers and bounded retries.
package httpclient

import (
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	userAgentHeader        = "User-Agent"
	blockedRedirectTrailer = "\x00zev-blocked-redirect"
)

// ErrCrossOriginRedirect indicates that a redirect changed the request origin.
var ErrCrossOriginRedirect = errors.New("cross-origin redirect blocked")

// New creates an HTTP client with a snapshot of the custom headers, retry
// handling, and redirects disabled. Redirect responses are returned to the
// caller without replaying the approved request at another URL.
func New(headers map[string]string, logger ...*slog.Logger) *http.Client {
	client, err := NewWithProxy(headers, "", logger...)
	if err != nil {
		return &http.Client{
			Transport:     errorTransport{err: fmt.Errorf("configure HTTP proxy: %w", err)},
			Timeout:       30 * time.Second,
			CheckRedirect: checkRedirect,
		}
	}
	return client
}

// NewWithProxy creates an HTTP client that routes requests through the
// supplied HTTP, HTTPS, SOCKS5, or SOCKS5H proxy URL. It returns an error when
// the URL is invalid or the default HTTP transport cannot be cloned.
func NewWithProxy(headers map[string]string, proxyURL string, logger ...*slog.Logger) (*http.Client, error) {
	ownedHeaders := cloneHeaders(headers)
	if !hasHeader(ownedHeaders, userAgentHeader) {
		ownedHeaders[userAgentHeader] = "zev/1.0"
	}

	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("default HTTP transport has an unsupported type")
	}
	baseTransport := base.Clone()
	if err := ConfigureProxy(baseTransport, proxyURL); err != nil {
		return nil, err
	}

	headerTransport := &headersTransport{
		headers: ownedHeaders,
		base:    newRetryTransport(baseTransport, firstLogger(logger), nil),
	}

	return &http.Client{
		Transport:     headerTransport,
		Timeout:       30 * time.Second,
		CheckRedirect: checkRedirect,
	}, nil
}

func cloneHeaders(headers map[string]string) map[string]string {
	owned := make(map[string]string, len(headers)+1)
	maps.Copy(owned, headers)
	return owned
}

func hasHeader(headers map[string]string, wanted string) bool {
	for key := range headers {
		if strings.EqualFold(key, wanted) {
			return true
		}
	}
	return false
}

func checkRedirect(req *http.Request, _ []*http.Request) error {
	if isBlockedRedirect(req.Response) {
		return ErrCrossOriginRedirect
	}
	return http.ErrUseLastResponse
}

func isBlockedRedirect(resp *http.Response) bool {
	return resp != nil && resp.Trailer[blockedRedirectTrailer] != nil
}

func sameOrigin(first, next *url.URL) bool {
	if first == nil || next == nil {
		return false
	}
	return strings.EqualFold(first.Scheme, next.Scheme) &&
		strings.EqualFold(first.Hostname(), next.Hostname()) &&
		effectivePort(first) == effectivePort(next)
}

func effectivePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	switch strings.ToLower(u.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

func firstLogger(loggers []*slog.Logger) *slog.Logger {
	if len(loggers) == 0 {
		return slog.New(slog.DiscardHandler)
	}
	return loggers[0]
}

type headersTransport struct {
	headers map[string]string
	base    http.RoundTripper
}

type errorTransport struct {
	err error
}

func (t errorTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, t.err
}

func (t *headersTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	reqCopy := req.Clone(req.Context())

	for key, value := range t.headers {
		reqCopy.Header.Set(key, value)
	}

	resp, err := wrapRoundTripResponse(t.base.RoundTrip(reqCopy))
	if err != nil || resp == nil || !isRedirectResponse(resp) {
		return resp, err
	}

	location := resp.Header.Get("Location")
	target, parseErr := reqCopy.URL.Parse(location)
	if location != "" && parseErr == nil && !sameOrigin(reqCopy.URL, target) {
		// net/http includes the redirect Location in CheckRedirect errors. Keep
		// that error safe while preserving the original redirect classification.
		if resp.Header == nil {
			resp.Header = make(http.Header)
		}
		resp.Header.Set("Location", "/")
		resp.Trailer = http.Header{blockedRedirectTrailer: {"1"}}
	}
	return resp, nil
}

func isRedirectResponse(resp *http.Response) bool {
	switch resp.StatusCode {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return true
	default:
		return false
	}
}

func wrapRoundTripResponse(resp *http.Response, err error) (*http.Response, error) {
	if err == nil {
		return resp, nil
	}
	return resp, fmt.Errorf("HTTP round trip: %w", err)
}
