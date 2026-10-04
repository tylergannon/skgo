package adapter

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDeclaredPrerenderInputsBuildAndProduceNativeArtifacts(t *testing.T) {
	root, err := filepath.Abs("../../example")
	if err != nil {
		t.Fatal(err)
	}
	dependencies := filepath.Join(root, "web", "node_modules")
	if _, err := os.Stat(filepath.Join(dependencies, ".bin", "vp")); err != nil {
		t.Fatalf("pinned frontend dependencies are missing: %v", err)
	}
	fixture := filepath.Join(t.TempDir(), "example")
	copyFixtureTree(t, root, fixture)
	linkFixtureDependencies(t, dependencies, filepath.Join(fixture, "web", "node_modules"), filepath.Join(root, "..", "internal", "adapter"))
	goMod := filepath.Join(fixture, "go.mod")
	mod, err := os.ReadFile(goMod)
	if err != nil {
		t.Fatal(err)
	}
	updatedMod := strings.Replace(string(mod), "replace github.com/tylergannon/skgo => ../", "replace github.com/tylergannon/skgo => "+filepath.Clean(filepath.Join(root, "..")), 1)
	if updatedMod == string(mod) {
		t.Fatal("could not point the isolated example module at this worktree")
	}
	if err := os.WriteFile(goMod, []byte(updatedMod), 0o644); err != nil {
		t.Fatal(err)
	}

	remote := filepath.Join(fixture, "web", "src", "routes", "declared.remote.go")
	declaredSource := `package site

import (
	"context"
	"os"
	"os/exec"
	"runtime"

	"github.com/tylergannon/polytype/devalue"
	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/businesslogic"
)

func getDeclaredSite(ctx context.Context, name string) (string, error) {
	if receipt := os.Getenv("SKGO_REMOTE_RECEIPT"); receipt != "" {
		file, err := os.OpenFile(receipt, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil { return "", err }
		if _, err := file.WriteString(name + "\n"); err != nil { file.Close(); return "", err }
		if err := file.Close(); err != nil { return "", err }
	}
	return "build:" + name, nil
}

func siteInputs() ([]string, error) {
	if receipt := os.Getenv("SKGO_INPUTS_RECEIPT"); receipt != "" {
		if err := os.WriteFile(receipt, []byte("called"), 0o600); err != nil { return nil, err }
	}
	if late := os.Getenv("SKGO_LATE_RECEIPT"); late != "" && runtime.GOOS != "windows" {
		cmd := exec.Command("/bin/sh", "-c", "sleep 1; printf late > \"$1\"", "sh", late)
		if err := cmd.Start(); err != nil { return nil, err }
	}
	return []string{"atlas", "beacon", "atlas"}, nil
}

func moneyInputs() ([]businesslogic.Money, error) {
	return []businesslogic.Money{{Cents: 125}}, nil
}

func emptyInputs() ([]devalue.UndefinedValue, error) {
	if receipt := os.Getenv("SKGO_EMPTY_INPUTS_RECEIPT"); receipt != "" {
		if err := os.WriteFile(receipt, []byte("called"), 0o600); err != nil { return nil, err }
	}
	return []devalue.UndefinedValue{}, nil
}

func moneyRemote(ctx context.Context, value businesslogic.Money) (string, error) { return value.Format(), nil }
func noArgumentRemote(ctx context.Context) (string, error) { return "noarg", nil }

var (
	_ = skgo.Prerender(getDeclaredSite, skgo.PrerenderOptions{Inputs: siteInputs})
	_ = skgo.Prerender(moneyRemote, skgo.PrerenderOptions{Inputs: moneyInputs})
	_ = skgo.Prerender(noArgumentRemote, skgo.PrerenderOptions{Inputs: emptyInputs})
)
`
	if err := os.WriteFile(remote, []byte(declaredSource), 0o644); err != nil {
		t.Fatal(err)
	}
	serverHook := filepath.Join(fixture, "web", "src", "hooks.server.ts")
	if err := os.WriteFile(serverHook, []byte(`import { getDeclaredSite, moneyRemote, noArgumentRemote } from "./routes/declared.remote";
import type { Handle } from "@sveltejs/kit";
void [getDeclaredSite, moneyRemote, noArgumentRemote];
export const handle: Handle = async ({ event, resolve }) => resolve(event);
`), 0o644); err != nil {
		t.Fatal(err)
	}
	generate := exec.Command("go", "generate", "./...")
	generate.Dir = fixture
	generate.Env = append(os.Environ(), "GOWORK=off")
	if output, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("go generate: %v\n%s", err, output)
	}
	for _, command := range [][]string{
		{filepath.Join(fixture, "web", "node_modules", ".bin", "svelte-kit"), "sync"},
		{filepath.Join(fixture, "web", "node_modules", ".bin", "svelte-check"), "--tsconfig", "./tsconfig.json"},
	} {
		check := exec.Command(command[0], command[1:]...)
		check.Dir = filepath.Join(fixture, "web")
		check.Env = append(os.Environ(), "GOWORK=off")
		if output, err := check.CombinedOutput(); err != nil {
			t.Fatalf("frontend typecheck %v: %v\n%s", command, err, output)
		}
	}
	build := exec.Command(filepath.Join(fixture, "web", "node_modules", ".bin", "vp"), "build")
	build.Dir = filepath.Join(fixture, "web")
	inputReceipt := filepath.Join(fixture, "inputs-called")
	emptyReceipt := filepath.Join(fixture, "empty-inputs-called")
	remoteReceipt := filepath.Join(fixture, "remotes-called")
	lateReceipt := filepath.Join(fixture, "late-descendant-write")
	build.Env = append(os.Environ(), "GOWORK=off", "ORIGIN=http://127.0.0.1:8080", "SKGO_INPUTS_RECEIPT="+inputReceipt, "SKGO_EMPTY_INPUTS_RECEIPT="+emptyReceipt, "SKGO_REMOTE_RECEIPT="+remoteReceipt, "SKGO_LATE_RECEIPT="+lateReceipt)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("vp build: %v\n%s", err, output)
	}
	if receipt, err := os.ReadFile(inputReceipt); err != nil || string(receipt) != "called" {
		t.Fatalf("declared input producer receipt = %q, %v", receipt, err)
	}
	if receipt, err := os.ReadFile(emptyReceipt); err != nil || string(receipt) != "called" {
		t.Fatalf("empty no-argument input producer receipt = %q, %v", receipt, err)
	}
	if calls, err := os.ReadFile(remoteReceipt); err != nil || string(calls) != "atlas\nbeacon\n" {
		t.Fatalf("unique Go body calls = %q, %v; want one call for atlas and beacon despite duplicate declared atlas input", calls, err)
	}
	if runtime.GOOS != "windows" {
		time.Sleep(1200 * time.Millisecond)
		if _, err := os.Stat(lateReceipt); !os.IsNotExist(err) {
			t.Fatalf("owned Go producer descendant performed late I/O after build completion: %v", err)
		}
	}
	for _, path := range []string{
		"web/build/prerendered/prerender/atlas.html",
		"web/build/prerendered/prerender/beacon.html",
	} {
		if _, err := os.Stat(filepath.Join(fixture, filepath.FromSlash(path))); err != nil {
			t.Errorf("native prerender artifact %s is missing: %v", path, err)
		}
	}
	manifest, err := os.ReadFile(filepath.Join(fixture, "web", "skgo.remotes.json"))
	if err != nil {
		t.Fatal(err)
	}
	var generated struct {
		Remotes []string `json:"remotes"`
	}
	if err := json.Unmarshal(manifest, &generated); err != nil {
		t.Fatal(err)
	}
	declaredModule := ""
	for _, remote := range generated.Remotes {
		if strings.HasSuffix(remote, "/getDeclaredSite") {
			declaredModule = remote
		}
	}
	if declaredModule == "" {
		t.Fatalf("generated manifest omitted declared-only remote: %s", manifest)
	}
	moduleParts := strings.SplitN(declaredModule, "/", 2)
	remoteArtifacts := filepath.Join(fixture, "web", "build", "prerendered", "_app", "remote", moduleParts[0], moduleParts[1])
	wantRemoteArtifacts := map[string]string{"WyJhdGxhcyJd": "build:atlas", "WyJiZWFjb24iXQ": "build:beacon"}
	for payload, literal := range wantRemoteArtifacts {
		path := filepath.Join(remoteArtifacts, payload)
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("native declared input artifact %s is missing: %v", path, err)
		}
		if !strings.Contains(string(contents), literal) {
			t.Errorf("native result for %s omitted body literal %q: %s", payload, literal, contents)
		}
	}
	entries, err := os.ReadDir(remoteArtifacts)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(wantRemoteArtifacts) {
		t.Fatalf("declared-only remote artifact count = %d, want exactly %d: %v", len(entries), len(wantRemoteArtifacts), entries)
	}
	var moneyModule string
	for _, remote := range generated.Remotes {
		if strings.HasSuffix(remote, "/moneyRemote") {
			moneyModule = remote
		}
	}
	if moneyModule == "" {
		t.Fatal("generated manifest omitted transported Money remote")
	}
	moneyParts := strings.SplitN(moneyModule, "/", 2)
	moneyArtifact := filepath.Join(fixture, "web", "build", "prerendered", "_app", "remote", moneyParts[0], moneyParts[1], "W1siTW9uZXkiLDFdLFsiX19za3JhbyIsMl0seyJjZW50cyI6M30sMTI1XQ")
	moneyData, err := os.ReadFile(moneyArtifact)
	if err != nil || !strings.Contains(string(moneyData), "$1.25") {
		t.Fatalf("native canonical transported Money artifact = %q, %v", moneyData, err)
	}
	var emptyModule string
	for _, remote := range generated.Remotes {
		if strings.HasSuffix(remote, "/noArgumentRemote") {
			emptyModule = remote
		}
	}
	if emptyModule == "" {
		t.Fatal("generated manifest omitted empty no-argument remote")
	}
	emptyParts := strings.SplitN(emptyModule, "/", 2)
	emptyArtifact := filepath.Join(fixture, "web", "build", "prerendered", "_app", "remote", emptyParts[0], emptyParts[1])
	emptyData, err := os.ReadFile(emptyArtifact)
	if err != nil || !strings.Contains(string(emptyData), "noarg") {
		t.Fatalf("native no-argument artifact after empty Inputs = %q, %v", emptyData, err)
	}
	buildManifest, err := os.ReadFile(filepath.Join(fixture, "web", "build", "skgo.manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(buildManifest), `"/prerender/atlas"`) || !strings.Contains(string(buildManifest), `"/prerender/beacon"`) {
		t.Fatalf("adapter manifest omitted declared input pages:\n%s", buildManifest)
	}
}

func copyFixtureTree(t *testing.T, source, destination string) {
	t.Helper()
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "web/node_modules" || relative == "web/.svelte-kit" || relative == "web/build" || relative == "web/dist" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
		}
		target := filepath.Join(destination, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, contents, info.Mode().Perm())
	})
	if err != nil {
		t.Fatalf("copy fixture tree: %v", err)
	}
}

func linkFixtureDependencies(t *testing.T, source, destination, adapterPath string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(destination, "@skgo"), 0o755); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() == "$app" || entry.Name() == ".vite-temp" || entry.Name() == "@skgo" {
			continue
		}
		if err := os.Symlink(filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name())); err != nil {
			t.Fatalf("link frontend dependency %s: %v", entry.Name(), err)
		}
	}
	if err := os.Symlink(adapterPath, filepath.Join(destination, "@skgo", "sveltekit-adapter")); err != nil {
		t.Fatal(err)
	}
}
