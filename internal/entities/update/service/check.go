package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const devVersion = "dev"

// CheckResult describes the latest version found on one update channel.
type CheckResult struct {
	Branch  Branch
	Version string
	Current string
	Update  bool
}

// UpdateAvailable compares a channel version with the installed version.
func UpdateAvailable(branch Branch, current, latest string) bool {
	if current == "" || latest == "" {
		return false
	}
	if branch == Main {
		return commitPattern.MatchString(strings.TrimPrefix(latest, "main-")) && current != latest
	}
	if branch != Stable {
		return false
	}
	if current == devVersion {
		return true
	}
	// Stable releases cannot be ordered against main commits from the
	// available metadata, so do not advertise a possible downgrade.
	if strings.HasPrefix(current, "main-") {
		return false
	}
	installed, err := normalizeVersion(current)
	if err != nil {
		return false
	}
	latestVersion, err := normalizeVersion(latest)
	return err == nil && compareVersions(installed, latestVersion) < 0
}

// CheckLatest checks the specified channel without downloading or installing it.
func CheckLatest(ctx context.Context, source interface {
	ReleaseSource
	MainSource
}, branch Branch, current string,
) (CheckResult, error) {
	if ctx == nil {
		return CheckResult{}, errors.New("check updates: nil context")
	}
	if source == nil {
		return CheckResult{}, errors.New("check updates: missing source")
	}
	if branch == Main {
		commit, err := source.LatestMain(ctx)
		if err != nil {
			return CheckResult{}, fmt.Errorf("check main updates: %w", err)
		}
		if !commitPattern.MatchString(commit.SHA) {
			return CheckResult{}, fmt.Errorf("%w: invalid main commit", ErrInvalidRelease)
		}
		version := "main-" + commit.SHA
		return CheckResult{Branch: Main, Version: version, Current: current, Update: UpdateAvailable(Main, current, version)}, nil
	}
	if branch != Stable {
		return CheckResult{}, fmt.Errorf("%w: unsupported branch %q", ErrInvalidRelease, branch)
	}
	release, err := source.Latest(ctx)
	if err != nil {
		return CheckResult{}, fmt.Errorf("check stable updates: %w", err)
	}
	latest, err := normalizeVersion(release.Version)
	if err != nil {
		return CheckResult{}, err
	}
	if current != "dev" && !strings.HasPrefix(current, "main-") {
		if _, err := normalizeVersion(current); err != nil {
			return CheckResult{}, err
		}
	}
	return CheckResult{Branch: Stable, Version: latest, Current: current, Update: UpdateAvailable(Stable, current, latest)}, nil
}
