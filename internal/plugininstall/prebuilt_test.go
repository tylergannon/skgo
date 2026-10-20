package plugininstall

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/buildinfo"
)

func TestPrebuiltInstallRestoreAndFailureProtection(t *testing.T) {
	root := t.TempDir()
	const bytes = "literal independently qualified plugin bytes"
	filename := "receipt-v1.2.3-skgo-v4.5.6-darwin-arm64.so"
	artifact := filepath.Join(root, filename)
	if err := os.WriteFile(artifact, []byte(bytes), 0644); err != nil {
		t.Fatal(err)
	}
	host := filepath.Join(root, "skgo")
	if err := os.WriteFile(host, []byte("literal official executable"), 0755); err != nil {
		t.Fatal(err)
	}
	p, err := ParseArtifact(artifact)
	if err != nil {
		t.Fatal(err)
	}
	p.SHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte(bytes)))
	p.Options = map[string]string{"output": "clients"}
	p.Selections = []string{"receipt"}
	project := filepath.Join(root, "project")
	if err := os.Mkdir(project, 0755); err != nil {
		t.Fatal(err)
	}
	calls := 0
	o := Options{Host: buildinfo.Info{Executable: host, SkgoVersion: "v4.5.6", Build: &debug.BuildInfo{Main: debug.Module{Path: buildinfo.Module, Version: "v4.5.6"}, Settings: []debug.BuildSetting{{Key: "-trimpath", Value: "true"}, {Key: "CGO_ENABLED", Value: "1"}, {Key: "GOOS", Value: "darwin"}, {Key: "GOARCH", Value: "arm64"}}}}, Home: filepath.Join(root, "home"), CWD: project,
		probe: func(ctx context.Context, actualHost, candidate string, peers []string) (Exports, error) {
			calls++
			b, err := os.ReadFile(candidate)
			if err != nil || string(b) != bytes || actualHost != host || len(peers) != 1 || peers[0] != candidate {
				t.Fatalf("probe did not receive exact candidate: %q %v %v", b, err, peers)
			}
			return Exports{ID: "receipt", Version: "v1.2.3"}, nil
		}}
	if err := InstallPrebuilt(t.Context(), p, o); err != nil {
		t.Fatal(err)
	}
	installed, err := InstalledPath(p, o)
	if err != nil {
		t.Fatal(err)
	}
	pins, err := ReadPins(project)
	if err != nil || len(pins) != 1 || pins[0].Options["output"] != "clients" || pins[0].Selections[0] != "receipt" {
		t.Fatalf("pins: %+v %v", pins, err)
	}
	if filepath.IsAbs(pins[0].Artifact) {
		t.Fatal("project pin retained machine-absolute artifact path")
	}
	original, err := os.ReadFile(filepath.Join(project, PinsFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*Prebuilt, *Options)
	}{
		{"wrong checksum", func(p *Prebuilt, o *Options) { p.SHA256 = strings.Repeat("0", 64) }},
		{"wrong skgo version", func(p *Prebuilt, o *Options) { o.Host.SkgoVersion = "v4.5.7" }},
		{"project built host", func(p *Prebuilt, o *Options) {
			copy := *o.Host.Build
			copy.Main.Path = "example.com/app"
			o.Host.Build = &copy
		}},
		{"nonofficial build flags", func(p *Prebuilt, o *Options) {
			copy := *o.Host.Build
			copy.Settings = append([]debug.BuildSetting(nil), copy.Settings...)
			copy.Settings[0].Value = "false"
			o.Host.Build = &copy
		}},
		{"wrong host architecture", func(p *Prebuilt, o *Options) { p.Artifact = strings.Replace(p.Artifact, "arm64", "amd64", 1) }},
		{"wrong declared identity", func(p *Prebuilt, o *Options) {
			o.probe = func(context.Context, string, string, []string) (Exports, error) {
				return Exports{ID: "other", Version: p.Version}, nil
			}
		}},
		{"wrong declared version", func(p *Prebuilt, o *Options) {
			o.probe = func(context.Context, string, string, []string) (Exports, error) {
				return Exports{ID: p.ID, Version: "v9.0.0"}, nil
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate, options := p, o
			tc.change(&candidate, &options)
			if err := InstallPrebuilt(t.Context(), candidate, options); err == nil {
				t.Fatal("invalid candidate accepted")
			}
			b, _ := os.ReadFile(filepath.Join(project, PinsFile))
			if string(b) != string(original) {
				t.Fatal("failed install changed project pin")
			}
			b, _ = os.ReadFile(installed)
			if string(b) != bytes {
				t.Fatal("failed install replaced prior plugin")
			}
		})
	}
	if err := os.Remove(installed); err != nil {
		t.Fatal(err)
	}
	if _, err := InstalledPath(p, o); err == nil {
		t.Fatal("missing cache accepted")
	}
	if err := RestorePrebuilt(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("probe calls = %d; restore must probe but never execute setup", calls)
	}
	b, _ := os.ReadFile(filepath.Join(project, PinsFile))
	if string(b) != string(original) {
		t.Fatal("restore changed configuration")
	}
	if err := os.WriteFile(installed, []byte("damaged"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := InstalledPath(p, o); err == nil {
		t.Fatal("damaged cached artifact accepted")
	}
}

func TestArtifactIdentityAndDuplicatePins(t *testing.T) {
	p, err := ParseArtifact("https://example.com/native-shell-v0.1.29-skgo-v0.41.0-darwin-arm64.so?download=1")
	if err != nil || p.ID != "native-shell" || p.Version != "v0.1.29" || p.SkgoVersion != "v0.41.0" {
		t.Fatalf("identity %+v %v", p, err)
	}
	for _, bad := range []string{"https://example.com/native-shell.so", "http://example.com/native-shell-v0.1.29-skgo-v0.41.0-darwin-arm64.so", "https://user@example.com/native-shell-v0.1.29-skgo-v0.41.0-darwin-arm64.so"} {
		if _, err := ParseArtifact(bad); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	root := t.TempDir()
	const pin = `{"id":"receipt","version":"v1.2.3","skgoVersion":"v4.5.6","artifact":"receipt-v1.2.3-skgo-v4.5.6-darwin-arm64.so","sha256":"0000000000000000000000000000000000000000000000000000000000000000"}`
	if err := os.WriteFile(filepath.Join(root, PinsFile), []byte(`{"plugins":[`+pin+`,`+pin+`]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPins(root); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate pins: %v", err)
	}
}
