package service

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCompareVersionsDoesNotOverflow(t *testing.T) {
	t.Parallel()
	cases := []struct {
		a, b string
		want int
	}{
		{"999999999999999999999999.0.0", "999999999999999999999998.0.0", 1},
		{"1.999999999999999999999999.0", "1.9999999999999999999999999.0", -1},
		{latestFixtureVersion, latestFixtureVersion, 0},
	}
	for _, tc := range cases {
		if got := compareVersions(tc.a, tc.b); got != tc.want {
			t.Fatalf("compare(%s,%s)=%d want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestUpdateRejectsInvalidAndCanceledRequests(t *testing.T) {
	t.Parallel()
	for _, version := range []string{"", "main", "1.2", "01.2.3", "1.2.3-beta", "1.2.3+build"} {
		t.Run(version, func(t *testing.T) {
			var calls []string
			s := New(Params{CurrentVersion: version, Releases: releaseFake{version: latestFixtureVersion, calls: &calls}, Installation: installFake{calls: &calls}})
			if _, err := s.Update(t.Context()); err == nil {
				t.Fatal("invalid version accepted")
			}
			if len(calls) != 0 {
				t.Fatal(calls)
			}
		})
	}
	var calls []string
	s := New(Params{CurrentVersion: developmentVersion, Releases: releaseFake{version: latestFixtureVersion, calls: &calls}, Installation: installFake{calls: &calls}})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.Update(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	if len(calls) != 0 {
		t.Fatal(calls)
	}
}

func TestUpdateSourceFailureCannotInstall(t *testing.T) {
	t.Parallel()
	failure := errors.New("fixture request failed")
	var calls []string
	s := New(Params{CurrentVersion: developmentVersion, Releases: releaseFake{calls: &calls, err: failure}, Installation: installFake{calls: &calls}})
	if _, err := s.Update(t.Context()); !errors.Is(err, failure) {
		t.Fatalf("err=%v", err)
	}
	if got := strings.Join(calls, ","); got != "lock,latest,unlock" {
		t.Fatal(got)
	}
}
