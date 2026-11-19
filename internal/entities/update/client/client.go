// Package client downloads stable releases from the fixed GitHub repository.
package client

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/ra1phdd/zev/internal/core/httpclient"
	"github.com/ra1phdd/zev/internal/entities/update/service"
)

const (
	defaultBaseURL      = "https://api.github.com"
	releasePath         = "/repos/ra1phdd/zev/releases/latest"
	maxHTTPBody         = 32 << 20
	maxSource           = 64 << 20
	maxBinary           = 64 << 20
	maxExpanded         = 128 << 20
	maxBuildDiagnostics = 16 << 10
	requestTimeout      = 45 * time.Second
)

type buildDiagnosticBuffer struct {
	bytes.Buffer
	truncated bool
}

func (b *buildDiagnosticBuffer) Write(data []byte) (int, error) {
	length := len(data)
	remaining := maxBuildDiagnostics - b.Len()
	if remaining > 0 {
		if len(data) > remaining {
			data = data[:remaining]
		}
		_, _ = b.Buffer.Write(data)
	}
	if len(data) < length {
		b.truncated = true
	}
	return length, nil
}

func (b *buildDiagnosticBuffer) Truncated() bool { return b.truncated }

// Client is the fixed-endpoint GitHub release adapter. Its private seams are for
// local tests only; no provider configuration, tokens, or custom headers are read.
type Client struct {
	baseURL    string
	httpClient *http.Client
	proxyURL   string
}

// New creates a bounded HTTPS client for public GitHub releases.
func New() *Client {
	client, err := NewWithProxy("")
	if err == nil {
		return client
	}
	return &Client{baseURL: defaultBaseURL, httpClient: &http.Client{Transport: updateErrorTransport{err: fmt.Errorf("configure update proxy: %w", err)}, Timeout: requestTimeout, CheckRedirect: checkRedirect}}
}

// NewWithProxy creates a bounded release client that uses proxyURL, or the
// environment default when it is empty. NO_PROXY is ignored for fail-closed
// routing once a proxy is selected.
func NewWithProxy(proxyURL string) (*Client, error) {
	resolvedProxy, err := httpclient.ResolveProxy(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("resolve update proxy: %w", err)
	}
	transport := &http.Transport{DialContext: (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 15 * time.Second, IdleConnTimeout: 30 * time.Second, MaxIdleConns: 4}
	if err := httpclient.ConfigureProxy(transport, resolvedProxy); err != nil {
		return nil, fmt.Errorf("configure update proxy: %w", err)
	}
	return &Client{baseURL: defaultBaseURL, proxyURL: resolvedProxy, httpClient: &http.Client{Transport: transport, Timeout: requestTimeout, CheckRedirect: checkRedirect}}, nil
}

type updateErrorTransport struct{ err error }

func (t updateErrorTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, t.err }

type githubRelease struct {
	TagName    string        `json:"tag_name"` //nolint:tagliatelle // GitHub's wire format uses snake_case.
	Draft      bool          `json:"draft"`
	Prerelease bool          `json:"prerelease"`
	Assets     []githubAsset `json:"assets"`
}
type githubAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"` //nolint:tagliatelle // GitHub's wire format uses snake_case.
	Size int64  `json:"size"`
}

type githubCommit struct {
	SHA string `json:"sha"`
}

var commitSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// Latest obtains the latest stable metadata from the fixed repository.
func (c *Client) Latest(ctx context.Context) (service.Release, error) {
	if ctx == nil {
		return service.Release{}, errors.New("latest release: nil context")
	}
	body, err := c.get(ctx, c.baseURL+releasePath)
	if err != nil {
		return service.Release{}, fmt.Errorf("request latest release: %w", err)
	}
	var gr githubRelease
	if err := json.Unmarshal(body, &gr); err != nil {
		return service.Release{}, fmt.Errorf("decode latest release: %w", err)
	}
	version := strings.TrimPrefix(gr.TagName, "v")
	if gr.Draft || gr.Prerelease || gr.TagName != "v"+version || !versionPattern.MatchString(version) {
		return service.Release{}, service.ErrInvalidRelease
	}
	assets := make([]service.Asset, len(gr.Assets))
	for i, a := range gr.Assets {
		assets[i] = service.Asset{Name: a.Name, URL: a.URL, Size: a.Size}
	}
	return service.Release{Version: version, TagName: gr.TagName, Assets: assets}, nil
}

