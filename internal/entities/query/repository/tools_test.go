package repository

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestOpenAIToolResponseValidationReadsAndClosesBeforeReturning(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		suffix  string
		wantErr bool
	}{
		{name: "exact limit", suffix: strings.Repeat(" ", MaxProviderResponseBytes-2)},
		{name: "trailing value", suffix: ` {}`, wantErr: true},
		{name: "oversized", suffix: strings.Repeat(" ", MaxProviderResponseBytes*2), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := &countedResponseBody{Reader: strings.NewReader("{}" + tc.suffix)}
			client := &http.Client{Transport: responseTestTransport{body: body}, Timeout: time.Minute}
			validated := WithOpenAIToolResponseValidation(client)
			if validated == client || validated.Timeout != client.Timeout || client.Transport != (responseTestTransport{body: body}) {
				t.Fatal("validation changed original client or lost its settings")
			}
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://provider.test", nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := validated.Do(req)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validation error = %v, wantErr %v", err, tc.wantErr)
			}
			if !body.closed || body.read != min(2+len(tc.suffix), MaxProviderResponseBytes+1) {
				t.Fatalf("body state: closed=%v, bytes=%d", body.closed, body.read)
			}
			if tc.wantErr {
				if resp != nil {
					t.Fatal("invalid envelope returned a response")
				}
				if 2+len(tc.suffix) > MaxProviderResponseBytes && !errors.Is(err, ErrProviderResponseTooLarge) {
					t.Fatalf("oversize cause lost: %v", err)
				}
				return
			}
			data, readErr := io.ReadAll(resp.Body)
			if err := resp.Body.Close(); err != nil {
				t.Fatal(err)
			}
			if readErr != nil || string(data) != "{}"+tc.suffix {
				t.Fatalf("validated replay: %d bytes, error %v", len(data), readErr)
			}
		})
	}
}
