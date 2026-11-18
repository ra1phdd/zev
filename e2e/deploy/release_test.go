//go:build e2e

package deploy_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	releaseSuccess = "success"
	releasePush    = "push"
)

func TestReleaseEntrypointsUseGatedPublisher(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Run             string            `yaml:"run"`
				Uses            string            `yaml:"uses"`
				With            map[string]string `yaml:"with"`
				If              string            `yaml:"if"`
				ContinueOnError bool              `yaml:"continue-on-error"` //nolint:tagliatelle // GitHub Actions uses kebab-case keys.
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	var goSetup, gatedPublisher, goreleaserSetup bool
	var installs strings.Builder
	for _, step := range workflow.Jobs["release"].Steps {
		if strings.HasPrefix(step.Uses, "actions/setup-go@") {
			goSetup = step.With["go-version-file"] == "go.mod"
		}
		if strings.HasPrefix(step.Uses, "goreleaser/goreleaser-action@") {
			goreleaserSetup = step.With["install-only"] == "true" && step.With["version"] == "v2.18.3"
			if step.With["args"] != "" {
				t.Fatal("GoReleaser action must only install; the gated script owns the build")
			}
		}
		if strings.Contains(step.Run, "deploy/release.sh") {
			gatedPublisher = step.If == "" && !step.ContinueOnError
		}
		for _, forbidden := range []string{"git push", "gh release", "publish-release.sh", "goreleaser release"} {
			if strings.Contains(step.Run, forbidden) {
				t.Fatalf("workflow bypasses shared gates: %s", step.Run)
			}
		}
		installs.WriteString(step.Run)
		installs.WriteByte('\n')
	}
	if !goSetup || !gatedPublisher || !goreleaserSetup {
		t.Fatalf("release setup/gate missing: Go=%t publisher=%t GoReleaser=%t", goSetup, gatedPublisher, goreleaserSetup)
	}
	for _, tool := range []string{
		"mvdan.cc/gofumpt@v0.12.0",
		"golang.org/x/tools/cmd/goimports@v0.52.0",
		"golang.org/x/tools/cmd/deadcode@v0.52.0",
		"github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0",
	} {
		if !strings.Contains(installs.String(), "go install "+tool) {
			t.Errorf("missing pinned lint tool: %s", tool)
		}
	}
	makefile, err := os.ReadFile(filepath.Join(repoRoot(t), "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	_, releaseSection, ok := strings.Cut(string(makefile), "\nrelease:")
	if !ok {
		t.Fatal("release target missing")
	}
	release, _, _ := strings.Cut(releaseSection, "\n\n")
	if !strings.Contains(release, "bash deploy/release.sh") || strings.Contains(release, "git tag") || strings.Contains(release, "goreleaser release") {
		t.Fatalf("make release bypasses shared gates: %s", release)
	}
}

func TestReleaseGateFailurePreventsTagAndArtifactPublication(t *testing.T) {
	t.Parallel()
	requireBash(t)
	for _, gate := range []string{releaseSuccess, "fmt", "lint", "test", "test-race", "test-e2e", "build", "dirty", "untracked", "untracked-toml", "tag-conflict", "goreleaser", "hook-dirty", "hook-untracked", "hook-untracked-toml", "checksum", releasePush} {
		t.Run(gate, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			bin := filepath.Join(root, "bin")
			mustMkdir(t, bin, filepath.Join(root, "dist"), filepath.Join(root, "deploy"))
			mustWrite(t, filepath.Join(root, "VERSION"), installerVersion)
			archive := "zev_1.2.3_linux_amd64.tar.gz"
			archivePath := filepath.Join(root, "dist", archive)
			mustWrite(t, archivePath, "release fixture")
			writeChecksumFixture(t, filepath.Join(root, "dist"), archive, archivePath, fixtureValid)
			publisher, err := os.ReadFile(filepath.Join(repoRoot(t), "deploy", "publish-release.sh"))
			if err != nil {
				t.Fatal(err)
			}
			mustWrite(t, filepath.Join(root, "deploy", "publish-release.sh"), string(publisher))
			for name, script := range map[string]string{
				"make": `#!/bin/sh
set -eu
printf 'make %s\n' "$*" >> "$ZEV_PUBLICATION_LOG"
[ "$1" != "$ZEV_FAIL_GATE" ]
`,
				"git": `#!/bin/sh
set -eu
case "$1" in
  diff) [ "$ZEV_FAIL_GATE" != dirty ] && [ ! -f DIRTY ]; exit $? ;;
  ls-files)
    if [ "$ZEV_FAIL_GATE" = untracked ] || { [ "$ZEV_FAIL_GATE" = hook-untracked ] && [ -f UNTRACKED ]; }; then printf 'internal/cli/untracked.go\n'; fi
    if [ "$ZEV_FAIL_GATE" = untracked-toml ] || { [ "$ZEV_FAIL_GATE" = hook-untracked-toml ] && [ -f UNTRACKED_TOML ]; }; then printf 'locale/untracked.toml\n'; fi
    exit 0 ;;
  rev-parse)
    if [ "$ZEV_FAIL_GATE" = tag-conflict ] && [ "$2" = v1.2.3 ]; then printf conflicting; exit 0; fi
    if [ "$2" = HEAD ]; then printf head; exit 0; fi
    exit 1 ;;
  rev-list) printf old; exit 0 ;;
  tag|push) printf 'git %s\n' "$*" >> "$ZEV_PUBLICATION_LOG" ;;
  *) exit 2 ;;
esac
if [ "$1" = push ] && [ "$ZEV_FAIL_GATE" = push ]; then exit 1; fi
`,
				"goreleaser": `#!/bin/sh
set -eu
printf 'goreleaser %s\n' "$*" >> "$ZEV_PUBLICATION_LOG"
[ "$ZEV_FAIL_GATE" != goreleaser ]
if [ "$ZEV_FAIL_GATE" = checksum ]; then printf corrupt > dist/zev_1.2.3_linux_amd64.tar.gz; fi
if [ "$ZEV_FAIL_GATE" = hook-dirty ]; then printf dirty > DIRTY; fi
if [ "$ZEV_FAIL_GATE" = hook-untracked ]; then printf untracked > UNTRACKED; fi
if [ "$ZEV_FAIL_GATE" = hook-untracked-toml ]; then printf untracked > UNTRACKED_TOML; fi
`,
				"gh": `#!/bin/sh
set -eu
printf 'gh %s\n' "$*" >> "$ZEV_PUBLICATION_LOG"
`,
			} {
				path := filepath.Join(bin, name)
				mustWrite(t, path, script)
				mustChmod(t, path)
			}
			log := filepath.Join(root, "calls")
			cmd := exec.CommandContext(t.Context(), "bash", filepath.Join(repoRoot(t), "deploy", "release.sh"), installerVersion) //nolint:gosec // Executes the repository-owned release driver with offline fixture tools.
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "ZEV_PUBLICATION_LOG="+log, "ZEV_FAIL_GATE="+gate)
			output, err := cmd.CombinedOutput()
			if (err != nil) != (gate != releaseSuccess) {
				t.Fatalf("release error = %v, output: %s", err, output)
			}
			calls, err := os.ReadFile(log) //nolint:gosec // Log path is private to this test's temporary directory.
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			published := strings.Contains(string(calls), "git push") || strings.Contains(string(calls), "gh release")
			preTagGate := gate == "fmt" || gate == "lint" || gate == "test" || gate == "test-race" || gate == "test-e2e" || gate == "build" || gate == "dirty" || gate == "untracked" || gate == "untracked-toml" || gate == "tag-conflict"
			if preTagGate && strings.Contains(string(calls), "git tag") {
				t.Fatalf("failed %s gate created a tag: %s", gate, calls)
			}
			if gate != releaseSuccess && gate != releasePush && published {
				t.Fatalf("failed %s gate published: %s", gate, calls)
			}
			if gate == releasePush && strings.Contains(string(calls), "gh release") {
				t.Fatalf("failed tag push published artifacts: %s", calls)
			}
			if gate == releaseSuccess {
				want := fmt.Sprintf("make fmt\nmake lint\nmake test\nmake test-race\nmake test-e2e\nmake build VERSION=%s\ngit tag -a v%s -m Release v%s\ngoreleaser release --clean --skip=publish\ngit push origin v%s\ngh release create v%s dist/checksums.txt --draft --verify-tag --title v%s --generate-notes\ngh release upload v%s dist/%s\ngh release edit v%s --draft=false\n", installerVersion, installerVersion, installerVersion, installerVersion, installerVersion, installerVersion, installerVersion, archive, installerVersion)
				if string(calls) != want {
					t.Fatalf("release sequence:\n%s\nwant:\n%s", calls, want)
				}
			}
		})
	}
}
