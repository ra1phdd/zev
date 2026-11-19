package cli

import (
	"testing"

	"github.com/ra1phdd/zev/internal/core/config"
)

func TestSecurityModeDoesNotOverridePeerToolApprovalMode(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultConfig()
	cfg.Security.Mode = config.SecurityModeBypass
	cfg.ToolApprovalMode = toolModeManual

	enabled, mode := resolveToolSettings(cfg, parsedFlags{ToolApprovalSet: true, ToolApproval: toolModeManual})
	if !enabled {
		t.Fatal("tools unexpectedly disabled")
	}
	if mode != toolModeManual {
		t.Fatalf("tool mode = %q, want %q", mode, toolModeManual)
	}
}

func TestExplicitToolBypassRemainsPeerMode(t *testing.T) {
	t.Parallel()
	cfg := config.DefaultConfig()
	cfg.Security.Mode = config.SecurityModeNormal

	_, mode := resolveToolSettings(cfg, parsedFlags{ToolApprovalSet: true, ToolApproval: toolModeBypass})
	if mode != toolModeBypass {
		t.Fatalf("tool mode = %q, want %q", mode, toolModeBypass)
	}
}
