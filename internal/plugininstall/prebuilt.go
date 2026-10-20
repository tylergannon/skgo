package plugininstall

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tylergannon/skgo/internal/buildinfo"
	"github.com/tylergannon/skgo/internal/pluginstore"
	"github.com/tylergannon/skgo/internal/templates"
	"golang.org/x/mod/semver"
)

// PinsFile is the project-owned declaration of exact prebuilt plugins.
const PinsFile = "skgo-plugins.json"

// Prebuilt pins trusted executable code and its generator configuration. Artifact
// is an HTTPS URL or a local file; SHA256 authenticates the bytes before loading.
// A plugin release targets exactly one official skgo release.
type Prebuilt struct {
	ID          string            `json:"id"`
	Version     string            `json:"version"`
	SkgoVersion string            `json:"skgoVersion"`
	Artifact    string            `json:"artifact"`
	SHA256      string            `json:"sha256"`
	Options     map[string]string `json:"options,omitempty"`
	Selections  []string          `json:"selections,omitempty"`
}

type projectPins struct {
	Plugins []Prebuilt `json:"plugins"`
}

var artifactPattern = regexp.MustCompile(`^([a-z][a-z0-9-]*)-(v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?)-skgo-(v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?)-(darwin|linux)-(arm64|amd64)\.so$`)

// ParseArtifact derives release identity from the required artifact name. It
// does not fetch the artifact or infer a checksum.
func ParseArtifact(artifact string) (Prebuilt, error) {
	name, err := artifactName(artifact)
	if err != nil {
		return Prebuilt{}, err
	}
	m := artifactPattern.FindStringSubmatch(name)
	if m == nil || !semver.IsValid(m[2]) || !semver.IsValid(m[3]) {
		return Prebuilt{}, fmt.Errorf("artifact must be named ID-vX.Y.Z-skgo-vX.Y.Z-OS-ARCH.so: %s", name)
	}
	return Prebuilt{ID: m[1], Version: m[2], SkgoVersion: m[3], Artifact: artifact}, nil
}

func artifactName(artifact string) (string, error) {
	u, err := url.Parse(artifact)
	if err != nil {
		return "", err
	}
	if u.Scheme != "" {
		if u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
			return "", fmt.Errorf("artifact must be an HTTPS URL or a local file")
		}
		return path.Base(u.Path), nil
	}
	return filepath.Base(artifact), nil
}

func (p Prebuilt) validate() error {
	identity, err := ParseArtifact(p.Artifact)
	if err != nil {
		return err
	}
	if identity.ID != p.ID || identity.Version != p.Version || identity.SkgoVersion != p.SkgoVersion {
		return fmt.Errorf("plugin pin identity does not match artifact name")
	}
	sum, err := hex.DecodeString(p.SHA256)
	if err != nil || len(sum) != 32 || p.SHA256 != strings.ToLower(p.SHA256) {
		return fmt.Errorf("plugin %s needs a lowercase SHA256 digest", p.ID)
	}
	return nil
}

// ReadPins reads only this project's declared providers. Missing declarations
// mean no prebuilt plugins; malformed or duplicate declarations fail explicitly.
func ReadPins(cwd string) ([]Prebuilt, error) {
	f, err := os.Open(filepath.Join(cwd, PinsFile))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var pins projectPins
	d := json.NewDecoder(f)
	d.DisallowUnknownFields()
	if err = d.Decode(&pins); err != nil {
		return nil, fmt.Errorf("%s: %w", PinsFile, err)
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("%s must contain one object", PinsFile)
	}
	seen := map[string]bool{}
	for _, p := range pins.Plugins {
		if err := p.validate(); err != nil {
			return nil, err
		}
		if seen[p.ID] {
			return nil, fmt.Errorf("duplicate pinned plugin %q", p.ID)
		}
		seen[p.ID] = true
	}
	return pins.Plugins, nil
}

func prebuiltPath(p Prebuilt, o Options) (string, error) {
	if err := p.validate(); err != nil {
		return "", err
	}
	if o.Host.SkgoVersion != p.SkgoVersion {
		return "", fmt.Errorf("plugin %s %s requires official skgo %s; executing host is %s", p.ID, p.Version, p.SkgoVersion, o.Host.SkgoVersion)
	}
	if o.Host.Build == nil || o.Host.Build.Main.Path != buildinfo.Module || o.Host.Build.Main.Version != p.SkgoVersion {
		return "", fmt.Errorf("prebuilt plugins require the matching official skgo executable; project-built go tool hosts are unsupported")
	}
	settings := map[string]string{}
	for _, s := range o.Host.Build.Settings {
		settings[s.Key] = s.Value
	}
	if settings["-trimpath"] != "true" || settings["CGO_ENABLED"] != "1" {
		return "", fmt.Errorf("prebuilt plugins require the cgo-enabled, -trimpath official skgo executable; project-built go tool hosts are unsupported")
	}
	name, _ := artifactName(p.Artifact)
	if !strings.HasSuffix(name, "-"+settings["GOOS"]+"-"+settings["GOARCH"]+".so") {
		return "", fmt.Errorf("plugin artifact %s does not target executing host %s/%s", name, settings["GOOS"], settings["GOARCH"])
	}
	digest, err := pluginstore.Digest(o.Host.Executable)
	if err != nil {
		return "", err
	}
	return filepath.Join(o.Home, ".skgo", "plugins", "prebuilt", digest, p.SHA256, name), nil
}

