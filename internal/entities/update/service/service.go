// Package service coordinates release acquisition, replacement, and migrations.
package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrInvalidRelease indicates release metadata or update dependencies are invalid.
var ErrInvalidRelease = errors.New("invalid update release")

// Release is a validated release candidate supplied by a ReleaseSource.
type Release struct {
	Version    string
	TagName    string
	Draft      bool
	Prerelease bool
	Assets     []Asset
}

// Asset identifies a release asset. Client adapters populate its URL and name.
type Asset struct {
	Name string
	URL  string
	Size int64
}

// ReleaseSource obtains and downloads a release candidate.
type ReleaseSource interface {
	Latest(context.Context) (Release, error)
	Download(context.Context, Release) ([]byte, error)
}

// Installation owns executable replacement and post-install migration. Install
// must verify that the candidate reports the supplied release version before
// replacing the target. A nonempty path with an error can indicate a committed
// replacement; callers must not automatically roll back or claim success.
type Installation interface {
	WithLock(context.Context, func() error) error
	Executable() (string, error)
	Install(context.Context, []byte, string) (string, error)
	RunMigrations(context.Context, string) error
}

// Params configures an update Service.
type Params struct {
	CurrentVersion string
	Releases       ReleaseSource
	Installation   Installation
}

// Result describes the update decision and the selected version.
type Result struct {
	Version string
	Updated bool
}

// Service coordinates one locked update attempt. Installation.WithLock must keep
// its lock held for the complete callback, including network and migrations.
type Service struct{ params Params }

// New creates an update service.
func New(params Params) *Service { return &Service{params: params} }

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

func normalizeVersion(value string) (string, error) {
	value = strings.TrimPrefix(value, "v")
	if !versionPattern.MatchString(value) {
		return "", fmt.Errorf("%w: version %q", ErrInvalidRelease, value)
	}
	return value, nil
}

func compareVersions(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	for i := range 3 {
		if len(as[i]) < len(bs[i]) {
			return -1
		}
		if len(as[i]) > len(bs[i]) {
			return 1
		}
		if as[i] < bs[i] {
			return -1
		}
		if as[i] > bs[i] {
			return 1
		}
	}
	return 0
}

func (s *Service) validate() error {
	if s.params.Releases == nil || s.params.Installation == nil {
		return fmt.Errorf("%w: incomplete update dependencies", ErrInvalidRelease)
	}
	if s.params.CurrentVersion == "" {
		return fmt.Errorf("%w: empty current version", ErrInvalidRelease)
	}
	return nil
}

// Update downloads, verifies, installs, and migrates the latest stable release.
// A migration error deliberately leaves the installed candidate in place.
func (s *Service) Update(ctx context.Context) (result Result, err error) {
	if ctx == nil {
		return result, errors.New("update: nil context")
	}
	if err := s.validate(); err != nil {
		return result, err
	}
	if err := ctx.Err(); err != nil {
		return result, fmt.Errorf("update: %w", err)
	}
	current := s.params.CurrentVersion
	currentNormalized := current
	if current != "dev" {
		currentNormalized, err = normalizeVersion(current)
		if err != nil {
			return result, err
		}
	}
	err = s.params.Installation.WithLock(ctx, func() error {
		return s.updateLocked(ctx, current, currentNormalized, &result)
	})
	if err != nil {
		return result, fmt.Errorf("locked update: %w", err)
	}
	return result, nil
}

func (s *Service) updateLocked(ctx context.Context, current, currentNormalized string, result *Result) error {
	release, err := s.params.Releases.Latest(ctx)
	if err != nil {
		return fmt.Errorf("get latest release: %w", err)
	}
	if release.Draft || release.Prerelease {
		return fmt.Errorf("%w: draft or prerelease", ErrInvalidRelease)
	}
	latest, err := normalizeVersion(release.Version)
	if err != nil {
		return err
	}
	if current != "dev" && compareVersions(currentNormalized, latest) >= 0 {
		return s.migrateExisting(ctx, currentNormalized, result)
	}
	data, err := s.params.Releases.Download(ctx, release)
	if err != nil {
		return fmt.Errorf("download %s: %w", latest, err)
	}
	path, err := s.params.Installation.Install(ctx, data, latest)
	if err != nil {
		if path != "" {
			*result = Result{Version: latest, Updated: true}
			migrationErr := s.params.Installation.RunMigrations(ctx, path)
			return errors.Join(
				fmt.Errorf("install %s: replacement committed but durability is uncertain: %w", latest, err),
				migrationErr,
			)
		}
		return fmt.Errorf("install %s: %w", latest, err)
	}
	*result = Result{Version: latest, Updated: true}
	if err := s.params.Installation.RunMigrations(ctx, path); err != nil {
		return fmt.Errorf("run migrations for %s (installed version retained; retry zev update --migrate-only): %w", latest, err)
	}
	return nil
}

func (s *Service) migrateExisting(ctx context.Context, version string, result *Result) error {
	path, err := s.params.Installation.Executable()
	if err != nil {
		return fmt.Errorf("locate installed executable: %w", err)
	}
	if err := s.params.Installation.RunMigrations(ctx, path); err != nil {
		return fmt.Errorf("run migrations for %s (installed version retained; retry zev update --migrate-only): %w", version, err)
	}
	*result = Result{Version: version}
	return nil
}
