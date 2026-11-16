package deploy_test

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	installerVersion = "1.2.3"
	fixtureRegular   = "regular"
	fixtureMalformed = "malformed"
	fixtureDuplicate = "duplicate"
	fixtureMismatch  = "mismatch"
	fixtureValid     = "valid"
	fixtureSymlink   = "symlink"
	fixtureTraversal = "traversal"
	fixtureSentinel  = "keep me"
	fixtureDirEnv    = "ZEV_FIXTURE_DIR"
)

func TestShellInstallerRejectsInvalidReleaseInputsBeforeInstall(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		checksums  string
		archive    string
		hideHasher bool
	}{
		{name: "missing checksum", archive: fixtureRegular},
		{name: "malformed checksum", checksums: fixtureMalformed, archive: fixtureRegular},
		{name: "duplicate checksum", checksums: fixtureDuplicate, archive: fixtureRegular},
		{name: "checksum mismatch", checksums: fixtureMismatch, archive: fixtureRegular},
		{name: "missing hash tool", checksums: fixtureValid, archive: fixtureRegular, hideHasher: true},
		{name: "archive symlink", checksums: fixtureValid, archive: fixtureSymlink},
		{name: "archive traversal", checksums: fixtureValid, archive: fixtureTraversal},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := runShellFixture(t, test.checksums, test.archive, test.hideHasher)
			if result.err == nil {
				t.Fatalf("installer succeeded for invalid fixture; output:\n%s", result.output)
			}
			if got := readFixtureSentinel(t, result.prefix); got != fixtureSentinel {
				t.Fatalf("installer changed target after failure: %q", got)
			}
			if strings.Contains(result.output, "Checksum verified") &&
				(test.checksums == "" || test.checksums == fixtureMalformed || test.checksums == fixtureMismatch || test.hideHasher) {
				t.Fatalf("installer claimed checksum verification without verification:\n%s", result.output)
			}
			if strings.Contains(result.installLog, "install") {
				t.Fatalf("installer copied a binary after failure: %s", result.installLog)
			}
		})
	}
}

func TestShellInstallerVerifiesAndInstallsFixture(t *testing.T) {
	t.Parallel()
	result := runShellFixture(t, fixtureValid, fixtureRegular, false)
	if result.err != nil {
		t.Fatalf("installer failed: %v\n%s", result.err, result.output)
	}
	if got := readFixtureSentinel(t, result.prefix); got != fixtureSentinel {
		t.Fatalf("installer changed sentinel: %q", got)
	}
	installed, err := os.ReadFile(filepath.Join(result.prefix, "zev"))
	if err != nil {
		t.Fatalf("read installed binary: %v", err)
	}
	if string(installed) != "fixture binary" {
		t.Fatalf("installed unexpected binary: %q", installed)
	}
	if !strings.Contains(result.output, "Checksum verified") {
		t.Fatalf("successful installation did not report verification:\n%s", result.output)
	}
}

func TestShellInstallerRejectsVersionInjection(t *testing.T) {
	t.Parallel()
	result := runShellFixtureWithVersion(t, "1.2.3/../../evil")
	if result.err == nil {
		t.Fatalf("installer accepted an unsafe version; output:\n%s", result.output)
	}
	if !strings.Contains(result.output, "invalid version") {
		t.Fatalf("installer did not report invalid version:\n%s", result.output)
	}
	if got := readFixtureSentinel(t, result.prefix); got != fixtureSentinel {
		t.Fatalf("installer changed target after version rejection: %q", got)
	}
	if result.installLog != "" {
		t.Fatalf("installer attempted install after version rejection: %s", result.installLog)
	}
}

func TestPowerShellInstallerRejectsInvalidReleaseInputsBeforeInstall(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		checksums string
		archive   string
	}{
		{name: "missing checksum", archive: fixtureRegular},
		{name: "malformed checksum", checksums: fixtureMalformed, archive: fixtureRegular},
		{name: "duplicate checksum", checksums: fixtureDuplicate, archive: fixtureRegular},
		{name: "checksum mismatch", checksums: fixtureMismatch, archive: fixtureRegular},
		{name: "archive symlink", checksums: fixtureValid, archive: fixtureSymlink},
		{name: "archive traversal", checksums: fixtureValid, archive: fixtureTraversal},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := runPowerShellFixture(t, test.checksums, test.archive, installerVersion)
			if result.err == nil {
				t.Fatalf("installer succeeded for invalid fixture; output:\n%s", result.output)
			}
			if got := readFixtureSentinel(t, result.prefix); got != fixtureSentinel {
				t.Fatalf("installer changed target after failure: %q", got)
			}
			if test.checksums != fixtureValid && strings.Contains(result.output, "Checksum verified") {
				t.Fatalf("installer claimed checksum verification for invalid checksum fixture:\n%s", result.output)
			}
		})
	}
}

