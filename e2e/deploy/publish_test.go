//go:build e2e

package deploy_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleasePublicationOrderAndFailures(t *testing.T) {
	t.Parallel()
	requireBash(t)
	for _, scenario := range []struct {
		name       string
		corrupt    bool
		failUpload bool
		checkOnly  bool
		wantCalls  int
	}{
		{name: releaseSuccess, wantCalls: 3},
		{name: "checksum gate verifies without publishing", checkOnly: true},
		{name: "corrupt archive", corrupt: true},
		{name: "checksum gate does not publish", corrupt: true, checkOnly: true},
		{name: "failed upload remains draft", failUpload: true, wantCalls: 2},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			dist := filepath.Join(root, "dist")
			bin := filepath.Join(root, "bin")
			mustMkdir(t, dist, bin)
			archive := "zev_1.2.3_linux_amd64.tar.gz"
			mustWrite(t, filepath.Join(dist, archive), "release fixture")
			digest := sha256.Sum256([]byte("release fixture"))
			if scenario.corrupt {
				digest = sha256.Sum256([]byte("different bytes"))
			}
			mustWrite(t, filepath.Join(dist, "checksums.txt"), fmt.Sprintf("%x  %s\n", digest, archive))
			log := filepath.Join(root, "calls")
			mustWrite(t, filepath.Join(bin, "gh"), `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$ZEV_PUBLICATION_LOG"
if [ "$2" = upload ] && [ "$ZEV_FAIL_UPLOAD" = true ]; then exit 1; fi
`)
			mustChmod(t, filepath.Join(bin, "gh"))
			script := filepath.Join(repoRoot(t), "deploy", "publish-release.sh")
			args := []string{installerVersion}
			if scenario.checkOnly {
				args = []string{"--check", installerVersion}
			}
			cmd := exec.CommandContext(t.Context(), "bash", append([]string{script}, args...)...) //nolint:gosec // Executes a repository-owned script against temporary fixtures.
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "ZEV_PUBLICATION_LOG="+log, fmt.Sprintf("ZEV_FAIL_UPLOAD=%t", scenario.failUpload))
			output, err := cmd.CombinedOutput()
			if (err != nil) != (scenario.corrupt || scenario.failUpload) {
				t.Fatalf("publication error = %v, output: %s", err, output)
			}
			calls, readErr := os.ReadFile(log) //nolint:gosec // Log path is private to this test's temporary directory.
			if readErr != nil && !os.IsNotExist(readErr) {
				t.Fatal(readErr)
			}
			lines := strings.Split(strings.TrimSpace(string(calls)), "\n")
			if len(calls) == 0 {
				lines = nil
			}
			if len(lines) != scenario.wantCalls {
				t.Fatalf("calls = %q, want %d", lines, scenario.wantCalls)
			}
			if len(lines) > 0 && (!strings.HasPrefix(lines[0], "release create v1.2.3 dist/checksums.txt ") || !strings.Contains(lines[0], "--draft")) {
				t.Fatalf("first publication must upload checksums to a draft: %s", lines[0])
			}
			if len(lines) > 1 && !strings.HasPrefix(lines[1], "release upload v1.2.3 dist/"+archive) {
				t.Fatalf("unexpected archive upload: %s", lines[1])
			}
			if len(lines) > 2 && lines[2] != "release edit v1.2.3 --draft=false" {
				t.Fatalf("unexpected final publication: %s", lines[2])
			}
		})
	}
}
