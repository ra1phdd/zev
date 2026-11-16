package repository

import (
	"errors"
	"fmt"
	"io"
	"net/http"
)

const (
	// MaxProviderResponseBytes bounds the provider HTTP envelope before SDK decoding.
	MaxProviderResponseBytes = 256 * 1024
)

// ErrProviderResponseTooLarge indicates that the provider exceeded its response envelope.
var ErrProviderResponseTooLarge = errors.New("provider response exceeds size limit")

// WithResponseBodyLimit returns a client that limits every provider response body.
// The original client and transport are left unchanged so callers can reuse them.
func WithResponseBodyLimit(client *http.Client) *http.Client {
	limited := *client
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	limited.Transport = responseLimitTransport{base: base}
	return &limited
}

type responseLimitTransport struct {
	base http.RoundTripper
}

func (t responseLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil || resp == nil || resp.Body == nil {
		if err != nil {
			return resp, fmt.Errorf("provider response transport: %w", err)
		}
		return resp, nil
	}
	resp.Body = &limitedResponseBody{body: resp.Body}
	return resp, nil
}

type limitedResponseBody struct {
	body io.ReadCloser
	read int
}

func (b *limitedResponseBody) Read(p []byte) (int, error) {
	if b.read > MaxProviderResponseBytes {
		return 0, ErrProviderResponseTooLarge
	}
	remaining := MaxProviderResponseBytes + 1 - b.read
	if len(p) > remaining {
		p = p[:remaining]
	}
	n, err := b.body.Read(p)
	b.read += n
	if b.read > MaxProviderResponseBytes {
		return n, ErrProviderResponseTooLarge
	}
	if errors.Is(err, io.EOF) {
		return n, io.EOF
	}
	if err != nil {
		return n, fmt.Errorf("reading provider response: %w", err)
	}
	return n, nil
}

func (b *limitedResponseBody) Close() error {
	if err := b.body.Close(); err != nil {
		return fmt.Errorf("closing provider response: %w", err)
	}
	return nil
}
