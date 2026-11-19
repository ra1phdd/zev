package config

import (
	"testing"
)

func TestSecurityModeDefaultsAndRoundTrip(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	if cfg.Security.Mode != SecurityModeAuto {
		t.Fatalf("security mode = %q, want %q", cfg.Security.Mode, SecurityModeAuto)
	}

	parsed, err := Parse([]byte(`version: 1
active_provider: openai
providers:
  openai:
    type: openai
security:
  mode: bypass
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if parsed.Security.Mode != SecurityModeBypass {
		t.Fatalf("parsed security mode = %q, want %q", parsed.Security.Mode, SecurityModeBypass)
	}

	cfg.Security.Mode = SecurityModeBypass
	data, err := encodeYAML(cfg)
	if err != nil {
		t.Fatalf("encodeYAML() error = %v", err)
	}
	roundTrip, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse(encoded) error = %v", err)
	}
	if roundTrip.Security.Mode != SecurityModeBypass {
		t.Fatalf("round-tripped security mode = %q, want %q", roundTrip.Security.Mode, SecurityModeBypass)
	}

	cfg.Security.WebFetchEnabled = true
	data, err = encodeYAML(cfg)
	if err != nil {
		t.Fatalf("encodeYAML(web fetch) error = %v", err)
	}
	roundTrip, err = Parse(data)
	if err != nil || !roundTrip.Security.WebFetchEnabled {
		t.Fatalf("web fetch setting did not round-trip: cfg=%+v err=%v", roundTrip, err)
	}
}

func TestLegacySecurityModeNormalIsNormalizedToAuto(t *testing.T) {
	t.Parallel()
	cfg, err := Parse([]byte(`version: 1
active_provider: openai
providers:
  openai:
    type: openai
security:
  mode: normal
`))
	if err != nil {
		t.Fatalf("Parse(legacy normal mode) error = %v", err)
	}
	if cfg.Security.Mode != SecurityModeAuto {
		t.Fatalf("legacy mode normalized to %q, want %q", cfg.Security.Mode, SecurityModeAuto)
	}
}

func TestSecurityModeRejectsUnknownValue(t *testing.T) {
	t.Parallel()
	_, err := Parse([]byte(`version: 1
active_provider: openai
providers:
  openai:
    type: openai
security:
  mode: unsafe
`))
	if err == nil {
		t.Fatal("unknown security mode was accepted")
	}
}

func TestLegacySandboxIsRejected(t *testing.T) {
	t.Parallel()
	_, err := Parse([]byte(`version: 1
active_provider: openai
providers:
  openai:
    type: openai
security:
  sandbox:
    mode: off
`))
	if err == nil {
		t.Fatal("Parse accepted removed security.sandbox setting")
	}
}

func TestLegacySandboxRejectsUnsupportedMode(t *testing.T) {
	t.Parallel()
	_, err := Parse([]byte(`version: 1
active_provider: openai
providers:
  openai:
    type: openai
security:
  sandbox:
    mode: required
`))
	if err == nil {
		t.Fatal("unsupported legacy sandbox mode was accepted")
	}
}

func TestCommandWrapperSettingDefaultsEnabledAndCanBeDisabled(t *testing.T) {
	t.Parallel()
	if !DefaultConfig().CommandWrapperEnabled {
		t.Fatal("command wrapper should be enabled by default")
	}

	cfg, err := Parse([]byte(`version: 1
active_provider: openai
providers:
  openai:
    type: openai
command_wrapper_enabled: false
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if cfg.CommandWrapperEnabled {
		t.Fatal("explicit false command_wrapper_enabled was ignored")
	}
}
