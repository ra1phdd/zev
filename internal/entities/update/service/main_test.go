package service

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const fixtureMainSHA = "0123456789abcdef0123456789abcdef01234567"

type mainFake struct {
	commit MainCommit
	calls  *[]string
	err    error
}

func (f mainFake) LatestMain(context.Context) (MainCommit, error) {
	*f.calls = append(*f.calls, "main-latest")
	return f.commit, f.err
}

func (f mainFake) BuildMain(_ context.Context, commit MainCommit, version string) ([]byte, error) {
	*f.calls = append(*f.calls, "main-build:"+commit.SHA+":"+version)
	return []byte("main binary"), f.err
}

func TestUpdateBranchMainBuildsInstallsAndMigrates(t *testing.T) {
	t.Parallel()
	var calls []string
	s := New(Params{
		CurrentVersion: latestFixtureVersion,
		CurrentBranch:  Stable,
		Releases:       releaseFake{version: latestFixtureVersion, calls: &calls},
		Main:           mainFake{commit: MainCommit{SHA: fixtureMainSHA}, calls: &calls},
		Installation:   installFake{calls: &calls},
	})
	result, err := s.UpdateBranch(t.Context(), Main)
	if err != nil || !result.Updated || result.Branch != Main || result.Version != "main-"+fixtureMainSHA {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if got := strings.Join(calls, ","); got != "lock,main-latest,main-build:"+fixtureMainSHA+":main-"+fixtureMainSHA+",install,migrate,unlock" {
		t.Fatalf("calls=%s", got)
	}
}

func TestUpdateBranchMainSkipsBuildForSameCommit(t *testing.T) {
	t.Parallel()
	var calls []string
	s := New(Params{
		CurrentVersion: "main-" + fixtureMainSHA,
		CurrentBranch:  Main,
		Releases:       releaseFake{version: latestFixtureVersion, calls: &calls},
		Main:           mainFake{commit: MainCommit{SHA: fixtureMainSHA}, calls: &calls},
		Installation:   installFake{calls: &calls},
	})
	result, err := s.UpdateBranch(t.Context(), Main)
	if err != nil || result.Updated || result.Branch != Main {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if got := strings.Join(calls, ","); got != "lock,main-latest,executable,migrate,unlock" {
		t.Fatalf("calls=%s", got)
	}
}

func TestUpdateBranchMainRejectsInvalidCommitBeforeBuilding(t *testing.T) {
	t.Parallel()
	var calls []string
	s := New(Params{
		CurrentVersion: developmentVersion,
		Releases:       releaseFake{version: latestFixtureVersion, calls: &calls},
		Main:           mainFake{commit: MainCommit{SHA: "../../candidate"}, calls: &calls},
		Installation:   installFake{calls: &calls},
	})
	if _, err := s.UpdateBranch(t.Context(), Main); !errors.Is(err, ErrInvalidRelease) {
		t.Fatalf("err=%v", err)
	}
	if got := strings.Join(calls, ","); got != "lock,main-latest,unlock" {
		t.Fatalf("calls=%s", got)
	}
}

func TestUpdateFromMainInstallsStableRelease(t *testing.T) {
	t.Parallel()
	var calls []string
	s := New(Params{
		CurrentVersion: "main-" + fixtureMainSHA,
		CurrentBranch:  Main,
		Releases:       releaseFake{version: latestFixtureVersion, calls: &calls},
		Installation:   installFake{calls: &calls},
	})
	result, err := s.UpdateBranch(t.Context(), Stable)
	if err != nil || !result.Updated || result.Branch != Stable || result.Version != latestFixtureVersion {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if got := strings.Join(calls, ","); got != stableInstallCalls {
		t.Fatalf("calls=%s", got)
	}
}