// InstalledPath returns this pin's host-specific cached path only when the bytes
// still match its checksum. It never downloads, builds, or runs shell setup.
func InstalledPath(p Prebuilt, o Options) (string, error) {
	file, err := prebuiltPath(p, o)
	if err != nil {
		return "", err
	}
	digest, err := pluginstore.Digest(file)
	if err != nil {
		return "", fmt.Errorf("plugin %s unavailable; run skgo plugin restore: %w", p.ID, err)
	}
	if digest != p.SHA256 {
		return "", fmt.Errorf("installed plugin %s checksum mismatch; run skgo plugin restore", p.ID)
	}
	return file, nil
}

// InstallPrebuilt verifies and probes an exact artifact with the executing host,
// then pins it in the project. Installation does not execute plugin commands.
func InstallPrebuilt(ctx context.Context, p Prebuilt, o Options) error {
	if o.Out == nil {
		o.Out = io.Discard
	}
	if _, err := prebuiltPath(p, o); err != nil {
		return err
	}
	pins, err := ReadPins(o.CWD)
	if err != nil {
		return err
	}
	replaced := false
	for i := range pins {
		if pins[i].ID == p.ID {
			pins[i] = p
			replaced = true
		}
	}
	if !replaced {
		pins = append(pins, p)
	}
	if err := installPrebuilt(ctx, p, pins, o); err != nil {
		return err
	}
	// Store local staging paths relative to the project, rather than persisting
	// the installing machine's absolute home directory. Shared pins should use
	// the release's HTTPS URL; a local file is useful for candidate qualification.
	if u, _ := url.Parse(p.Artifact); u.Scheme == "" {
		absolute := p.Artifact
		if !filepath.IsAbs(absolute) {
			absolute = filepath.Join(o.CWD, absolute)
		}
		p.Artifact, err = filepath.Rel(o.CWD, absolute)
		if err != nil {
			return err
		}
		for i := range pins {
			if pins[i].ID == p.ID {
				pins[i] = p
			}
		}
	}
	data, err := json.MarshalIndent(projectPins{Plugins: pins}, "", "  ")
	if err != nil {
		return err
	}
	temp, err := writeTemp(o.CWD, append(data, '\n'), 0644)
	if err != nil {
		return err
	}
	defer os.Remove(temp)
	return os.Rename(temp, filepath.Join(o.CWD, PinsFile))
}

// RestorePrebuilt restores all exact declarations without changing configuration
// or rerunning shell preparation. No plugin source is compiled.
func RestorePrebuilt(ctx context.Context, o Options) error {
	if o.Out == nil {
		o.Out = io.Discard
	}
	pins, err := ReadPins(o.CWD)
	if err != nil {
		return err
	}
	for _, p := range pins {
		if err := installPrebuilt(ctx, p, pins, o); err != nil {
			return err
		}
	}
	return nil
}

func installPrebuilt(ctx context.Context, p Prebuilt, pins []Prebuilt, o Options) error {
	slot, err := prebuiltPath(p, o)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(slot), 0755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(slot), ".candidate-*.so")
	if err != nil {
		return err
	}
	candidate := temp.Name()
	defer os.Remove(candidate)
	var reader io.ReadCloser
	u, _ := url.Parse(p.Artifact)
	if u.Scheme == "https" {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.Artifact, nil)
		if err != nil {
			temp.Close()
			return err
		}
		// Keep redirects on HTTPS, including CDN redirects used by release assets.
		client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" {
				return fmt.Errorf("artifact redirect requires HTTPS")
			}
			if len(via) >= 10 {
				return fmt.Errorf("too many artifact redirects")
			}
			return nil
		}}
		response, err := client.Do(request)
		if err != nil {
			temp.Close()
			return err
		}
		reader = response.Body
		if response.StatusCode != http.StatusOK {
			reader.Close()
			temp.Close()
			return fmt.Errorf("download plugin: %s", response.Status)
		}
	} else {
		local := p.Artifact
		if !filepath.IsAbs(local) {
			local = filepath.Join(o.CWD, local)
		}
		reader, err = os.Open(local)
		if err != nil {
			temp.Close()
			return err
		}
	}
	_, copyErr := io.Copy(temp, reader)
	reader.Close()
	closeErr := temp.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	digest, err := pluginstore.Digest(candidate)
	if err != nil {
		return err
	}
	if digest != p.SHA256 {
		return fmt.Errorf("plugin %s checksum mismatch: got %s, expected %s", p.ID, digest, p.SHA256)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, hostDir, err := o.location()
	if err != nil {
		return err
	}
	unlock, err := lock(ctx, filepath.Join(hostDir, ".install.lock"))
	if err != nil {
		return err
	}
	defer unlock()
	peers, _, err := templates.Files(o.ManualDirs, o.Home, o.CWD, hostDir)
	if err != nil {
		return err
	}
	for _, peer := range pins {
		if peer.ID == p.ID {
			continue
		}
		if installed, e := InstalledPath(peer, o); e == nil {
			peers = append(peers, installed)
		}
	}
	peers = append(peers, candidate)
	probe := o.probe
	if probe == nil {
		probe = qualify
	}
	exports, err := probe(ctx, o.Host.Executable, candidate, peers)
	if err != nil {
		return fmt.Errorf("plugin %s cannot load into this executable; use the matching official skgo release (project-built go tool hosts are unsupported): %w", p.ID, err)
	}
	if exports.Version != p.Version {
		return fmt.Errorf("plugin artifact declares version %q, expected %q", exports.Version, p.Version)
	}
	if exports.ID != p.ID {
		return fmt.Errorf("plugin artifact declares ID %q, expected %q", exports.ID, p.ID)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Chmod(candidate, 0755); err != nil {
		return err
	}
	if err := os.Rename(candidate, slot); err != nil {
		return err
	}
	fmt.Fprintf(o.Out, "Installed prebuilt %s@%s for skgo %s (%s).\n", p.ID, p.Version, p.SkgoVersion, slot)
	return nil
}
