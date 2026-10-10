package plugininstall

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tylergannon/skgo/internal/buildinfo"
	"github.com/tylergannon/skgo/internal/pluginstore"
)

func TestPublishFailureKeepsPriorSelection(t *testing.T) {
	dir := t.TempDir()
	slot := filepath.Join(dir, "module.so")
	ref := pluginstore.Reference(slot)
	os.WriteFile(slot, []byte("working prior binary"), 0755)
	os.WriteFile(ref, []byte("prior source"), 0644)
	if err := publish(filepath.Join(dir, "missing.so"), slot, pluginstore.Source{Module: "example.com/new", Version: "v1.0.0"}); err == nil {
		t.Fatal("missing candidate accepted")
	}
	for name, want := range map[string]string{slot: "working prior binary", ref: "prior source"} {
		if b, e := os.ReadFile(name); e != nil || string(b) != want {
			t.Fatalf("%s changed: %q %v", name, b, e)
		}
	}
	candidate := filepath.Join(dir, "candidate.so")
	os.WriteFile(candidate, []byte("new binary"), 0755)
	if err := publish(candidate, slot, pluginstore.Source{Module: "example.com/new", Version: "v1.0.0"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(slot); string(b) != "new binary" {
		t.Fatalf("candidate not published: %q", b)
	}
	if b, _ := os.ReadFile(ref); string(b) != `{"module":"example.com/new","version":"v1.0.0"}` {
		t.Fatalf("source reference: %q", b)
	}
}
func TestHostLockSerializesAndCancellationReleasesWait(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".install.lock")
	unlock, err := lock(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	second, err := lock(ctx, path)
	if err == nil {
		second()
		unlock()
		t.Fatal("two publishers held the same host lock")
	}
	unlock()
	third, err := lock(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	third()
}

func TestInstallQualifiesReplacementAndPreservesWorkingSelectionOnFailure(t *testing.T) {
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	write := func(path string, data []byte) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	read := func(path string) []byte {
		t.Helper()
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	// Build through a real consuming module so the host records its replacement;
	// Install deliberately rejects an unidentifiable development-checkout host.
	hostModule := filepath.Join(root, "host-module")
	write(filepath.Join(hostModule, "go.mod"), []byte("module example.test/install-host\n\ngo 1.27.1\n\nrequire github.com/tylergannon/skgo v0.0.0\nreplace github.com/tylergannon/skgo => "+repo+"\n"))
	host := filepath.Join(root, "skgo")
	cmd := exec.Command("go", "build", "-mod=mod", "-o", host, "github.com/tylergannon/skgo/cmd/skgo")
	cmd.Dir = hostModule
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build actual installer host: %v\n%s", err, b)
	}
	b, err := exec.Command(host, "buildinfo", "--json").Output()
	if err != nil {
		t.Fatal(err)
	}
	var info buildinfo.Info
	if err := json.Unmarshal(b, &info); err != nil {
		t.Fatal(err)
	}
	if info.Build == nil || info.Build.Main.Replace == nil || info.Build.Main.Replace.Path != repo {
		t.Fatalf("host does not record its source: %s", b)
	}
	fixture := read(filepath.Join(repo, "cmd/skgo/testdata/template-plugin/plugin.go"))
	const mainModule = "example.test/install-main"
	const peerModule = "example.test/install-peer"
	sources := map[string]source{}
	makeSource := func(module, version, leaf string, code []byte) string {
		t.Helper()
		dir := filepath.Join(root, "sources", leaf)
		write(filepath.Join(dir, "go.mod"), []byte(fmt.Sprintf("module %s\n\ngo 1.27.1\n\nrequire github.com/tylergannon/skgo v0.0.0\n", module)))
		write(filepath.Join(dir, "main.go"), code)
		ref := module + "@" + version
		sources[ref] = source{resolution: resolution{Path: module, Version: version}, Root: dir, Package: "."}
		return ref
	}
	first := makeSource(mainModule, "v1.0.0", "first", fixture)
	peerCode := bytes.ReplaceAll(fixture, []byte(`"external-fixture"`), []byte(`"other-fixture"`))
	peerCode = bytes.ReplaceAll(peerCode, []byte(`"receipt"`), []byte(`"peer-receipt"`))
	peerCode = bytes.ReplaceAll(peerCode, []byte(`"receipt-app"`), []byte(`"peer-app"`))
	peer := makeSource(peerModule, "v1.0.0", "peer", peerCode)
	replacementCode := bytes.Replace(fixture, []byte(`Version: "1"`), []byte(`Version: "2"`), 1)
	replacement := makeSource(mainModule, "v1.1.0", "replacement", replacementCode)
	conflictCode := bytes.ReplaceAll(replacementCode, []byte(`"receipt"`), []byte(`"peer-receipt"`))
	conflict := makeSource(mainModule, "v1.2.0", "conflict", conflictCode)
	brokenCode := append([]byte("//go:generate go run ./gen\n"), replacementCode...)
	broken := makeSource(mainModule, "v1.3.0", "broken", brokenCode)
	write(filepath.Join(sources[broken].Root, "gen/main.go"), []byte("package main\nimport \"os\"\nfunc main(){os.Stderr.WriteString(\"intentional preparation failure\\n\");os.Exit(1)}\n"))
	// Only remote resolution is replaced; staging, generation, native plugin
	// compilation, actual-host probes, locking and publication remain real.
	var output bytes.Buffer
	cleaned := 0
	o := Options{Host: info, Home: filepath.Join(root, "home"), CWD: root, Out: &output,
		fetchSource: func(_ context.Context, ref, version string, _ io.Writer) (source, func(), error) {
			if version != info.Build.GoVersion {
				t.Fatalf("resolver lost host toolchain: %q", version)
			}
			s, ok := sources[ref]
			if !ok {
				t.Fatalf("unexpected source request %q", ref)
			}
			return s, func() { cleaned++ }, nil
		}}
	install := func(ref string) error {
		t.Helper()
		output.Reset()
		return Install(context.Background(), ref, o)
	}
	for _, ref := range []string{first, peer, replacement} {
		if err := install(ref); err != nil {
			t.Fatalf("install %s: %v\n%s", ref, err, &output)
		}
		if !strings.Contains(output.String(), "Installed "+ref+" for this host") {
			t.Fatalf("missing installed source receipt: %s", &output)
		}
	}
	digest, err := pluginstore.Digest(host)
	if err != nil {
		t.Fatal(err)
	}
	slot := pluginstore.Slot(o.Home, digest, mainModule)
	peerSlot := pluginstore.Slot(o.Home, digest, peerModule)
	retained := map[string][]byte{}
	for _, path := range []string{slot, pluginstore.Reference(slot), peerSlot, pluginstore.Reference(peerSlot), host} {
		retained[path] = read(path)
	}
	if string(retained[pluginstore.Reference(slot)]) != `{"module":"example.test/install-main","version":"v1.1.0"}` {
		t.Fatalf("replacement reference: %s", retained[pluginstore.Reference(slot)])
	}
	for _, tc := range []struct{ ref, diagnostic, log string }{
		{conflict, `add-on "peer-receipt" conflicts`, ""},
		{broken, "prepare plugin", "intentional preparation failure"},
	} {
		t.Run(tc.ref, func(t *testing.T) {
			err := install(tc.ref)
			if err == nil || !strings.Contains(err.Error(), tc.diagnostic) {
				t.Fatalf("bad candidate accepted or wrong failure: %v\n%s", err, &output)
			}
			if tc.ref == conflict && !strings.Contains(err.Error(), "skgo plugin remove "+peerModule) {
				t.Fatalf("conflict lacks source removal command: %v", err)
			}
			if !strings.Contains(output.String(), tc.log) {
				t.Fatalf("missing preparation diagnostics: %s", &output)
			}
			for path, want := range retained {
				if got := read(path); !bytes.Equal(got, want) {
					t.Fatalf("failed install changed %s", path)
				}
			}
		})
	}
	if cleaned != 5 {
		t.Fatalf("source cleanup calls=%d, want 5", cleaned)
	}
	// Byte preservation alone is insufficient: use the prior plugin through
	// ordinary managed discovery, with no private probe or manual directory.
	app := filepath.Join(root, "consumer")
	write(filepath.Join(app, "go.mod"), []byte("module example.test/preserved\n\ngo 1.27.1\n"))
	if err := os.Mkdir(filepath.Join(app, "web"), 0755); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command(host, "add", "--root", app, "--name", "StillWorks", "--set", "text=retained installation", "receipt")
	cmd.Env = append(os.Environ(), "HOME="+o.Home, "SKGO_PLUGIN_DIRS=", pluginstore.ProbeFiles+"=")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("retained plugin no longer usable: %v\n%s", err, b)
	}
	if b := read(filepath.Join(app, "receipt.txt")); string(b) != "StillWorks\nexample.test/preserved\nretained installation\n" {
		t.Fatalf("retained plugin result: %q", b)
	}
	if b := read(filepath.Join(sources[broken].Root, "main.go")); !bytes.Equal(b, brokenCode) {
		t.Fatal("preparation edited source checkout")
	}
}
