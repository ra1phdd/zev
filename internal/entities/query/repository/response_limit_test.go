package repository

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestResponseBodyLimitBoundsReadsAndCloses(t *testing.T) {
	t.Parallel()

	for _, size := range []int{0, MaxProviderResponseBytes - 1, MaxProviderResponseBytes, MaxProviderResponseBytes + 1, MaxProviderResponseBytes * 4} {
		t.Run(strings.Repeat("x", min(size, 5)), func(t *testing.T) {
			t.Parallel()
			body := &countedResponseBody{Reader: strings.NewReader(strings.Repeat("x", size))}
			client := &http.Client{Transport: responseTestTransport{body: body}}
			limited := WithResponseBodyLimit(client)
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://provider.test", nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := limited.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_, readErr := io.ReadAll(resp.Body)
			if err := resp.Body.Close(); err != nil {
				t.Fatal(err)
			}
			if !body.closed {
				t.Fatal("underlying response body was not closed")
			}
			if size > MaxProviderResponseBytes {
				if !errors.Is(readErr, ErrProviderResponseTooLarge) {
					t.Fatalf("read error = %v, want ErrProviderResponseTooLarge", readErr)
				}
			} else if readErr != nil {
				t.Fatalf("read error = %v, want nil", readErr)
			}
			if body.read != min(size, MaxProviderResponseBytes+1) {
				t.Fatalf("bytes consumed = %d, want %d", body.read, min(size, MaxProviderResponseBytes+1))
			}
			if _, ok := client.Transport.(responseTestTransport); !ok {
				t.Fatal("original client transport changed")
			}
		})
	}
}

type responseTestTransport struct {
	body io.ReadCloser
}

func (t responseTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: t.body, Request: req}, nil
}

type countedResponseBody struct {
	*strings.Reader
	read   int
	closed bool
}

func (b *countedResponseBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.read += n
	if err != nil {
		if errors.Is(err, io.EOF) {
			return n, io.EOF
		}
		return n, fmt.Errorf("reading test response body: %w", err)
	}
	return n, nil
}

func (b *countedResponseBody) Close() error {
	b.closed = true
	return nil
}
