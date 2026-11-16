package main

import (
	"bytes"
	"errors"
	"testing"
)

type failAfterOutput struct {
	writesLeft int
	err        error
}

func (w *failAfterOutput) Write(p []byte) (int, error) {
	if w.writesLeft == 0 {
		return 0, w.err
	}
	w.writesLeft--
	return len(p), nil
}

func TestShowHTTPHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{name: "empty", want: "Headers:\n  (none set)\n"},
		{name: "one header", headers: map[string]string{"X-Example": "value:with:colons"}, want: "Headers:\n  X-Example: value:with:colons\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			if err := showHTTPHeaders(&out, "Headers:", tt.headers); err != nil {
				t.Fatal(err)
			}
			if out.String() != tt.want {
				t.Errorf("showHTTPHeaders() output = %q, want %q", out.String(), tt.want)
			}
			for writesLeft := range 2 {
				outputErr := errors.New("header output unavailable")
				out := &failAfterOutput{writesLeft: writesLeft, err: outputErr}
				if err := showHTTPHeaders(out, "Headers:", tt.headers); !errors.Is(err, outputErr) {
					t.Errorf("showHTTPHeaders() after %d writes: error = %v, want output error", writesLeft, err)
				}
			}
		})
	}
}
