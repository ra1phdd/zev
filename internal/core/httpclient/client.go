// Package httpclient provides HTTP clients with headers and bounded retries.
package httpclient

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// New creates an HTTP client with custom headers and retry handling.
func New(headers map[string]string, logger ...*slog.Logger) *http.Client {
	if headers == nil {
		headers = make(map[string]string)
	}

	if _, ok := headers["User-Agent"]; !ok {
		headers["User-Agent"] = "zev/1.0"
	}

	transport := &headersTransport{
		headers: headers,
		base:    newRetryTransport(http.DefaultTransport, firstLogger(logger), nil),
	}

	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
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

func (t *headersTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	reqCopy := req.Clone(req.Context())

	for key, value := range t.headers {
		reqCopy.Header[key] = []string{value}
	}

	return wrapRoundTripResponse(t.base.RoundTrip(reqCopy))
}

func wrapRoundTripResponse(resp *http.Response, err error) (*http.Response, error) {
	if err == nil {
		return resp, nil
	}
	return resp, fmt.Errorf("HTTP round trip: %w", err)
}