func TestPowerShellInstallerVerifiesAndInstallsFixture(t *testing.T) {
	t.Parallel()
	result := runPowerShellFixture(t, fixtureValid, fixtureRegular, installerVersion)
	if result.err != nil {
		t.Fatalf("installer failed: %v\n%s", result.err, result.output)
	}
	if got := readFixtureSentinel(t, result.prefix); got != fixtureSentinel {
		t.Fatalf("installer changed sentinel: %q", got)
	}
	installed, err := os.ReadFile(filepath.Join(result.prefix, "zev.exe"))
	if err != nil {
		t.Fatalf("read installed binary: %v", err)
	}
	if string(installed) != "fixture binary" {
		t.Fatalf("installed unexpected binary: %q", installed)
	}
	if !strings.Contains(result.output, "Checksum verified") {
		t.Fatalf("successful installation did not report verification:\n%s", result.output)
	}
}

func TestPowerShellInstallerRejectsVersionInjection(t *testing.T) {
	t.Parallel()
	result := runPowerShellFixture(t, fixtureValid, fixtureRegular, "1.2.3/../../evil")
	if result.err == nil {
		t.Fatalf("installer accepted an unsafe version; output:\n%s", result.output)
	}
	if !strings.Contains(result.output, "invalid version") {
		t.Fatalf("installer did not report invalid version:\n%s", result.output)
	}
	if got := readFixtureSentinel(t, result.prefix); got != fixtureSentinel {
		t.Fatalf("installer changed target after version rejection: %q", got)
	}
}

type fixtureResult struct {
	prefix     string
	installLog string
	output     string
	err        error
}

func runShellFixture(t *testing.T, checksumKind, archiveKind string, hideHasher bool) fixtureResult {
	t.Helper()
	root := t.TempDir()
	fixtureDir := filepath.Join(root, "downloads")
	binDir := filepath.Join(root, "bin")
	prefix := filepath.Join(root, "prefix")
	mustMkdir(t, fixtureDir, binDir, prefix)
	mustWrite(t, filepath.Join(prefix, "sentinel"), fixtureSentinel)

	archivePath := filepath.Join(fixtureDir, fmt.Sprintf("zev_%s_linux_amd64.tar.gz", installerVersion))
	mustWriteTar(t, archivePath, archiveKind, "zev")
	writeChecksumFixture(t, fixtureDir, filepath.Base(archivePath), archivePath, checksumKind)
	mustWrite(t, filepath.Join(binDir, "curl"), shellDownloader)
	mustWrite(t, filepath.Join(binDir, "install"), shellInstallerMock)
	mustWrite(t, filepath.Join(binDir, "uname"), "#!/bin/sh\ncase \"$1\" in -s) printf Linux;; -m) printf x86_64;; esac\n")
	mustChmod(t, filepath.Join(binDir, "uname"))
	if hideHasher {
		mustWrite(t, filepath.Join(binDir, "sha256sum"), "#!/bin/sh\nexit 127\n")
		mustWrite(t, filepath.Join(binDir, "shasum"), "#!/bin/sh\nexit 127\n")
	}
	mustChmod(t, filepath.Join(binDir, "curl"), filepath.Join(binDir, "install"), filepath.Join(binDir, "sha256sum"), filepath.Join(binDir, "shasum"))

	path := binDir + string(os.PathListSeparator) + os.Getenv("PATH")
	if hideHasher {
		path = binDir + string(os.PathListSeparator) + "/usr/bin:/bin"
	}
	return executeFixture(t, filepath.Join(repoRoot(t), "deploy", "install.sh"), []string{"-v", installerVersion}, map[string]string{
		"PATH":                path,
		"PREFIX":              prefix,
		fixtureDirEnv:         fixtureDir,
		"ZEV_INSTALL_LOG":     filepath.Join(root, "install.log"),
		"ZEV_TEST_NO_NETWORK": "1",
	})
}

