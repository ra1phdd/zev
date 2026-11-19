package client

import (
	"strings"
	"testing"
)

func TestBuildDiagnosticBufferIsBounded(t *testing.T) {
	t.Parallel()
	var output buildDiagnosticBuffer
	input := strings.Repeat("x", maxBuildDiagnostics+100)
	if n, err := output.Write([]byte(input)); err != nil || n != len(input) {
		t.Fatalf("Write() = (%d, %v)", n, err)
	}
	if len(output.String()) != maxBuildDiagnostics || !output.Truncated() {
		t.Fatalf("diagnostic length=%d truncated=%t", len(output.String()), output.Truncated())
	}
}

func TestUpdateBuildEnvironmentOverridesAmbientProxyAndBypass(t *testing.T) {
	t.Parallel()
	got := updateBuildEnvironment([]string{
		"PATH=/bin",
		"HTTP_PROXY=http://ambient.invalid:8080",
		"https_proxy=https://ambient.invalid:8443",
		"NO_PROXY=*",
		"no_proxy=localhost",
	}, "socks5h://selected.example:1080")
	joined := "\n" + strings.Join(got, "\n") + "\n"
	for _, want := range []string{
		"PATH=/bin",
		"HTTP_PROXY=socks5h://selected.example:1080",
		"HTTPS_PROXY=socks5h://selected.example:1080",
		"ALL_PROXY=socks5h://selected.example:1080",
		"NO_PROXY=",
		"http_proxy=socks5h://selected.example:1080",
		"https_proxy=socks5h://selected.example:1080",
		"all_proxy=socks5h://selected.example:1080",
		"no_proxy=",
	} {
		if !strings.Contains(joined, "\n"+want+"\n") {
			t.Errorf("build environment missing %q", want)
		}
	}
	for _, forbidden := range []string{"ambient.invalid", "NO_PROXY=*", "no_proxy=localhost"} {
		if strings.Contains(joined, forbidden) {
			t.Errorf("build environment retained %q", forbidden)
		}
	}
}
