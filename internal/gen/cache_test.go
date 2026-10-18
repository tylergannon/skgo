package gen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

type cacheRunResult struct {
	Loads int
	Logs  []string
	Error string
}

func TestGenerationCacheProcess(t *testing.T) {
	raw := os.Getenv("SKGO_CACHE_TEST_CONFIG")
	if raw == "" {
		return
	}
	var cfg Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	result := runCache(cfg)
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("CACHE_RESULT=%s\n", data)
}

// Run owns the persisted plan boundary. A fresh invocation has no in-memory
// cache to clear; ordinary hit/miss cases can observe it without another test
// process. Only the persistence witness below needs separate processes.
func runCache(cfg Config) cacheRunResult {
	var result cacheRunResult
	cfg.OnPackageLoad = func(*packages.Config, []string) { result.Loads++ }
	cfg.Logf = func(format string, args ...any) { result.Logs = append(result.Logs, fmt.Sprintf(format, args...)) }
	if err := Run(cfg); err != nil {
		result.Error = err.Error()
	}
	return result
}
func runCacheProcess(t *testing.T, cfg Config) cacheRunResult {
	t.Helper()
	cfg.Logf = nil
	cfg.OnPackageLoad = nil
	// Config's callbacks are intentionally outside the persisted consumer contract.
	raw, err := json.Marshal(struct {
		Web, Out, Package, LocalsPackage, LocalsType, HookPackage, HookSymbol, ConfigPackage, ConfigSymbol, SwiftOut, CacheDir string
		SwiftRemotes                                                                                                           []string
		Language                                                                                                               Language
	}{cfg.Web, cfg.Out, cfg.Package, cfg.LocalsPackage, cfg.LocalsType, cfg.HookPackage, cfg.HookSymbol, cfg.ConfigPackage, cfg.ConfigSymbol, cfg.SwiftOut, cfg.CacheDir, cfg.SwiftRemotes, cfg.Language})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestGenerationCacheProcess$", "-test.count=1")
	cmd.Env = append(os.Environ(), "SKGO_CACHE_TEST_CONFIG="+string(raw))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fresh process: %v\n%s", err, output)
	}
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "CACHE_RESULT=") {
			var result cacheRunResult
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "CACHE_RESULT=")), &result); err != nil {
				t.Fatal(err)
			}
			return result
		}
	}
	t.Fatalf("no cache result: %s", output)
	return cacheRunResult{}
}
func TestGenerationCacheFreshProcessesAndRecovery(t *testing.T) {
	t.Parallel()
	_, cfg := foreignFixture(t, `package data
import("context";"github.com/tylergannon/skgo")
type Reply struct { Text string }
func query(context.Context)(Reply,error){return Reply{},nil}
var _=skgo.Query(query)
`, nil)
	cfg.CacheDir = t.TempDir()
	first := runCacheProcess(t, cfg)
	if first.Error != "" || first.Loads == 0 {
		t.Fatalf("cold: %+v", first)
	}
	typesPath := filepath.Join(cfg.Web, "src/data/types.ts")
	assertCacheFile(t, typesPath, `"Text": string`)
	// A fresh process must replay its persisted plan even when output vanished.
	for _, path := range []string{filepath.Join(cfg.Out, generatedGoFile), typesPath} {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	warm := runCacheProcess(t, cfg)
	if warm.Error != "" || warm.Loads != 0 {
		t.Fatalf("warm did not reuse persisted analysis: %+v", warm)
	}
	assertCacheFile(t, typesPath, `"Text": string`)
	assertCacheFile(t, filepath.Join(cfg.Out, generatedGoFile), `Name:   "query"`)
	source := filepath.Join(cfg.Web, "src/data/data.remote.go")
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, bytes.Replace(raw, []byte("Text string"), []byte("Count int"), 1), 0644); err != nil {
		t.Fatal(err)
	}
	changed := runCache(cfg)
	if changed.Error != "" || changed.Loads == 0 {
		t.Fatalf("source edit was not analyzed: %+v", changed)
	}
	raw, err = os.ReadFile(typesPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"Count": number`)) || bytes.Contains(raw, []byte(`"Text": string`)) {
		t.Fatalf("stale projection: %s", raw)
	}
}

const cacheRemote = `package data
import("context";"github.com/tylergannon/skgo";"example.com/wire")
func query(context.Context)(wire.Thing,error){return wire.Thing{},nil}
var _=skgo.Query(query)
`

func cacheWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
func assertCacheFile(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), want) {
		t.Fatalf("%s missing %q: %s", path, want, data)
	}
}

func TestGenerationCacheRebasesElevenModules(t *testing.T) {
	view := &generationSourceView{roots: map[string]string{}}
	for i := range 11 {
		view.roots[fmt.Sprintf("/authored/project-%d", i)] = fmt.Sprintf("/private/view/module-%d", i)
	}
	for range 20 {
		got := view.originalText(`/private/view/module-10/types.go:7:2: /private/view/module-1/model.go; "/private/view/module-0"; /private/view/module-100/untouched.go`)
		want := `/authored/project-10/types.go:7:2: /authored/project-1/model.go; "/authored/project-0"; /private/view/module-100/untouched.go`
		if got != want {
			t.Fatalf("module prefix corruption: %s", got)
		}
	}
}

func TestGenerationPlanPersistenceAndOwnership(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cfg := Config{Web: filepath.Join(root, "web"), Out: filepath.Join(root, "generated"), Logf: func(string, ...any) {}}
	cacheWrite(t, filepath.Join(root, "go.mod"), "module plan.test\n\ngo 1.27\n")
	stub := filepath.Join(cfg.Web, "src/data.remote.ts")
	values := filepath.Join(cfg.Web, ".svelte-kit/skgo-env-values.js")
	authored := filepath.Join(cfg.Web, "src/handwritten.ts")
	cacheWrite(t, authored, "export const receipt = 541;\n")
	cacheWrite(t, values, "export const values = { SECRET: 'live' };\n")
	plan := generationPlan{Files: map[string]string{stub: tsHeader + "export const query = 'Go';\n", values: "export const values = {};\n"}}
	path := filepath.Join(root, "cache", "fixture.plan")
	entry := generationEntry{InputHash: "fixture-input-541", SourceHash: "fixture-sources-729", Metadata: &kitRouteMetadata{}, Plan: plan}
	if err := writeGenerationEntry(path, entry); err != nil {
		t.Fatal(err)
	}
	got, ok := readGenerationEntry(path)
	if !ok || got.InputHash != "fixture-input-541" || got.SourceHash != "fixture-sources-729" || got.Plan.Files[stub] != tsHeader+"export const query = 'Go';\n" {
		t.Fatalf("persisted plan: %+v, %t", got, ok)
	}
	if err := publishGenerationPlan(cfg, got.Plan); err != nil {
		t.Fatal(err)
	}
	assertCacheFile(t, stub, "export const query = 'Go';")
	assertCacheFile(t, values, "SECRET: 'live'")
	assertCacheFile(t, authored, "receipt = 541")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] ^= 1
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, ok := readGenerationEntry(path); ok {
		t.Fatal("corrupt persisted plan admitted")
	}
}