func runShellFixtureWithVersion(t *testing.T, version string) fixtureResult {
	t.Helper()
	root := t.TempDir()
	fixtureDir := filepath.Join(root, "downloads")
	binDir := filepath.Join(root, "bin")
	prefix := filepath.Join(root, "prefix")
	mustMkdir(t, fixtureDir, binDir, prefix)
	mustWrite(t, filepath.Join(prefix, "sentinel"), fixtureSentinel)
	mustWrite(t, filepath.Join(binDir, "curl"), shellDownloader)
	mustWrite(t, filepath.Join(binDir, "install"), shellInstallerMock)
	mustWrite(t, filepath.Join(binDir, "uname"), "#!/bin/sh\ncase \"$1\" in -s) printf Linux;; -m) printf x86_64;; esac\n")
	mustChmod(t, filepath.Join(binDir, "uname"))
	mustChmod(t, filepath.Join(binDir, "curl"), filepath.Join(binDir, "install"))
	return executeFixture(t, filepath.Join(repoRoot(t), "deploy", "install.sh"), []string{"-v", version}, map[string]string{
		"PATH":            binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"PREFIX":          prefix,
		fixtureDirEnv:     fixtureDir,
		"ZEV_INSTALL_LOG": filepath.Join(root, "install.log"),
	})
}

func runPowerShellFixture(t *testing.T, checksumKind, archiveKind, version string) fixtureResult {
	t.Helper()
	if _, err := exec.LookPath("pwsh"); err != nil {
		t.Fatalf("pwsh is required for installer fixtures: %v", err)
	}
	root := t.TempDir()
	fixtureDir := filepath.Join(root, "downloads")
	prefix := filepath.Join(root, "prefix")
	mustMkdir(t, fixtureDir, prefix)
	mustWrite(t, filepath.Join(prefix, "sentinel"), fixtureSentinel)
	archivePath := filepath.Join(fixtureDir, fmt.Sprintf("zev_%s_windows_amd64.zip", installerVersion))
	mustWriteZip(t, archivePath, archiveKind)
	writeChecksumFixture(t, fixtureDir, filepath.Base(archivePath), archivePath, checksumKind)

	wrapper := filepath.Join(root, "run.ps1")
	mustWrite(t, wrapper, fmt.Sprintf(`$ErrorActionPreference = 'Stop'
function Invoke-WebRequest {
    param([Parameter(Position=0)][string]$Uri, [string]$OutFile, [switch]$UseBasicParsing)
    $name = ($Uri -split '/')[-1]
    $source = Join-Path $env:ZEV_FIXTURE_DIR $name
    if (-not (Test-Path -LiteralPath $source)) { throw "fixture missing: $name" }
    if ($OutFile) { Copy-Item -LiteralPath $source -Destination $OutFile -Force }
    else { [PSCustomObject]@{ Content = [System.IO.File]::ReadAllText($source) } }
}
& '%s' -Version $env:ZEV_TEST_VERSION -Prefix $env:ZEV_PREFIX
`, strings.ReplaceAll(filepath.Join(repoRoot(t), "deploy", "install.ps1"), "'", "''")))
	return executeFixture(t, wrapper, nil, map[string]string{
		"PROCESSOR_ARCHITECTURE": "AMD64",
		"PROCESSOR_ARCHITEW6432": "AMD64",
		fixtureDirEnv:            fixtureDir,
		"ZEV_PREFIX":             prefix,
		"ZEV_TEST_VERSION":       version,
	})
}

