package security

import (
	"reflect"
	"testing"
)

func TestSanitizedEnvironmentDropsSecretsAndSortsKeys(t *testing.T) {
	t.Parallel()
	const (
		locale      = "LC_ALL=C"
		pathBin     = "PATH=/bin"
		pathUserBin = "PATH=/usr/bin"
	)
	got := SanitizedEnvironment([]string{
		"ZEV_PROVIDER_API_KEY=[REDACTED]",
		"LANG=C",
		pathUserBin,
		locale,
		"SSH_AUTH_SOCK=/tmp/agent.sock",
		pathBin,
	})
	want := []string{"LANG=C", locale, pathBin}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SanitizedEnvironment() = %#v, want %#v", got, want)
	}
}

func TestSanitizedEnvironmentRejectsMalformedEntries(t *testing.T) {
	t.Parallel()
	if got := SanitizedEnvironment([]string{"PATH", "=value", "LC_ALL=C"}); !reflect.DeepEqual(got, []string{"LC_ALL=C"}) {
		t.Fatalf("SanitizedEnvironment() = %#v", got)
	}
}
