package service

import "testing"

func TestMaskAPIKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  string
		want string
	}{
		{
			name: "empty key",
			key:  "",
			want: "",
		},
		{
			name: "short key (less than 6 chars)",
			key:  "abc",
			want: "***",
		},
		{
			name: "normal OpenAI key",
			key:  "sk-proj-1234567890abcdefghijklmnop",
			want: "sk-***...nop",
		},
		{
			name: "normal Gemini key",
			key:  "AIzaSyAbCdEfGhIjKlMnOpQrStUvWxYz",
			want: "AIz***...xYz",
		},
		{
			name: "exactly 6 chars",
			key:  "abcdef",
			want: "abc***...def",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := maskAPIKey(tt.key)
			if got != tt.want {
				t.Errorf("maskAPIKey() = %q, want %q", got, tt.want)
			}
		})
	}
}
