package client

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/ra1phdd/zev/internal/entities/update/service"
)

const testReleaseVersion = "1.2.3"

func TestLatestUsesFixedEndpoint(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/ra1phdd/zev/releases/latest" || r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected request %s", r.URL)
		}
		if _, err := io.WriteString(w, `{"tag_name":"v1.2.3","draft":false,"prerelease":false,"assets":[]}`); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	c := New()
	c.baseURL = server.URL
	c.httpClient.Transport = server.Client().Transport
	r, err := c.Latest(t.Context())
	if err != nil || r.Version != testReleaseVersion {
		t.Fatalf("release=%+v err=%v", r, err)
	}
}

func TestChecksumRequiresExactlyOneMatchingEntry(t *testing.T) {
	t.Parallel()
	data := []byte("fixture")
	name := "zev_1.2.3_linux_amd64.tar.gz"
	sum := sha256.Sum256(data)
	line := fmt.Sprintf("%x  %s\n", sum, name)
	for _, tc := range []struct {
		name, text string
		valid      bool
	}{{"valid", line, true}, {"duplicate", line + line, false}, {"missing", "", false}, {"wrong", strings.Repeat("0", 64) + "  " + name + "\n", false}, {"star", fmt.Sprintf("%x *%s\n", sum, name), true}} {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyChecksum([]byte(tc.text), name, data)
			if (err == nil) != tc.valid {
				t.Fatalf("err=%v valid=%v", err, tc.valid)
			}
		})
	}
}

func testArchive(t *testing.T, name string, kind byte, data []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	h := &tar.Header{Name: name, Mode: 0o755, Typeflag: kind, Size: int64(len(data))}
	if kind != tar.TypeReg {
		h.Size = 0
	}
	if err := tw.WriteHeader(h); err != nil {
		t.Fatal(err)
	}
	if h.Size > 0 {
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestExtractionRejectsUnsafeEntries(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		kind byte
	}{{"../zev", tar.TypeReg}, {"dir/zev", tar.TypeReg}, {"zev", tar.TypeSymlink}, {"zev", tar.TypeLink}, {"/zev", tar.TypeReg}} {
		t.Run(fmt.Sprintf("%s-%d", tc.name, tc.kind), func(t *testing.T) {
			_, err := extractArchive(t.Context(), tc.name+".tar.gz", testArchive(t, tc.name, tc.kind, []byte("fixture")))
			if err == nil {
				t.Fatal("unsafe entry accepted")
			}
		})
	}
	got, err := extractArchive(t.Context(), "zev.tar.gz", testArchive(t, "zev", tar.TypeReg, []byte("fixture")))
	if err != nil || string(got) != "fixture" {
		t.Fatalf("data=%q err=%v", got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := extractTar(ctx, testArchive(t, "zev", tar.TypeReg, nil)); err == nil {
		t.Fatal("canceled extraction accepted")
	}
}

type fixtureTransport struct {
	base      *url.URL
	transport http.RoundTripper
}

func (f fixtureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.URL.Scheme, cloned.URL.Host = f.base.Scheme, f.base.Host
	response, err := f.transport.RoundTrip(cloned)
	if err != nil {
		return nil, fmt.Errorf("fixture request: %w", err)
	}
	return response, nil
}

func TestDownloadFetchesAndVerifiesReleaseArchive(t *testing.T) {
	t.Parallel()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(executable) //nolint:gosec // Reads the current test binary returned by os.Executable for a synthetic archive.
	if err != nil {
		t.Fatal(err)
	}
	archive := testArchive(t, "zev", tar.TypeReg, binary)
	name, err := assetName(testReleaseVersion)
	if err != nil {
		t.Skipf("unsupported fixture platform: %v", err)
	}
	if strings.HasSuffix(name, ".zip") {
		t.Skip("tar fixture; Windows ZIP extraction is a separate path")
	}
	sum := sha256.Sum256(archive)
	for _, valid := range []bool{true, false} {
		t.Run(fmt.Sprintf("valid=%t", valid), func(t *testing.T) {
			checksums := fmt.Sprintf("%x  %s\n", sum, name)
			if !valid {
				checksums = strings.Repeat("0", 64) + "  " + name + "\n"
			}
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "" {
					t.Error("update used authorization")
				}
				var body []byte
				switch r.URL.Path {
				case "/ra1phdd/zev/releases/download/v1.2.3/" + name:
					body = archive
				case "/ra1phdd/zev/releases/download/v1.2.3/checksums.txt":
					body = []byte(checksums)
				default:
					t.Errorf("unexpected download path: %s", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				if _, err := w.Write(body); err != nil {
					t.Error(err)
				}
			}))
			t.Cleanup(server.Close)
			base, err := url.Parse(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			c := New()
			c.httpClient.Transport = fixtureTransport{base: base, transport: server.Client().Transport}
			prefix := "https://github.com/ra1phdd/zev/releases/download/v1.2.3/"
			release := service.Release{Version: testReleaseVersion, TagName: "v1.2.3", Assets: []service.Asset{
				{Name: name, URL: prefix + name, Size: int64(len(archive))},
				{Name: "checksums.txt", URL: prefix + "checksums.txt", Size: int64(len(checksums))},
			}}
			got, err := c.Download(t.Context(), release)
			if valid && (err != nil || !bytes.Equal(got, binary)) {
				t.Fatalf("download changed candidate: %v", err)
			}
			if !valid && (err == nil || got != nil) {
				t.Fatalf("bad checksum returned an executable: %v", err)
			}
		})
	}
}

func TestAssetURLsRejectForeignHostsAndPaths(t *testing.T) {
	t.Parallel()
	name := "checksums.txt"
	r := service.Release{Version: testReleaseVersion, TagName: "v1.2.3"}
	for _, u := range []string{"http://github.com/ra1phdd/zev/releases/download/v1.2.3/checksums.txt", "https://evil.test/checksums.txt", "https://github.com/other/zev/releases/download/v1.2.3/checksums.txt", "https://github.com/ra1phdd/zev/releases/download/v1.2.3/checksums.txt?token=secret"} {
		if validAssetURL(u, r, name) {
			t.Errorf("accepted %s", u)
		}
	}
}
