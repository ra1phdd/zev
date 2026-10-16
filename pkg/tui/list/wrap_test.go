package list

import (
	"testing"
)

func TestWrapText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		text     string
		maxWidth int
		indent   int
		want     string
	}{
		{
			name:     "short text no wrap needed",
			text:     "Hello",
			maxWidth: 20,
			indent:   4,
			want:     "Hello",
		},
		{
			name:     "exact width no wrap",
			text:     "Hello World",
			maxWidth: 11,
			indent:   4,
			want:     "Hello World",
		},
		{
			name:     "simple wrap with space",
			text:     "Hello World Test",
			maxWidth: 12,
			indent:   4,
			want:     "Hello World\n    Test",
		},
		{
			name:     "wrap without space break",
			text:     "VeryLongWordWithoutSpaces",
			maxWidth: 10,
			indent:   2,
			want:     "VeryLongWo\n  rdWithou\n  tSpaces",
		},
		{
			name:     "multiple wraps",
			text:     "This is a very long command that should wrap multiple times",
			maxWidth: 20,
			indent:   4,
			want:     "This is a very long\n    command that\n    should wrap\n    multiple times",
		},
		{
			name:     "zero indent",
			text:     "Hello World Test Again",
			maxWidth: 12,
			indent:   0,
			want:     "Hello World\nTest Again",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := wrapText(tt.text, tt.maxWidth, tt.indent)
			if got != tt.want {
				t.Errorf("wrapText() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}