// LatestMain returns the current commit at the fixed development branch.
func (c *Client) LatestMain(ctx context.Context) (service.MainCommit, error) {
	if ctx == nil {
		return service.MainCommit{}, errors.New("latest main commit: nil context")
	}
	body, err := c.get(ctx, c.baseURL+"/repos/ra1phdd/zev/commits/main")
	if err != nil {
		return service.MainCommit{}, fmt.Errorf("request latest main commit: %w", err)
	}
	var commit githubCommit
	if err := json.Unmarshal(body, &commit); err != nil {
		return service.MainCommit{}, fmt.Errorf("decode latest main commit: %w", err)
	}
	if !commitSHA.MatchString(commit.SHA) {
		return service.MainCommit{}, service.ErrInvalidRelease
	}
	return service.MainCommit{SHA: commit.SHA}, nil
}

// BuildMain downloads and compiles the exact main-branch commit locally.
func (c *Client) BuildMain(ctx context.Context, commit service.MainCommit, version string) (binary []byte, err error) {
	if ctx == nil {
		return nil, errors.New("build main: nil context")
	}
	if !commitSHA.MatchString(commit.SHA) || version != "main-"+commit.SHA {
		return nil, service.ErrInvalidRelease
	}
	source, err := c.get(ctx, "https://github.com/ra1phdd/zev/archive/"+commit.SHA+".tar.gz")
	if err != nil {
		return nil, fmt.Errorf("download main source: %w", err)
	}
	root, cleanup, err := extractSource(ctx, source)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cleanupErr := cleanup(); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("remove main source directory: %w", cleanupErr))
		}
	}()
	output, err := os.CreateTemp("", ".zev-main-build-*")
	if err != nil {
		return nil, fmt.Errorf("create main build output: %w", err)
	}
	outputPath := output.Name()
	if err := output.Close(); err != nil {
		return nil, errors.Join(
			fmt.Errorf("close main build output: %w", err),
			removeFile(outputPath),
		)
	}
	defer func() {
		if removeErr := removeFile(outputPath); removeErr != nil {
			err = errors.Join(err, fmt.Errorf("remove main build output: %w", removeErr))
		}
	}()
	binary, err = c.compileMain(ctx, root, outputPath, version, commit.SHA)
	return binary, err
}

func (c *Client) compileMain(ctx context.Context, root, outputPath, version, commitSHA string) ([]byte, error) {
	//nolint:gosec // The executable, arguments, version format, and output path are validated/generated above.
	cmd := exec.CommandContext(ctx, "go", "build", "-trimpath", "-ldflags", "-s -w -X main.version="+version+" -buildid=", "-o", outputPath, "./cmd/zev")
	cmd.Dir = root
	cmd.Env = os.Environ()
	if c.proxyURL != "" {
		cmd.Env = updateBuildEnvironment(cmd.Env, c.proxyURL)
	}
	cmd.Stdout = io.Discard
	var diagnostics buildDiagnosticBuffer
	cmd.Stderr = &diagnostics
	if runtime.GOARCH == "arm" {
		cmd.Env = append(cmd.Env, "GOARM=7")
	}
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(diagnostics.String())
		if diagnostics.Truncated() {
			detail += " [diagnostics truncated]"
		}
		if detail != "" {
			return nil, fmt.Errorf("compile main commit %s: %w: %s", commitSHA, err, detail)
		}
		return nil, fmt.Errorf("compile main commit %s: %w", commitSHA, err)
	}
	binary, err := os.ReadFile(outputPath) //nolint:gosec // Output path is created by this process.
	if err != nil {
		return nil, fmt.Errorf("read main build output: %w", err)
	}
	if len(binary) == 0 || len(binary) > maxBinary {
		return nil, errors.New("main build output exceeds size limit")
	}
	return binary, nil
}

func updateBuildEnvironment(environment []string, proxyURL string) []string {
	proxyNames := map[string]struct{}{
		"http_proxy": {}, "https_proxy": {}, "all_proxy": {}, "no_proxy": {},
	}
	result := make([]string, 0, len(environment)+8)
	for _, entry := range environment {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, isProxy := proxyNames[strings.ToLower(key)]; isProxy {
			continue
		}
		result = append(result, entry)
	}
	result = append(result, "HTTP_PROXY="+proxyURL, "HTTPS_PROXY="+proxyURL, "ALL_PROXY="+proxyURL, "NO_PROXY=")
	if runtime.GOOS != "windows" {
		result = append(result, "http_proxy="+proxyURL, "https_proxy="+proxyURL, "all_proxy="+proxyURL, "no_proxy=")
	}
	return result
}