func executeFixture(t *testing.T, script string, args []string, extra map[string]string) fixtureResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmdArgs := append([]string{script}, args...)
	cmd := exec.CommandContext(ctx, "bash", cmdArgs...) //nolint:gosec // Executes repository-owned fixtures with generated paths, never user input.
	if strings.HasSuffix(script, ".ps1") || filepath.Ext(script) == ".ps1" || strings.HasSuffix(script, "run.ps1") {
		cmd = exec.CommandContext(ctx, "pwsh", append([]string{"-NoLogo", "-NoProfile", "-File", script}, args...)...) //nolint:gosec // Executes a generated test wrapper, never user input.
	}
	env := os.Environ()
	for key, value := range extra {
		env = append(env, key+"="+value)
	}
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	installLog := ""
	if path := extra["ZEV_INSTALL_LOG"]; path != "" {
		data, readErr := os.ReadFile(path) //nolint:gosec // Test fixture path is created under t.TempDir.
		if readErr == nil {
			installLog = string(data)
		}
	}
	prefix := extra["PREFIX"]
	if prefix == "" {
		prefix = extra["ZEV_PREFIX"]
	}
	return fixtureResult{prefix: prefix, installLog: installLog, output: string(output), err: err}
}

func writeChecksumFixture(t *testing.T, dir, archiveName, archivePath, kind string) {
	t.Helper()
	if kind == "" {
		return
	}
	data, err := os.ReadFile(archivePath) //nolint:gosec // Test fixture path is created under t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	valid := hex.EncodeToString(digest[:])
	line := valid + "  " + archiveName + "\n"
	switch kind {
	case fixtureDuplicate:
		line += line
	case fixtureMismatch:
		line = strings.Repeat("0", 64) + "  " + archiveName + "\n"
	case fixtureMalformed:
		line = "not-a-checksum  " + archiveName + "\n"
	case fixtureValid:
	default:
		t.Fatalf("unknown checksum fixture %q", kind)
	}
	mustWrite(t, filepath.Join(dir, "checksums.txt"), line)
}

func mustWriteTar(t *testing.T, path, kind, binaryName string) {
	t.Helper()
	file, err := os.Create(path) //nolint:gosec // Test fixture path is created under t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	name := binaryName
	header := &tar.Header{Name: name, Mode: 0o755, Size: int64(len("fixture binary")), Typeflag: tar.TypeReg}
	contents := []byte("fixture binary")
	switch kind {
	case fixtureSymlink:
		header.Typeflag = tar.TypeSymlink
		header.Size = 0
		header.Linkname = "target"
	case fixtureTraversal:
		header.Name = "../escape"
	case fixtureRegular:
	default:
		t.Fatalf("unknown tar fixture %q", kind)
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if header.Typeflag == tar.TypeReg {
		if _, err := tarWriter.Write(contents); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func mustWriteZip(t *testing.T, path, kind string) {
	t.Helper()
	file, err := os.Create(path) //nolint:gosec // Test fixture path is created under t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	name := "zev.exe"
	header := &zip.FileHeader{Name: name, Method: zip.Store}
	header.SetMode(0o755)
	contents := []byte("fixture binary")
	switch kind {
	case fixtureSymlink:
		header.SetMode(os.ModeSymlink | 0o777)
	case fixtureTraversal:
		header = &zip.FileHeader{Name: "../escape", Method: zip.Store}
	case fixtureRegular:
	default:
		t.Fatalf("unknown zip fixture %q", kind)
	}
	entry, err := writer.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	if kind == fixtureRegular {
		if _, err := entry.Write(contents); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func readFixtureSentinel(t *testing.T, prefix string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(prefix, "sentinel")) //nolint:gosec // Prefix is a generated t.TempDir fixture, not user input.
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func mustMkdir(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
}

func mustChmod(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			if err := os.Chmod(path, 0o700); err != nil { //nolint:gosec // Fixture executables need owner execute permission.
				t.Fatal(err)
			}
		}
	}
}

func mustWrite(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil { //nolint:gosec // Test fixture path is created under t.TempDir.
		t.Fatal(err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(workingDirectory)
}

const shellDownloader = `#!/bin/sh
set -eu
url=''
out=''
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) out=$2; shift 2 ;;
    *) url=$1; shift ;;
  esac
done
name=${url##*/}
source_file="$ZEV_FIXTURE_DIR/$name"
[ -f "$source_file" ]
if [ -n "$out" ]; then cp "$source_file" "$out"; else cat "$source_file"; fi
`

const shellInstallerMock = `#!/bin/sh
set -eu
printf 'install %s\n' "$*" >> "$ZEV_INSTALL_LOG"
case "$1" in
  -d) mkdir -p "$2" ;;
  -m) cp "$3" "$4"; chmod "$2" "$4" ;;
  *) exit 2 ;;
esac
`
