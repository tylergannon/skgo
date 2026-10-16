package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/buildinfo"
)

func TestDownloadPublishedPlatformArchivePreservesBytes(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "skgo")
	if err := os.WriteFile(old, []byte("old signed fixture"), 0755); err != nil {
		t.Fatal(err)
	}
	server := releaseFixture(t, "success")
	defer server.Close()
	staged, version, err := downloadRelease(context.Background(), server.Client(), server.URL+"/latest", dir, "darwin_arm64")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(staged)
	if version != "v1.2.3" || filepath.Dir(staged) != dir {
		t.Fatalf("staged %s (%s)", staged, version)
	}
	data, err := os.ReadFile(staged)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "signed executable fixture" {
		t.Fatalf("released executable bytes changed: %q", data)
	}
	stat, err := os.Stat(staged)
	if err != nil || stat.Mode().Perm() != 0755 {
		t.Fatalf("not executable: %v %v", stat, err)
	}
	data, err = os.ReadFile(old)
	if err != nil || string(data) != "old signed fixture" {
		t.Fatal("replaced CLI before version verification")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 2 {
		t.Fatalf("download debris: %v %v", entries, err)
	}
}
func TestInvalidReleaseNeverReplacesExistingCLI(t *testing.T) {
	for _, tc := range []struct{ mode, message string }{{"corrupt", "SHA-256 mismatch"}, {"missing-binary", "no skgo executable"}, {"missing-asset", "missing skgo_1.2.3_darwin_arm64.tar.gz"}, {"draft", "stable published"}, {"unavailable", "404"}} {
		t.Run(tc.mode, func(t *testing.T) {
			dir := t.TempDir()
			old := filepath.Join(dir, "skgo")
			if err := os.WriteFile(old, []byte("old signed fixture"), 0755); err != nil {
				t.Fatal(err)
			}
			server := releaseFixture(t, tc.mode)
			defer server.Close()
			_, _, err := downloadRelease(context.Background(), server.Client(), server.URL+"/latest", dir, "darwin_arm64")
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("invalid release allowed: %v", err)
			}
			data, err := os.ReadFile(old)
			if err != nil || string(data) != "old signed fixture" {
				t.Fatal("invalid download damaged installed CLI")
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("private/staging debris after failure: %v %v", entries, err)
			}
		})
	}
}
func TestWrongDownloadedVersionCannotReplaceCLI(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "skgo")
	stage := filepath.Join(dir, ".downloaded-skgo")
	for path, data := range map[string]string{old: "old signed fixture", stage: "different release fixture"} {
		if err := os.WriteFile(path, []byte(data), 0755); err != nil {
			t.Fatal(err)
		}
	}
	err := Run(context.Background(), Options{info: func() (buildinfo.Info, error) { return buildinfo.Info{Executable: old, SkgoVersion: "v1.0.0"}, nil }, release: func(context.Context, string) (string, string, error) { return stage, "v1.2.3", nil }, run: func(_ context.Context, c command) ([]byte, error) {
		if c.Name == "go" && len(c.Args) > 0 && c.Args[0] == "env" {
			return json.Marshal(map[string]string{"GOBIN": dir})
		}
		if c.Name == stage {
			return []byte(`{"skgoVersion":"v1.2.2"}`), nil
		}
		t.Fatalf("ran unverified replacement: %+v", c)
		return nil, nil
	}})
	if err == nil || !strings.Contains(err.Error(), "expected stable release v1.2.3") {
		t.Fatal(err)
	}
	data, err := os.ReadFile(old)
	if err != nil || string(data) != "old signed fixture" {
		t.Fatal("wrong version replaced signed CLI")
	}
	if _, err = os.Stat(stage); !os.IsNotExist(err) {
		t.Fatal("unverified staging executable retained")
	}
}
func releaseFixture(t *testing.T, mode string) *httptest.Server {
	t.Helper()
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	name := "skgo"
	if mode == "missing-binary" {
		name = "README.md"
	}
	fixture := []byte("signed executable fixture")
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: int64(len(fixture)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(fixture); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	checksum := fmt.Sprintf("%x  skgo_1.2.3_darwin_arm64.tar.gz\n", sha256.Sum256(archive.Bytes()))
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			if mode == "unavailable" {
				http.NotFound(w, r)
				return
			}
			assets := []map[string]string{{"name": "skgo_1.2.3_darwin_arm64.tar.gz", "browser_download_url": server.URL + "/cli.tar.gz"}, {"name": "checksums.txt", "browser_download_url": server.URL + "/checksums.txt"}}
			if mode == "missing-asset" {
				assets = assets[:1]
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v1.2.3", "draft": mode == "draft", "prerelease": false, "assets": assets})
		case "/checksums.txt":
			_, _ = w.Write([]byte(checksum))
		case "/cli.tar.gz":
			if mode == "corrupt" {
				_, _ = w.Write([]byte("corrupt download"))
				return
			}
			_, _ = w.Write(archive.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	return server
}