func extractSource(ctx context.Context, data []byte) (string, func() error, error) {
	if len(data) > maxSource {
		return "", noopCleanup, errors.New("main source archive exceeds size limit")
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return "", noopCleanup, fmt.Errorf("open main source archive: %w", err)
	}
	dir, err := os.MkdirTemp("", ".zev-main-source-*")
	if err != nil {
		return "", noopCleanup, errors.Join(fmt.Errorf("create main source directory: %w", err), gz.Close())
	}
	if err := extractSourceEntries(ctx, gz, dir); err != nil {
		return sourceFailure(dir, errors.Join(err, gz.Close()))
	}
	if err := gz.Close(); err != nil {
		return sourceFailure(dir, fmt.Errorf("close main source archive: %w", err))
	}
	root, err := findSourceModule(dir)
	if err != nil {
		return sourceFailure(dir, fmt.Errorf("locate main source module: %w", err))
	}
	return root, func() error { return os.RemoveAll(dir) }, nil
}

func removeFile(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("remove %q: %w", path, err)
	}
	return nil
}

func noopCleanup() error { return nil }

func sourceFailure(dir string, err error) (string, func() error, error) {
	return "", noopCleanup, errors.Join(err, os.RemoveAll(dir))
}

func extractSourceEntries(ctx context.Context, source io.Reader, dir string) error {
	tr := tar.NewReader(io.LimitReader(source, maxExpanded+1))
	expanded := int64(0)
	for entries := 0; ; entries++ {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("extract main source: %w", err)
		}
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read main source archive: %w", err)
		}
		if entries >= 8192 {
			return errors.New("main source archive contains an unsafe entry")
		}
		target, err := sourceTarget(dir, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o700); err != nil {
				return fmt.Errorf("create main source directory: %w", err)
			}
		case tar.TypeReg:
			written, err := extractSourceFile(tr, target, header.Size, expanded)
			if err != nil {
				return err
			}
			expanded = written
		default:
			return errors.New("main source archive contains an unsafe entry")
		}
	}
}

func sourceTarget(dir, archiveName string) (string, error) {
	name := filepath.Clean(filepath.FromSlash(archiveName))
	if filepath.IsAbs(name) || name == "." || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
		return "", errors.New("main source archive contains an unsafe entry")
	}
	target := filepath.Join(dir, name)
	if !strings.HasPrefix(target, dir+string(filepath.Separator)) {
		return "", errors.New("main source archive escapes its directory")
	}
	return target, nil
}

func extractSourceFile(source io.Reader, target string, size, expanded int64) (int64, error) {
	if size < 0 || size > maxExpanded-expanded {
		return expanded, errors.New("main source archive exceeds expanded size limit")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return expanded, fmt.Errorf("create main source parent: %w", err)
	}
	//nolint:gosec // sourceTarget confines archive paths to the private extraction directory.
	file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return expanded, fmt.Errorf("create main source file: %w", err)
	}
	_, copyErr := io.CopyN(file, source, size)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		return expanded, errors.Join(copyErr, closeErr)
	}
	return expanded + size, nil
}

func findSourceModule(dir string) (string, error) {
	var root string
	if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && path != dir {
			if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
				if root != "" {
					return errors.New("main source archive has multiple Go modules")
				}
				root = path
			}
		}
		return nil
	}); err != nil {
		return "", fmt.Errorf("walk main source directory: %w", err)
	}
	if root == "" {
		return "", errors.New("main source archive has no Go module")
	}
	return root, nil
}

