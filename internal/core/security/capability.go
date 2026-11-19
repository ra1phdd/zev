package security

import (
	"fmt"
	"path/filepath"
	"slices"
)

// Enforcement identifies the boundary that actually restricts a capability.
type Enforcement string

const (
	// EnforcementOS means the operating system enforces the boundary.
	EnforcementOS Enforcement = "os"
	// EnforcementBuiltin means a bounded application builtin enforces the boundary.
	EnforcementBuiltin Enforcement = "restricted_builtin"
	// EnforcementApplication means application checks enforce the boundary.
	EnforcementApplication Enforcement = "application"
)

// Capability is the concrete capability exposed by one effect adapter.
// Descriptions supplied by a model or plugin are never capabilities.
type Capability struct {
	ID                  string
	Owner               Source
	Effects             []Effect
	FilesystemRoots     []string
	NetworkDestinations []string
	CredentialSources   []string
	Enforcement         Enforcement
}

// Covers reports whether the adapter can enforce every requested effect and
// scope in an action. A missing descriptor is an enforcement failure, not an
// implicit allow.
//
//nolint:gocognit // Scope coverage is deliberately explicit and fail-closed.
func (c Capability) Covers(action Action) bool {
	if c.ID == "" || c.Owner != action.Source || c.Enforcement == "" {
		return false
	}
	for _, effect := range action.Effects {
		if !slices.Contains(c.Effects, effect) {
			return false
		}
	}
	if len(action.Paths) > 0 {
		if len(c.FilesystemRoots) == 0 {
			return false
		}
		for _, path := range action.Paths {
			if !withinAnyRoot(path, c.FilesystemRoots) {
				return false
			}
		}
	}
	if len(action.NetworkDestinations) > 0 {
		if len(c.NetworkDestinations) == 0 {
			return false
		}
		for _, destination := range action.NetworkDestinations {
			if !slices.Contains(c.NetworkDestinations, destination) {
				return false
			}
		}
	}
	return true
}

// CapabilitySet is the immutable set of capabilities wired into one service.
type CapabilitySet []Capability

// HostActionCapability describes an intentionally unconfined host dispatch.
// It records that the application boundary is the only enforcement layer; it
// does not claim filesystem, network, or process isolation.
func HostActionCapability(action Action) Capability {
	return Capability{
		ID:          "host." + string(action.Source) + "." + string(action.Delivery),
		Owner:       action.Source,
		Effects:     append([]Effect(nil), action.Effects...),
		Enforcement: EnforcementApplication,
	}
}

// Require fails closed when no wired adapter covers the action.
func (s CapabilitySet) Require(action Action) error {
	for _, capability := range s {
		if capability.Covers(action) {
			return nil
		}
	}
	return fmt.Errorf("no enforced capability covers %s action", action.Source)
}

func withinAnyRoot(value string, roots []string) bool {
	cleanValue, err := filepath.Abs(filepath.Clean(value))
	if err != nil {
		return false
	}
	for _, root := range roots {
		cleanRoot, rootErr := filepath.Abs(filepath.Clean(root))
		if rootErr != nil {
			continue
		}
		relative, relErr := filepath.Rel(cleanRoot, cleanValue)
		if relErr == nil && relative != ".." && relative != ".."+string(filepath.Separator) && !isOutsidePath(relative) {
			return true
		}
	}
	return false
}

func isOutsidePath(relative string) bool {
	return len(relative) >= 3 && relative[:3] == ".."+string(filepath.Separator)
}
