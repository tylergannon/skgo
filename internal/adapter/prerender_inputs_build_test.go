package adapter

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func timedFixtureOutput(t *testing.T, stage string, command *exec.Cmd) ([]byte, error) {
	t.Helper()
	start := time.Now()
	output, err := command.CombinedOutput()
	t.Logf("fixture stage %s: %s", stage, time.Since(start))
	if command.ProcessState != nil {
		t.Logf("fixture stage %s CPU: user=%s system=%s", stage, command.ProcessState.UserTime(), command.ProcessState.SystemTime())
	}
	return output, err
}

func TestDeclaredPrerenderInputsBuildAndProduceNativeArtifacts(t *testing.T) {
	t.Parallel()
	for _, language := range []string{"ts", "js"} {
		t.Run(language, func(t *testing.T) {
			t.Parallel()
			testDeclaredPrerenderInputsBuild(t, language)
		})
	}
}

func testDeclaredPrerenderInputsBuild(t *testing.T, language string) {
	root, err := filepath.Abs("../../example")
	if err != nil {
		t.Fatal(err)
	}
	dependencies := filepath.Join(root, "web", "node_modules")
	if _, err := os.Stat(filepath.Join(dependencies, ".bin", "vp")); err != nil {
		t.Fatalf("pinned frontend dependencies are missing: %v", err)
	}
	fixture := prepareDeclaredInputsApp(t)

	remote := filepath.Join(fixture, "web", "src", "routes", "declared.remote.go")
	declaredSource := `package site

import (
	"context"
	"errors"
	"os"

	"github.com/tylergannon/devalue/v5"
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
	pageSourceText = strings.Replace(pageSourceText, `var _ = skgo.Prerender(buildReceipt, skgo.PrerenderOptions{Inputs: receiptNames})`, `func buildReceiptInputs() ([]string, error) {
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
	viteConfig := filepath.Join(fixture, "web", "vite.config.ts")
	viteSource, err := os.ReadFile(viteConfig)
	if err != nil {
		t.Fatal(err)
	}
	viteSourceText := strings.Replace(string(viteSource), "adapter: skgo({ precompress: false }),", "adapter: skgo({ precompress: false }),\n      prerender: { handleHttpError: \"ignore\" },", 1)
	if viteSourceText == string(viteSource) {
		t.Fatal("could not set permissive native HTTP error policy")
	}
	if err := os.WriteFile(viteConfig, []byte(viteSourceText), 0o644); err != nil {
		t.Fatal(err)
	}
	identifierReceipt := filepath.Join(fixture, "identifier-receipt")
	inputReceipt := filepath.Join(fixture, "inputs-called")
	emptyReceipt := filepath.Join(fixture, "empty-inputs-called")
	moneyReceipt := filepath.Join(fixture, "money-inputs-called")
	pageInputsReceipt := filepath.Join(fixture, "page-inputs-called")
	errorInputsReceipt := filepath.Join(fixture, "error-inputs-called")
	tsconfig := filepath.Join(fixture, "web", "tsconfig.json")
	tsconfigData, err := os.ReadFile(tsconfig)
	if err != nil {
		t.Fatal(err)
	}
	if language == "js" {
		if err := os.Remove(tsconfig); err != nil {
			t.Fatal(err)
		}
		writeFixtureFile(t, filepath.Join(fixture, "web", "jsconfig.json"), `{"extends":"$app/tsconfig","compilerOptions":{"allowJs":true,"checkJs":true,"strict":true}}`)
	}
	generate := exec.Command("go", "generate", "./...")
	generate.Dir = fixture
	generate.Env = append(fixtureBuildEnv(), "GOWORK=off", "SKGO_INPUTS_RECEIPT="+inputReceipt, "SKGO_EMPTY_INPUTS_RECEIPT="+emptyReceipt, "SKGO_MONEY_INPUTS_RECEIPT="+moneyReceipt, "SKGO_PAGE_INPUTS_RECEIPT="+pageInputsReceipt, "SKGO_ERROR_INPUTS_RECEIPT="+errorInputsReceipt, "SKGO_IDENTIFIER_RECEIPT="+identifierReceipt)
	if output, err := timedFixtureOutput(t, "generate", generate); err != nil {
		t.Fatalf("go generate: %v\n%s", err, output)
	}
	installInputsErrorPolicy(t, fixture, language)
	if language == "js" {
		var config map[string]any
		if err := json.Unmarshal(tsconfigData, &config); err != nil {
			t.Fatal(err)
		}
		options, _ := config["compilerOptions"].(map[string]any)
		if options == nil {
			options = map[string]any{}
			config["compilerOptions"] = options
		}
		options["allowJs"], options["checkJs"] = true, true
		data, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		writeFixtureFile(t, tsconfig, string(data))
	}
	for _, receipt := range []string{inputReceipt, emptyReceipt, moneyReceipt, pageInputsReceipt, errorInputsReceipt, identifierReceipt} {
		if _, err := os.Stat(receipt); !os.IsNotExist(err) {
			t.Fatalf("producer ran during generation: %s (%v)", receipt, err)
		}
	}
	build := exec.Command(filepath.Join(fixture, "web", "node_modules", ".bin", "vp"), "build")
	build.Dir = filepath.Join(fixture, "web")
	remoteReceipt := filepath.Join(fixture, "remotes-called")
	pageRemoteReceipt := filepath.Join(fixture, "page-remotes-called")
	nativeErrorReceipt := filepath.Join(fixture, "native-errors")
	build.Env = append(fixtureBuildEnv(), "GOWORK=off", "ORIGIN=http://127.0.0.1:8080", "SKGO_INPUTS_RECEIPT="+inputReceipt, "SKGO_EMPTY_INPUTS_RECEIPT="+emptyReceipt, "SKGO_MONEY_INPUTS_RECEIPT="+moneyReceipt, "SKGO_PAGE_INPUTS_RECEIPT="+pageInputsReceipt, "SKGO_ERROR_INPUTS_RECEIPT="+errorInputsReceipt, "SKGO_REMOTE_RECEIPT="+remoteReceipt, "SKGO_PAGE_REMOTE_RECEIPT="+pageRemoteReceipt, "SKGO_NATIVE_ERROR_RECEIPT="+nativeErrorReceipt, "SKGO_IDENTIFIER_RECEIPT="+identifierReceipt)
	buildOutput, err := timedFixtureOutput(t, "native build", build)
	if err != nil {
		t.Fatalf("vp build: %v\n%s", err, buildOutput)
	}
	// Kit's native build calls sync.all, including all route and app types.
	// Check that output rather than repeating Kit sync in a second process.
	check := exec.Command(filepath.Join(fixture, "web", "node_modules", ".bin", "svelte-check"), "--tsconfig", "./tsconfig.json")
	check.Dir = filepath.Join(fixture, "web")
	check.Env = append(fixtureBuildEnv(), "GOWORK=off")
	if output, err := timedFixtureOutput(t, "svelte-check", check); err != nil {
		t.Fatalf("frontend typecheck: %v\n%s", err, output)
	}
	t.Run("authored remote identifier", func(t *testing.T) { assertAuthoredInputsIdentifier(t, fixture, language) })
	started := regexp.MustCompile(`(?m)^skgo prerender service started \(pid ([0-9]+)\)$`).FindAllStringSubmatch(string(buildOutput), -1)
	stopped := regexp.MustCompile(`(?m)^skgo prerender service stopped \(pid ([0-9]+)\)$`).FindAllStringSubmatch(string(buildOutput), -1)
	if len(started) != 1 || len(stopped) != 1 || started[0][1] != stopped[0][1] {
		t.Fatalf("build did not start and stop exactly one Go helper:\n%s", buildOutput)
	}
	helperPID, err := strconv.Atoi(started[0][1])
	if err != nil || helperPID < 2 {
		t.Fatalf("invalid build helper PID %q: %v", started[0][1], err)
	}
	if runtime.GOOS != "windows" {
		assertInputsProcessGroupGone(t, helperPID)
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
	if language != "js" {
		return
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

  "github.com/tylergannon/skgo"
  generated "github.com/tylergannon/skgo/example/internal/skgo"
  "github.com/tylergannon/skgo/example/web"
)

func TestPrerenderHelperIsBuildOnlyInGoja(t *testing.T) {
  dist, err := fs.Sub(web.Build, "build")
  if err != nil { t.Fatal(err) }
  manifest,err:=skgo.ReadManifest(dist);if err!=nil{t.Fatal(err)}
  loads,err:=skgo.NewLoads(manifest.LoadConfig("http://127.0.0.1:8080"),generated.Loads()...);if err!=nil{t.Fatal(err)}
  remotes,err:=skgo.NewRemotes(manifest.RemoteConfig("http://127.0.0.1:8080"),generated.Remotes()...);if err!=nil{t.Fatal(err)}
  renderer,err:=skgo.NewSSR(dist,manifest,loads,remotes,skgo.SSROptions{Runtimes:1});if err!=nil{t.Fatal(err)}
  handler,err:=skgo.NewStaticHandler(dist,skgo.WithSSR(renderer));if err!=nil{t.Fatal(err)}
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
	gojaTestCmd.Env = append(fixtureBuildEnv(), "GOWORK=off")
	if output, err := timedFixtureOutput(t, "compiled consumer", gojaTestCmd); err != nil || !strings.Contains(string(output), "--- PASS: TestPrerenderHelperIsBuildOnlyInGoja") || strings.Contains(string(output), "--- SKIP:") {
		t.Fatalf("compiled JavaScript-mode Goja helper invocation: %v\n%s", err, output)
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

// The native error-policy contract injects only its test policy after the
// generator has installed the Go-owned handle. Applications cannot author it.
func installInputsErrorPolicy(t *testing.T, fixture, extension string) {
	t.Helper()
	path := filepath.Join(fixture, "web", "src", "hooks.server."+extension)
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	annotation := "({ error, kind }: { error: unknown; kind: string })"
	if extension == "js" {
		annotation = "({ error, kind })"
	}
	source = append(source, []byte(`
import { appendFileSync } from "node:fs";
import { getDeclaredSite, moneyRemote, noArgumentRemote, appFailureRemote, unknownFailureRemote, redirectRemote } from "./routes/declared.remote";
void [getDeclaredSite, moneyRemote, noArgumentRemote, appFailureRemote, unknownFailureRemote, redirectRemote];
/** @param {{ error: unknown, kind: string }} input */
export const handleError = `+annotation+` => {
 const diagnostic = error instanceof Error ? error.message : JSON.stringify(error);
 appendFileSync(process.env.SKGO_NATIVE_ERROR_RECEIPT`+func() string {
		if extension == "ts" {
			return "!"
		}
		return " ?? \"\""
	}()+`, kind + "|" + diagnostic + "\n");
 return { message: kind === "app" ? "app-policy" : "Internal Error" };
};
`)...)
	if err := os.WriteFile(path, source, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestMalformedPrerenderInputsRejectsPermissiveKitBuild(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs("../../example")
	if err != nil {
		t.Fatal(err)
	}
	source, err := generatedMinimalInputsApp()
	if err != nil {
		t.Fatal(err)
	}
	fixture := cloneGeneratedInputsWeb(t, source)
	configPath := filepath.Join(fixture, "web", "vite.config.ts")
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	const adapterConfig = "adapter: skgo({ precompress: false }),"
	if strings.Count(string(config), adapterConfig) != 1 {
		t.Fatal("could not install the permissive native HTTP policy")
	}
	writeFixtureFile(t, configPath, strings.Replace(string(config), adapterConfig, adapterConfig+" prerender: {handleHttpError:'ignore'},", 1))
	nativeErrorReceipt := filepath.Join(fixture, "native-errors")
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
	malformedBuild.Env = append(fixtureBuildEnv(), "GOWORK=off", "ORIGIN=http://127.0.0.1:8080", "GOFLAGS="+os.Getenv("GOFLAGS")+" -ldflags=-w -overlay="+overlayPath, "SKGO_FORCE_BAD_INPUT_SHAPE=1", "SKGO_NATIVE_ERROR_RECEIPT="+nativeErrorReceipt)
	malformedOutput, malformedErr := malformedBuild.CombinedOutput()
	if malformedErr == nil || !strings.Contains(string(malformedOutput), "did not decode to an array") {
		t.Fatalf("permissive native HTTP policy accepted malformed Go Inputs shape: err=%v\n%s", malformedErr, malformedOutput)
	}
}
