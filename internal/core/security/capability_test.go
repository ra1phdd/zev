package security

import (
	"path/filepath"
	"testing"
)

func TestCapabilityRequiresEveryEffectAndScope(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	capability := Capability{
		ID:              "tool.exec.readonly",
		Owner:           SourceTool,
		Effects:         []Effect{EffectRead, EffectProcess},
		FilesystemRoots: []string{root},
		Enforcement:     EnforcementBuiltin,
	}
	base := Action{Source: SourceTool, Effects: []Effect{EffectRead}, Paths: []string{filepath.Join(root, "README.md")}}
	if !capability.Covers(base) {
		t.Fatal("capability did not cover its declared read scope")
	}
	base.Effects = append(base.Effects, EffectWrite)
	if capability.Covers(base) {
		t.Fatal("capability partially covered a write effect")
	}
	base.Effects = []Effect{EffectRead}
	base.Paths = []string{filepath.Join(filepath.Dir(root), "outside")}
	if capability.Covers(base) {
		t.Fatal("capability crossed its filesystem root")
	}
}

func TestCapabilityRequiresExplicitNetworkClass(t *testing.T) {
	t.Parallel()
	capability := Capability{ID: "web.public", Owner: SourceWebFetch, Effects: []Effect{EffectNetwork}, NetworkDestinations: []string{"public-http"}, Enforcement: EnforcementApplication}
	action := Action{Source: SourceWebFetch, Effects: []Effect{EffectNetwork}, NetworkDestinations: []string{"public-http"}}
	if !capability.Covers(action) {
		t.Fatal("public network capability did not cover its declared destination class")
	}
	action.NetworkDestinations = []string{"private-ip"}
	if capability.Covers(action) {
		t.Fatal("public network capability covered an undeclared destination class")
	}
}
