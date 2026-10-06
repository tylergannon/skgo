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
	"errors"
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
	if os.Getenv("SKGO_FAIL_INPUTS") != "" { return nil, errors.New("declared input producer failed") }
	if receipt := os.Getenv("SKGO_INPUTS_RECEIPT"); receipt != "" {
	file, err := os.OpenFile(receipt, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil { return nil, err }
		if _, err := file.WriteString("called\n"); err != nil { file.Close(); return nil, err }
		if err := file.Close(); err != nil { return nil, err }
	}
	if late := os.Getenv("SKGO_LATE_RECEIPT"); late != "" && runtime.GOOS != "windows" {
		cmd := exec.Command("/bin/sh", "-c", "sleep 1; printf late > \"$1\"", "sh", late)
		if err := cmd.Start(); err != nil { return nil, err }
	}
	return []string{"atlas", "beacon", "atlas"}, nil
}

func moneyInputs() ([]businesslogic.Money, error) {
	if receipt := os.Getenv("SKGO_MONEY_INPUTS_RECEIPT"); receipt != "" {
		file, err := os.OpenFile(receipt, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil { return nil, err }
		if _, err := file.WriteString("called\n"); err != nil { file.Close(); return nil, err }
		if err := file.Close(); err != nil { return nil, err }
	}
	return []businesslogic.Money{{Cents: 125}}, nil
}

func emptyInputs() ([]devalue.UndefinedValue, error) {
	if receipt := os.Getenv("SKGO_EMPTY_INPUTS_RECEIPT"); receipt != "" {
	file, err := os.OpenFile(receipt, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil { return nil, err }
		if _, err := file.WriteString("called\n"); err != nil { file.Close(); return nil, err }
		if err := file.Close(); err != nil { return nil, err }
	}
	return []devalue.UndefinedValue{}, nil
}

func moneyRemote(ctx context.Context, value businesslogic.Money) (string, error) { return value.Format(), nil }
func noArgumentRemote(ctx context.Context) (string, error) { return "noarg", nil }
func appFailureRemote(ctx context.Context) (string, error) { return "", skgo.Errorf(418, "private app failure") }
func unknownFailureRemote(ctx context.Context) (string, error) { return "", errors.New("private unknown failure") }
func redirectRemote(ctx context.Context) (string, error) { return "", &skgo.Redirect{Status: 307, Location: "/prerender/atlas"} }

func errorInputs() ([]devalue.UndefinedValue, error) {
	if receipt := os.Getenv("SKGO_ERROR_INPUTS_RECEIPT"); receipt != "" {
		file, err := os.OpenFile(receipt, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil { return nil, err }
		if _, err := file.WriteString("called\n"); err != nil { file.Close(); return nil, err }
		if err := file.Close(); err != nil { return nil, err }
	}
	return []devalue.UndefinedValue{{}}, nil
}

var (
	_ = skgo.Prerender(getDeclaredSite, skgo.PrerenderOptions{Inputs: siteInputs})
	_ = skgo.Prerender(moneyRemote, skgo.PrerenderOptions{Inputs: moneyInputs})
	_ = skgo.Prerender(noArgumentRemote, skgo.PrerenderOptions{Inputs: emptyInputs})
	_ = skgo.Prerender(appFailureRemote, skgo.PrerenderOptions{Inputs: errorInputs})
	_ = skgo.Prerender(unknownFailureRemote, skgo.PrerenderOptions{Inputs: errorInputs})
	_ = skgo.Prerender(redirectRemote, skgo.PrerenderOptions{Inputs: errorInputs})
)
`
	if err := os.WriteFile(remote, []byte(declaredSource), 0o644); err != nil {
		t.Fatal(err)
	}
	helperRoute := filepath.Join(fixture, "web", "src", "routes", "prerender-helper")
	if err := os.MkdirAll(helperRoute, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(helperRoute, "+page.ts"), []byte(`import { remoteInputs } from "@skgo/sveltekit-adapter/prerender";

export async function load() {
  try {
    await remoteInputs("goja-control", "remoteInputs");
    return { message: "inert helper unexpectedly returned" };
  } catch (error) {
    return { message: error instanceof Error ? error.message : String(error) };
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(helperRoute, "+page.svelte"), []byte(`<script>let { data } = $props();</script><p>{data.message}</p>`), 0o644); err != nil {
		t.Fatal(err)
	}
	pageRemote := filepath.Join(fixture, "web", "src", "routes", "about", "about.remote.go")
	pageSource, err := os.ReadFile(pageRemote)
	if err != nil {
		t.Fatal(err)
	}
	pageSourceText := strings.Replace(string(pageSource), `func buildReceipt(_ context.Context, name string) (string, error) {
	return "Go prerender remote: " + name, nil
}`, `func buildReceipt(ctx context.Context, name string) (string, error) {
	if receipt := os.Getenv("SKGO_PAGE_REMOTE_RECEIPT"); receipt != "" {
		file, err := os.OpenFile(receipt, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil { return "", err }
		if _, err := file.WriteString(name + "\n"); err != nil { file.Close(); return "", err }
		if err := file.Close(); err != nil { return "", err }
	}
	return "Go prerender remote: " + name, nil
}`, 1)
	if pageSourceText == string(pageSource) {
		t.Fatal("could not instrument duplicate page remote calls")
	}
	pageSourceText = strings.Replace(pageSourceText, `import (
	"context"`, `import (
	"context"
	"os"`, 1)
	pageSourceText = strings.Replace(pageSourceText, `var _ = skgo.Prerender(buildReceipt)`, `func buildReceiptInputs() ([]string, error) {
	if receipt := os.Getenv("SKGO_PAGE_INPUTS_RECEIPT"); receipt != "" {
		file, err := os.OpenFile(receipt, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil { return nil, err }
		if _, err := file.WriteString("called\n"); err != nil { file.Close(); return nil, err }
		if err := file.Close(); err != nil { return nil, err }
	}
	return []string{"atlas"}, nil
}

var _ = skgo.Prerender(buildReceipt, skgo.PrerenderOptions{Inputs: buildReceiptInputs})`, 1)
	if !strings.Contains(pageSourceText, "PrerenderOptions{Inputs: buildReceiptInputs}") {
		t.Fatal("could not add a declared input for the duplicate page remote")
	}
	if err := os.WriteFile(pageRemote, []byte(pageSourceText), 0o644); err != nil {
		t.Fatal(err)
	}
	serverHook := filepath.Join(fixture, "web", "src", "hooks.server.ts")
	if err := os.WriteFile(serverHook, []byte(`import { appendFileSync } from "node:fs";
import { getDeclaredSite, moneyRemote, noArgumentRemote, appFailureRemote, unknownFailureRemote, redirectRemote } from "./routes/declared.remote";
import type { Handle } from "@sveltejs/kit/hooks";
void [getDeclaredSite, moneyRemote, noArgumentRemote, appFailureRemote, unknownFailureRemote, redirectRemote];
export const handle: Handle = async ({ event, resolve }) => resolve(event);
export const handleError = ({ error, kind }: { error: unknown; kind: string }) => {
  const diagnostic = error instanceof Error ? error.message : JSON.stringify(error);
	  appendFileSync(process.env.SKGO_NATIVE_ERROR_RECEIPT!, kind + "|" + diagnostic + "\n");
  return { message: kind === "app" ? "app-policy" : "Internal Error" };
};
`), 0o644); err != nil {
		t.Fatal(err)
	}
	viteConfig := filepath.Join(fixture, "web", "vite.config.ts")
	viteSource, err := os.ReadFile(viteConfig)
	if err != nil {
		t.Fatal(err)
	}
	viteSourceText := strings.Replace(string(viteSource), "adapter: skgo(),", "adapter: skgo(),\n      prerender: { handleHttpError: \"ignore\" },", 1)
	if viteSourceText == string(viteSource) {
		t.Fatal("could not set permissive native HTTP error policy")
	}
	if err := os.WriteFile(viteConfig, []byte(viteSourceText), 0o644); err != nil {
		t.Fatal(err)
	}
	inputReceipt := filepath.Join(fixture, "inputs-called")
	emptyReceipt := filepath.Join(fixture, "empty-inputs-called")
	moneyReceipt := filepath.Join(fixture, "money-inputs-called")
	pageInputsReceipt := filepath.Join(fixture, "page-inputs-called")
	errorInputsReceipt := filepath.Join(fixture, "error-inputs-called")
	generate := exec.Command("go", "generate", "./...")
	generate.Dir = fixture
	generate.Env = append(os.Environ(), "GOWORK=off", "SKGO_INPUTS_RECEIPT="+inputReceipt, "SKGO_EMPTY_INPUTS_RECEIPT="+emptyReceipt, "SKGO_MONEY_INPUTS_RECEIPT="+moneyReceipt, "SKGO_PAGE_INPUTS_RECEIPT="+pageInputsReceipt, "SKGO_ERROR_INPUTS_RECEIPT="+errorInputsReceipt)
	if output, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("go generate: %v\n%s", err, output)
	}
	for _, receipt := range []string{inputReceipt, emptyReceipt, moneyReceipt, pageInputsReceipt, errorInputsReceipt} {
		if _, err := os.Stat(receipt); !os.IsNotExist(err) {
			t.Fatalf("producer ran during generation: %s (%v)", receipt, err)
		}
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
	remoteReceipt := filepath.Join(fixture, "remotes-called")
	pageRemoteReceipt := filepath.Join(fixture, "page-remotes-called")
	nativeErrorReceipt := filepath.Join(fixture, "native-errors")
	lateReceipt := filepath.Join(fixture, "late-descendant-write")
	build.Env = append(os.Environ(), "GOWORK=off", "ORIGIN=http://127.0.0.1:8080", "SKGO_INPUTS_RECEIPT="+inputReceipt, "SKGO_EMPTY_INPUTS_RECEIPT="+emptyReceipt, "SKGO_MONEY_INPUTS_RECEIPT="+moneyReceipt, "SKGO_PAGE_INPUTS_RECEIPT="+pageInputsReceipt, "SKGO_ERROR_INPUTS_RECEIPT="+errorInputsReceipt, "SKGO_REMOTE_RECEIPT="+remoteReceipt, "SKGO_PAGE_REMOTE_RECEIPT="+pageRemoteReceipt, "SKGO_LATE_RECEIPT="+lateReceipt, "SKGO_NATIVE_ERROR_RECEIPT="+nativeErrorReceipt)
	buildOutput, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("vp build: %v\n%s", err, buildOutput)
	}
	if strings.Contains(string(buildOutput), "private unknown failure") {
		t.Fatalf("native public prerender output leaked the private unknown diagnostic:\n%s", buildOutput)
	}
	if receipt, err := os.ReadFile(inputReceipt); err != nil || string(receipt) != "called\n" {
		t.Fatalf("declared input producer receipt = %q, %v", receipt, err)
	}
	if receipt, err := os.ReadFile(emptyReceipt); err != nil || string(receipt) != "called\n" {
		t.Fatalf("empty no-argument input producer receipt = %q, %v", receipt, err)
	}
	if receipt, err := os.ReadFile(moneyReceipt); err != nil || string(receipt) != "called\n" {
		t.Fatalf("transported Money input producer receipt = %q, %v", receipt, err)
	}
	if receipt, err := os.ReadFile(pageInputsReceipt); err != nil || string(receipt) != "called\n" {
		t.Fatalf("duplicate page remote producer receipt = %q, %v", receipt, err)
	}
	if receipt, err := os.ReadFile(errorInputsReceipt); err != nil || string(receipt) != "called\ncalled\ncalled\n" {
		t.Fatalf("native error/redirect input producers = %q, %v", receipt, err)
	}
	errorHooks, err := os.ReadFile(nativeErrorReceipt)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(errorHooks), "app|") || !strings.Contains(string(errorHooks), "private app failure") || !strings.Contains(string(errorHooks), "unknown|private unknown failure") {
		t.Fatalf("native handleError did not receive app and unknown Go errors: %s", errorHooks)
	}
	if calls, err := os.ReadFile(remoteReceipt); err != nil || string(calls) != "atlas\nbeacon\n" {
		t.Fatalf("unique Go body calls = %q, %v; want one call for atlas and beacon despite duplicate declared atlas input", calls, err)
	}
	if calls, err := os.ReadFile(pageRemoteReceipt); err != nil || string(calls) != "atlas\n" {
		t.Fatalf("duplicate page remote calls = %q, %v; want one build response for two atlas calls", calls, err)
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
	for remoteName, checks := range map[string][]string{
		"redirectRemote": {"redirect", "/prerender/atlas"},
	} {
		var remoteID string
		for _, remote := range generated.Remotes {
			if strings.HasSuffix(remote, "/"+remoteName) {
				remoteID = remote
			}
		}
		if remoteID == "" {
			t.Fatalf("generated manifest omitted %s", remoteName)
		}
		parts := strings.SplitN(remoteID, "/", 2)
		artifact := filepath.Join(fixture, "web", "build", "prerendered", "_app", "remote", parts[0], parts[1])
		contents, err := os.ReadFile(artifact)
		if err != nil {
			t.Fatalf("native %s policy artifact: %v", remoteName, err)
		}
		for _, want := range checks {
			if !strings.Contains(string(contents), want) {
				t.Errorf("native %s artifact omitted %q: %s", remoteName, want, contents)
			}
		}
		if remoteName == "unknownFailureRemote" && strings.Contains(string(contents), "private unknown failure") {
			t.Errorf("native public unknown error exposed private Go diagnostic: %s", contents)
		}
	}
	failingBuild := exec.Command(filepath.Join(fixture, "web", "node_modules", ".bin", "vp"), "build")
	failingBuild.Dir = filepath.Join(fixture, "web")
	failingBuild.Env = append(os.Environ(), "GOWORK=off", "ORIGIN=http://127.0.0.1:8080", "SKGO_FAIL_INPUTS=1", "SKGO_NATIVE_ERROR_RECEIPT="+nativeErrorReceipt)
	failingOutput, failingErr := failingBuild.CombinedOutput()
	if failingErr == nil || !strings.Contains(string(failingOutput), "declared input producer failed") {
		t.Fatalf("permissive native HTTP policy accepted a Go Inputs producer failure: err=%v\n%s", failingErr, failingOutput)
	}
	badShapeOverlay := filepath.Join(t.TempDir(), "prerender_remote_bad_shape.go")
	remoteSourcePath := filepath.Join(root, "..", "prerender_remote.go")
	remoteSource, err := os.ReadFile(remoteSourcePath)
	if err != nil {
		t.Fatal(err)
	}
	badShapeSource := strings.Replace(string(remoteSource), `"net/url"`, `"net/url"
	"os"`, 1)
	badShapeSource = strings.Replace(badShapeSource, `	encoded, err := devalue.StringifyWith(values, transport.reducers())`, `	if os.Getenv("SKGO_FORCE_BAD_INPUT_SHAPE") != "" {
		return json.NewEncoder(out).Encode(struct { Inputs string `+"`json:\"inputs\"`"+` }{Inputs: "[null]"})
	}
	encoded, err := devalue.StringifyWith(values, transport.reducers())`, 1)
	if badShapeSource == string(remoteSource) || badShapeSource == "" {
		t.Fatal("could not create the malformed-input response test overlay")
	}
	if err := os.WriteFile(badShapeOverlay, []byte(badShapeSource), 0o644); err != nil {
		t.Fatal(err)
	}
	overlayData, err := json.Marshal(struct {
		Replace map[string]string `json:"Replace"`
	}{Replace: map[string]string{remoteSourcePath: badShapeOverlay}})
	if err != nil {
		t.Fatal(err)
	}
	overlayPath := filepath.Join(t.TempDir(), "go-overlay.json")
	if err := os.WriteFile(overlayPath, overlayData, 0o600); err != nil {
		t.Fatal(err)
	}
	malformedBuild := exec.Command(filepath.Join(fixture, "web", "node_modules", ".bin", "vp"), "build")
	malformedBuild.Dir = filepath.Join(fixture, "web")
	malformedBuild.Env = append(os.Environ(), "GOWORK=off", "ORIGIN=http://127.0.0.1:8080", "GOFLAGS=-overlay="+overlayPath, "SKGO_FORCE_BAD_INPUT_SHAPE=1", "SKGO_NATIVE_ERROR_RECEIPT="+nativeErrorReceipt)
	malformedOutput, malformedErr := malformedBuild.CombinedOutput()
	if malformedErr == nil || !strings.Contains(string(malformedOutput), "did not decode to an array") {
		t.Fatalf("permissive native HTTP policy accepted malformed Go Inputs shape: err=%v\n%s", malformedErr, malformedOutput)
	}
	jsconfig := filepath.Join(fixture, "web", "jsconfig.json")
	tsconfig := filepath.Join(fixture, "web", "tsconfig.json")
	tsconfigData, err := os.ReadFile(tsconfig)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(tsconfig); err != nil {
		t.Fatalf("remove TypeScript config before JavaScript-mode generation: %v", err)
	}
	if err := os.WriteFile(jsconfig, []byte(`{
  "extends": "$app/tsconfig",
  "compilerOptions": { "allowJs": true, "checkJs": true, "strict": true }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	jsGenerate := exec.Command("go", "generate", "./...")
	jsGenerate.Dir = fixture
	jsGenerate.Env = append(os.Environ(), "GOWORK=off")
	if output, err := jsGenerate.CombinedOutput(); err != nil {
		t.Fatalf("JavaScript-mode go generate: %v\n%s", err, output)
	}
	jsRemote := filepath.Join(fixture, "web", "src", "routes", "declared.remote.js")
	if _, err := os.Stat(jsRemote); err != nil {
		t.Fatalf("JavaScript-mode go generate did not emit the JavaScript remote: %v", err)
	}
	var tsconfigObject map[string]any
	if err := json.Unmarshal(tsconfigData, &tsconfigObject); err != nil {
		t.Fatal(err)
	}
	compilerOptions, _ := tsconfigObject["compilerOptions"].(map[string]any)
	if compilerOptions == nil {
		compilerOptions = map[string]any{}
		tsconfigObject["compilerOptions"] = compilerOptions
	}
	compilerOptions["allowJs"] = true
	compilerOptions["checkJs"] = true
	updatedTSConfig, err := json.MarshalIndent(tsconfigObject, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tsconfig, append(updatedTSConfig, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	sync := exec.Command(filepath.Join(fixture, "web", "node_modules", ".bin", "svelte-kit"), "sync")
	sync.Dir = filepath.Join(fixture, "web")
	if output, err := sync.CombinedOutput(); err != nil {
		t.Fatalf("JavaScript-mode svelte-kit sync: %v\n%s", err, output)
	}
	jsCheck := exec.Command(filepath.Join(fixture, "web", "node_modules", ".bin", "svelte-check"), "--tsconfig", "./tsconfig.json")
	jsCheck.Dir = filepath.Join(fixture, "web")
	if output, err := jsCheck.CombinedOutput(); err != nil {
		t.Fatalf("generated JavaScript frontend typecheck: %v\n%s", err, output)
	}
	jsBuild := exec.Command(filepath.Join(fixture, "web", "node_modules", ".bin", "vp"), "build")
	jsBuild.Dir = filepath.Join(fixture, "web")
	jsBuild.Env = append(os.Environ(), "GOWORK=off", "ORIGIN=http://127.0.0.1:8080")
	jsBuildOutput, err := jsBuild.CombinedOutput()
	if err != nil {
		t.Fatalf("JavaScript-mode vp build: %v\n%s", err, jsBuildOutput)
	}
	jsManifest, err := os.ReadFile(filepath.Join(fixture, "web", "skgo.remotes.json"))
	if err != nil {
		t.Fatal(err)
	}
	var jsGenerated struct {
		Remotes []string `json:"remotes"`
	}
	if err := json.Unmarshal(jsManifest, &jsGenerated); err != nil {
		t.Fatal(err)
	}
	jsDeclaredModule := ""
	for _, remote := range jsGenerated.Remotes {
		if strings.HasSuffix(remote, "/getDeclaredSite") {
			jsDeclaredModule = remote
		}
	}
	if jsDeclaredModule == "" {
		t.Fatalf("JavaScript-mode manifest omitted declared-only remote: %s", jsManifest)
	}
	jsModuleParts := strings.SplitN(jsDeclaredModule, "/", 2)
	jsDeclaredArtifact := filepath.Join(fixture, "web", "build", "prerendered", "_app", "remote", jsModuleParts[0], jsModuleParts[1], "WyJhdGxhcyJd")
	jsDeclaredData, err := os.ReadFile(jsDeclaredArtifact)
	if err != nil || !strings.Contains(string(jsDeclaredData), "build:atlas") {
		t.Fatalf("JavaScript-mode native declared-input artifact = %q, %v", jsDeclaredData, err)
	}
	jsServerBundle, err := os.ReadFile(filepath.Join(fixture, "web", "build", "ssr", "bundle.js"))
	if err != nil {
		t.Fatalf("JavaScript-mode Goja bundle: %v", err)
	}
	for _, forbidden := range []string{"node:child_process", "node:fs", "node:module"} {
		if strings.Contains(string(jsServerBundle), `node_module("`+forbidden+`")`) {
			t.Errorf("JavaScript-mode Goja bundle externalizes Node builtin %q", forbidden)
		}
	}
	gojaTest := filepath.Join(fixture, "cmd", "prerender_helper_goja_test.go")
	if err := os.WriteFile(gojaTest, []byte(`package main_test

import (
  "io/fs"
  "net/http/httptest"
  "strings"
  "testing"

  "github.com/tylergannon/skgo/example"
  "github.com/tylergannon/skgo/example/web"
)

func TestPrerenderHelperIsBuildOnlyInGoja(t *testing.T) {
  dist, err := fs.Sub(web.Build, "build")
  if err != nil { t.Fatal(err) }
  handler, _, err := example.NewHandler(dist, "", "http://127.0.0.1:8080")
  if err != nil { t.Fatal(err) }
  response := httptest.NewRecorder()
  handler.ServeHTTP(response, httptest.NewRequest("GET", "/prerender-helper", nil))
  if response.Code != 200 || !strings.Contains(response.Body.String(), "skgo: declared prerender inputs are build-only") {
    t.Fatalf("Goja helper invocation status=%d body=%s", response.Code, response.Body.String())
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	gojaTestCmd := exec.Command("go", "test", "./cmd", "-run", "^TestPrerenderHelperIsBuildOnlyInGoja$", "-count=1", "-v")
	gojaTestCmd.Dir = fixture
	gojaTestCmd.Env = append(os.Environ(), "GOWORK=off")
	if output, err := gojaTestCmd.CombinedOutput(); err != nil {
		t.Fatalf("compiled JavaScript-mode Goja helper invocation: %v\n%s", err, output)
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
	if err := linkFixtureDependencyTree(source, destination, adapterPath); err != nil {
		t.Fatal(err)
	}
}

func linkFixtureDependencyTree(source, destination, adapterPath string) error {
	if err := os.MkdirAll(filepath.Join(destination, "@skgo"), 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == "$app" || entry.Name() == ".vite-temp" || entry.Name() == "@skgo" {
			continue
		}
		if err := os.Symlink(filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name())); err != nil {
			return err
		}
	}
	return os.Symlink(adapterPath, filepath.Join(destination, "@skgo", "sveltekit-adapter"))
}