// Download verifies the archive SHA256 before extracting a single root binary.
// Checksums from the same release detect corruption, not a compromised publisher.
func (c *Client) Download(ctx context.Context, release service.Release) ([]byte, error) {
	if ctx == nil {
		return nil, errors.New("download release: nil context")
	}
	version := strings.TrimPrefix(release.Version, "v")
	if !versionPattern.MatchString(version) || release.TagName != "v"+version || release.Draft || release.Prerelease {
		return nil, service.ErrInvalidRelease
	}
	archiveName, err := assetName(version)
	if err != nil {
		return nil, err
	}
	archive, err := selectAsset(release, archiveName)
	if err != nil {
		return nil, err
	}
	checksum, err := selectAsset(release, "checksums.txt")
	if err != nil {
		return nil, err
	}
	sumData, err := c.get(ctx, checksum.URL)
	if err != nil {
		return nil, fmt.Errorf("download checksums: %w", err)
	}
	data, err := c.get(ctx, archive.URL)
	if err != nil {
		return nil, fmt.Errorf("download archive: %w", err)
	}
	if err := verifyChecksum(sumData, archiveName, data); err != nil {
		return nil, err
	}
	binary, err := extractArchive(ctx, archiveName, data)
	if err != nil {
		return nil, err
	}
	if err := validateBinary(binary, version); err != nil {
		return nil, err
	}
	return binary, nil
}

func selectAsset(release service.Release, name string) (service.Asset, error) {
	var selected service.Asset
	count := 0
	for _, a := range release.Assets {
		if a.Name == name {
			selected = a
			count++
		}
	}
	if count != 1 || selected.Size < 0 || selected.Size > maxHTTPBody || !validAssetURL(selected.URL, release, name) {
		return service.Asset{}, fmt.Errorf("release asset %s is missing, duplicated, oversized, or unsafe", name)
	}
	return selected, nil
}

func (c *Client) get(ctx context.Context, raw string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, fmt.Errorf("build release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "zev-update")
	resp, err := c.httpClient.Do(req) //nolint:bodyclose // readBody closes the response body and joins close errors.
	if err != nil {
		return nil, fmt.Errorf("get release resource: %w", err)
	}
	return readBody(resp)
}

func readBody(resp *http.Response) (data []byte, err error) {
	if resp == nil || resp.Body == nil {
		return nil, errors.New("release response has no body")
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close release response: %w", closeErr))
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("release server returned HTTP %d", resp.StatusCode)
	}
	data, err = io.ReadAll(io.LimitReader(resp.Body, maxHTTPBody+1))
	if err != nil {
		return nil, fmt.Errorf("read release response: %w", err)
	}
	if len(data) > maxHTTPBody {
		return nil, errors.New("release response exceeds size limit")
	}
	return data, nil
}

func checkRedirect(req *http.Request, via []*http.Request) error {
	u := req.URL
	if len(via) >= 5 || u.Scheme != "https" || u.User != nil || u.Port() != "" || !allowedHost(u.Hostname()) {
		return errors.New("release redirect outside bounded GitHub HTTPS hosts")
	}
	return nil
}

func allowedHost(host string) bool {
	switch host {
	case "github.com", "api.github.com", "codeload.github.com", "objects.githubusercontent.com", "github-releases.githubusercontent.com", "release-assets.githubusercontent.com":
		return true
	}
	return false
}

func validAssetURL(raw string, r service.Release, name string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Host != "github.com" || u.RawPath != "" {
		return false
	}
	want := "/ra1phdd/zev/releases/download/v" + strings.TrimPrefix(r.Version, "v") + "/" + name
	return u.Path == want
}

func verifyChecksum(text []byte, name string, data []byte) error {
	sum := sha256.Sum256(data)
	want := hex.EncodeToString(sum[:])
	matches := 0
	scan := bufio.NewScanner(bytes.NewReader(text))
	for scan.Scan() {
		fields := strings.Fields(scan.Text())
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != name {
			continue
		}
		matches++
		if !strings.EqualFold(fields[0], want) {
			return errors.New("release checksum mismatch")
		}
	}
	if err := scan.Err(); err != nil {
		return fmt.Errorf("read checksums: %w", err)
	}
	if matches != 1 {
		return errors.New("release checksum entry count is not exactly one")
	}
	return nil
}

func assetName(v string) (string, error) {
	arch := runtime.GOARCH
	if arch == "arm" { //nolint:nestif // ARM assets exist only for a confirmed GOARM=7 Linux binary.
		if runtime.GOOS != "linux" {
			return "", errors.New("unsupported update platform")
		}
		info, ok := debug.ReadBuildInfo()
		if !ok {
			return "", errors.New("cannot determine running GOARM")
		}
		goarm := ""
		for _, s := range info.Settings {
			if s.Key == "GOARM" {
				goarm = s.Value
			}
		}
		if goarm != "7" && goarm != "7,hardfloat" {
			return "", errors.New("self-update requires a GOARM=7 installation")
		}
		arch = "armv7"
	} else if arch != "amd64" && arch != "arm64" {
		return "", errors.New("unsupported update architecture")
	}
	extension := ".tar.gz"
	switch runtime.GOOS {
	case "windows":
		extension = ".zip"
	case "linux", "darwin":
	default:
		return "", errors.New("unsupported update operating system")
	}
	return "zev_" + v + "_" + runtime.GOOS + "_" + arch + extension, nil
}

func validateBinary(data []byte, version string) error {
	info, err := buildinfo.Read(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("release binary has no Go build metadata: %w", err)
	}
	settings := make(map[string]string, len(info.Settings))
	for _, s := range info.Settings {
		settings[s.Key] = s.Value
	}
	if settings["GOOS"] != runtime.GOOS || settings["GOARCH"] != runtime.GOARCH {
		return errors.New("release binary platform mismatch")
	}
	// Trimpath releases can omit ldflags; the native protocol probe remains mandatory.
	for field := range strings.FieldsSeq(settings["-ldflags"]) {
		if strings.HasPrefix(field, "main.version=") && strings.TrimPrefix(field, "main.version=") != version {
			return errors.New("release binary version mismatch")
		}
	}
	return nil
}

func extractArchive(ctx context.Context, name string, data []byte) (out []byte, err error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("extract release: %w", err)
	}
	if strings.HasSuffix(name, ".zip") {
		return extractZip(ctx, data)
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("open release archive: %w", err)
	}
	defer func() { err = errors.Join(err, gz.Close()) }()
	// Bound actual decompression, including padding and ignored documentation.
	expanded, err := io.ReadAll(io.LimitReader(gz, maxExpanded+1))
	if err != nil {
		return nil, fmt.Errorf("decompress release: %w", err)
	}
	if len(expanded) > maxExpanded {
		return nil, errors.New("expanded release archive exceeds size limit")
	}
	return extractTar(ctx, expanded)
}

