package service

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const (
	latestFixtureVersion = "1.2.3"
	developmentVersion   = "dev"
)

type releaseFake struct {
	version string
	err     error
	calls   *[]string
}

func (f releaseFake) Latest(context.Context) (Release, error) {
	*f.calls = append(*f.calls, "latest")
	return Release{Version: f.version}, f.err
}

func (f releaseFake) Download(context.Context, Release) ([]byte, error) {
	*f.calls = append(*f.calls, "download")
	return []byte("binary"), nil
}

type installFake struct {
	calls      *[]string
	migrateErr error
	installErr error
}

func (f installFake) WithLock(_ context.Context, fn func() error) error {
	*f.calls = append(*f.calls, "lock")
	defer func() { *f.calls = append(*f.calls, "unlock") }()
	return fn()
}

func (f installFake) Executable() (string, error) {
	*f.calls = append(*f.calls, "executable")
	return "/fixture/zev", nil
}

func (f installFake) Install(context.Context, []byte, string) (string, error) {
	*f.calls = append(*f.calls, "install")
	return "/fixture/zev", f.installErr
}

func (f installFake) RunMigrations(context.Context, string) error {
	*f.calls = append(*f.calls, "migrate")
	return f.migrateErr
}

func TestUpdateOrdersInstallationAndMigrations(t *testing.T) {
	t.Parallel()
	var calls []string
	s := New(Params{CurrentVersion: "1.0.0", Releases: releaseFake{version: latestFixtureVersion, calls: &calls}, Installation: installFake{calls: &calls}})
	result, err := s.Update(t.Context())
	if err != nil || !result.Updated || result.Version != latestFixtureVersion {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if got := strings.Join(calls, ","); got != "lock,latest,download,install,migrate,unlock" {
		t.Fatal(got)
	}
}

func TestUpdateNoDowngradeStillMigrates(t *testing.T) {
	t.Parallel()
	for _, current := range []string{latestFixtureVersion, "v2.0.0", "999999999999999999999.0.0"} {
		t.Run(current, func(t *testing.T) {
			var calls []string
			s := New(Params{CurrentVersion: current, Releases: releaseFake{version: latestFixtureVersion, calls: &calls}, Installation: installFake{calls: &calls}})
			result, err := s.Update(t.Context())
			if err != nil || result.Updated {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if got := strings.Join(calls, ","); got != "lock,latest,executable,migrate,unlock" {
				t.Fatal(got)
			}
		})
	}
}

func TestUpdateCommittedInstallationStillMigrates(t *testing.T) {
	t.Parallel()
	failure := errors.New("directory sync uncertain")
	var calls []string
	s := New(Params{CurrentVersion: developmentVersion, Releases: releaseFake{version: latestFixtureVersion, calls: &calls}, Installation: installFake{calls: &calls, installErr: failure}})
	result, err := s.Update(t.Context())
	if !errors.Is(err, failure) || !result.Updated || result.Version != latestFixtureVersion || !strings.Contains(err.Error(), "durability") {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if got := strings.Join(calls, ","); got != "lock,latest,download,install,migrate,unlock" {
		t.Fatal(got)
	}
}

func TestUpdateMigrationFailureRetainsNewVersion(t *testing.T) {
	t.Parallel()
	failure := errors.New("fixture migration failed")
	var calls []string
	s := New(Params{CurrentVersion: developmentVersion, Releases: releaseFake{version: latestFixtureVersion, calls: &calls}, Installation: installFake{calls: &calls, migrateErr: failure}})
	result, err := s.Update(t.Context())
	if !errors.Is(err, failure) || !result.Updated || !strings.Contains(err.Error(), "retained") || !strings.Contains(err.Error(), "zev update --migrate-only") {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}
