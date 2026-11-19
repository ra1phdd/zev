package service

import (
	"context"
	"testing"
)

const stableLatestVersion = "1.1.0"

type checkSourceFake struct {
	release Release
	commit  MainCommit
	err     error
}

func (f checkSourceFake) Latest(context.Context) (Release, error) {
	return f.release, f.err
}

func (f checkSourceFake) Download(context.Context, Release) ([]byte, error) {
	return nil, nil
}

func (f checkSourceFake) LatestMain(context.Context) (MainCommit, error) {
	return f.commit, f.err
}

func (f checkSourceFake) BuildMain(context.Context, MainCommit, string) ([]byte, error) {
	return nil, nil
}

func TestCheckLatest(t *testing.T) {
	t.Parallel()
	const sha = "0123456789abcdef0123456789abcdef01234567"
	for _, tc := range []struct {
		name    string
		branch  Branch
		current string
		source  checkSourceFake
		want    string
		update  bool
		wantErr bool
	}{
		{name: "stable update", branch: Stable, current: "1.0.0", source: checkSourceFake{release: Release{Version: stableLatestVersion}}, want: stableLatestVersion, update: true},
		{name: "stable current", branch: Stable, current: stableLatestVersion, source: checkSourceFake{release: Release{Version: stableLatestVersion}}, want: stableLatestVersion},
		{name: "stable newer", branch: Stable, current: "2.0.0", source: checkSourceFake{release: Release{Version: stableLatestVersion}}, want: stableLatestVersion},
		{name: "main update", branch: Main, current: "main-old", source: checkSourceFake{commit: MainCommit{SHA: sha}}, want: "main-" + sha, update: true},
		{name: "main current", branch: Main, current: "main-" + sha, source: checkSourceFake{commit: MainCommit{SHA: sha}}, want: "main-" + sha},
		{name: "invalid main commit", branch: Main, current: "main-old", source: checkSourceFake{commit: MainCommit{SHA: "bad"}}, wantErr: true},
		{name: "invalid branch", branch: "other", source: checkSourceFake{}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CheckLatest(t.Context(), tc.source, tc.branch, tc.current)
			if (err != nil) != tc.wantErr {
				t.Fatalf("CheckLatest() error = %v, wantErr %t", err, tc.wantErr)
			}
			if err == nil && (got.Version != tc.want || got.Update != tc.update || got.Branch != tc.branch) {
				t.Fatalf("CheckLatest() = %+v", got)
			}
		})
	}
}