func allowedDocument(name string) bool {
	return name == "README.md" || name == "README.ru.md" || name == "LICENSE"
}

func extractTar(ctx context.Context, data []byte) ([]byte, error) {
	tr := tar.NewReader(bytes.NewReader(data))
	var out []byte
	entries := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("extract release: %w", err)
		}
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read release archive: %w", err)
		}
		entries++
		if entries > 4 || h.Typeflag != tar.TypeReg || (!allowedDocument(h.Name) && h.Name != "zev") {
			return nil, errors.New("release archive contains an unsafe entry")
		}
		if h.Name != "zev" {
			continue
		}
		if out != nil || h.Size <= 0 || h.Size > maxBinary {
			return nil, errors.New("release binary duplicated, empty, or oversized")
		}
		out, err = io.ReadAll(io.LimitReader(tr, maxBinary+1))
		if err != nil || int64(len(out)) != h.Size {
			return nil, errors.New("invalid release binary size")
		}
	}
	if len(out) == 0 {
		return nil, errors.New("release archive has no binary")
	}
	return out, nil
}

func extractZip(ctx context.Context, data []byte) (out []byte, err error) {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open release zip: %w", err)
	}
	if len(z.File) > 4 {
		return nil, errors.New("release archive contains too many entries")
	}
	for _, f := range z.File {
		if !f.Mode().IsRegular() || (!allowedDocument(f.Name) && f.Name != "zev.exe") {
			return nil, errors.New("release archive contains an unsafe entry")
		}
		if f.Name != "zev.exe" {
			continue
		}
		if out != nil || f.UncompressedSize64 == 0 || f.UncompressedSize64 > maxBinary {
			return nil, errors.New("release binary duplicated, empty, or oversized")
		}
		out, err = readZipBinary(ctx, f)
		if err != nil {
			return nil, err
		}
	}
	if len(out) == 0 {
		return nil, errors.New("release archive has no binary")
	}
	return out, nil
}

func readZipBinary(ctx context.Context, f *zip.File) (out []byte, err error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("extract release: %w", err)
	}
	r, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("open zip binary: %w", err)
	}
	defer func() { err = errors.Join(err, r.Close()) }()
	out, err = io.ReadAll(io.LimitReader(r, maxBinary+1))
	if err != nil {
		return nil, fmt.Errorf("read zip binary: %w", err)
	}
	if len(out) > maxBinary {
		return nil, errors.New("release binary exceeds size limit")
	}
	return out, nil
}
