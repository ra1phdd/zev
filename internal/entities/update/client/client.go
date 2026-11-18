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
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/ra1phdd/zev/internal/entities/update/service"
)

const (
	defaultBaseURL = "https://api.github.com"
	releasePath    = "/repos/ra1phdd/zev/releases/latest"
	maxHTTPBody    = 32 << 20
	maxBinary      = 64 << 20
	maxExpanded    = 128 << 20
	requestTimeout = 45 * time.Second
)

// Client is the fixed-endpoint GitHub release adapter. Its private seams are for
// local tests only; no provider configuration, tokens, or custom headers are read.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New creates a bounded HTTPS client for public GitHub releases.
func New() *Client {
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 15 * time.Second, IdleConnTimeout: 30 * time.Second, MaxIdleConns: 4}
	return &Client{baseURL: defaultBaseURL, httpClient: &http.Client{Transport: transport, Timeout: requestTimeout, CheckRedirect: checkRedirect}}
}

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
	case "github.com", "api.github.com", "objects.githubusercontent.com", "github-releases.githubusercontent.com", "release-assets.githubusercontent.com":
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
