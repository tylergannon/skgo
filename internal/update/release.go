package update

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

func downloadLatestRelease(ctx context.Context, dir string) (string, string, error) {
	return downloadRelease(ctx, &http.Client{Timeout: 5 * time.Minute}, "https://api.github.com/repos/tylergannon/skgo/releases/latest", dir, runtime.GOOS+"_"+runtime.GOARCH)
}

// Download only a published release, never a candidate Git tag. The temporary
// executable is verified by Run before it atomically replaces the old CLI.
func downloadRelease(ctx context.Context, client *http.Client, api, dir, target string) (staged, version string, err error) {
	switch target {
	case "darwin_arm64", "linux_amd64", "linux_arm64":
	default:
		return "", "", fmt.Errorf("no SKGO release archive for %s", target)
	}
	body, err := releaseGET(ctx, client, api)
	if err != nil {
		return "", "", err
	}
	var release struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
		Assets     []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	err = json.NewDecoder(body).Decode(&release)
	_ = body.Close()
	if err != nil {
		return "", "", err
	}
	version = release.Tag
	if release.Draft || release.Prerelease || !semver.IsValid(version) || semver.Prerelease(version) != "" || semver.Build(version) != "" {
		return "", "", fmt.Errorf("GitHub did not return a stable published SKGO release")
	}
	archiveName := "skgo_" + strings.TrimPrefix(version, "v") + "_" + target + ".tar.gz"
	var archiveURL, checksumsURL string
	for _, asset := range release.Assets {
		switch asset.Name {
		case archiveName:
			archiveURL = asset.URL
		case "checksums.txt":
			checksumsURL = asset.URL
		}
	}
	if archiveURL == "" || checksumsURL == "" {
		return "", "", fmt.Errorf("release %s is missing %s or checksums.txt", version, archiveName)
	}
	body, err = releaseGET(ctx, client, checksumsURL)
	if err != nil {
		return "", "", err
	}
	sums, err := io.ReadAll(io.LimitReader(body, 1<<20))
	_ = body.Close()
	if err != nil {
		return "", "", err
	}
	var expected string
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == archiveName {
			if expected != "" {
				return "", "", fmt.Errorf("duplicate checksum for %s", archiveName)
			}
			expected = fields[0]
		}
	}
	decoded, err := hex.DecodeString(expected)
	if err != nil || len(decoded) != sha256.Size {
		return "", "", fmt.Errorf("release has no valid SHA-256 for %s", archiveName)
	}
	archive, err := os.CreateTemp(dir, ".skgo-download-*.tar.gz")
	if err != nil {
		return "", "", err
	}
	defer os.Remove(archive.Name())
	body, err = releaseGET(ctx, client, archiveURL)
	if err != nil {
		_ = archive.Close()
		return "", "", err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(archive, hash), body)
	_ = body.Close()
	closeErr := archive.Close()
	if copyErr != nil {
		return "", "", copyErr
	}
	if closeErr != nil {
		return "", "", closeErr
	}
	if hex.EncodeToString(hash.Sum(nil)) != strings.ToLower(expected) {
		return "", "", fmt.Errorf("SHA-256 mismatch for %s; installed CLI is unchanged", archiveName)
	}
	source, err := os.Open(archive.Name())
	if err != nil {
		return "", "", err
	}
	defer source.Close()
	gz, err := gzip.NewReader(source)
	if err != nil {
		return "", "", err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	// Stage on the destination filesystem so Run's rename is atomic.
	binary, err := os.CreateTemp(dir, ".skgo-release-*")
	if err != nil {
		return "", "", err
	}
	staged = binary.Name()
	defer func() {
		_ = binary.Close()
		if err != nil {
			_ = os.Remove(staged)
		}
	}()
	found := false
	for {
		var h *tar.Header
		h, err = tr.Next()
		if err == io.EOF {
			err = nil
			break
		}
		if err != nil {
			return staged, "", err
		}
		if h.Name != "skgo" {
			continue
		}
		if found || h.Typeflag != tar.TypeReg {
			return staged, "", fmt.Errorf("release archive has an invalid or duplicate skgo executable")
		}
		found = true
		if _, err = io.Copy(binary, tr); err != nil {
			return staged, "", err
		}
	}
	if !found {
		return staged, "", fmt.Errorf("release archive has no skgo executable")
	}
	if err = binary.Chmod(0755); err != nil {
		return staged, "", err
	}
	if err = binary.Close(); err != nil {
		return staged, "", err
	}
	return staged, version, nil
}
func releaseGET(ctx context.Context, client *http.Client, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "skgo-update")
	req.Header.Set("Accept", "application/vnd.github+json")
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		return nil, fmt.Errorf("download SKGO release: %s returned %s", url, response.Status)
	}
	return response.Body, nil
}
