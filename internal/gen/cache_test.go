package gen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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
	var result cacheRunResult
	cfg.OnPackageLoad = func(*packages.Config, []string) { result.Loads++ }
	cfg.Logf = func(format string, args ...any) { result.Logs = append(result.Logs, fmt.Sprintf(format, args...)) }
	if err := Run(cfg); err != nil {
		result.Error = err.Error()
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("CACHE_RESULT=%s\n", data)
}
func runCacheProcess(t *testing.T, cfg Config) cacheRunResult {
	t.Helper()
	cfg.Logf = nil
	cfg.OnPackageLoad = nil
	// Config's callbacks are intentionally outside the persisted consumer contract.
	raw, err := json.Marshal(struct {
		Web, Out, Package, LocalsPackage, LocalsType, HookPackage, HookSymbol, SwiftOut, CacheDir string
		SwiftRemotes                                                                              []string
		Language                                                                                  Language
	}{cfg.Web, cfg.Out, cfg.Package, cfg.LocalsPackage, cfg.LocalsType, cfg.HookPackage, cfg.HookSymbol, cfg.SwiftOut, cfg.CacheDir, cfg.SwiftRemotes, cfg.Language})
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
	root, cfg := foreignFixture(t, `package data
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
	baseline := generatedTree(t, filepath.Join(root, "app"))
	typesPath := filepath.Join(cfg.Web, "src/data/types.ts")
	raw, err := os.ReadFile(typesPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"Text": string`)) {
		t.Fatalf("literal Reply projection missing: %s", raw)
	}
	warm := runCacheProcess(t, cfg)
	if warm.Error != "" || warm.Loads != 0 {
		t.Fatalf("warm did not reuse analysis: %+v", warm)
	}
	if !reflect.DeepEqual(baseline, generatedTree(t, filepath.Join(root, "app"))) {
		t.Fatal("warm changed output")
	}
	for _, path := range []string{filepath.Join(cfg.Out, generatedGoFile), typesPath} {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	recovered := runCacheProcess(t, cfg)
	if recovered.Error != "" || recovered.Loads != 0 {
		t.Fatalf("recovery did not reuse analysis: %+v", recovered)
	}
	if !reflect.DeepEqual(baseline, generatedTree(t, filepath.Join(root, "app"))) {
		t.Fatal("recovery did not restore all output")
	}
	source := filepath.Join(cfg.Web, "src/data/data.remote.go")
	raw, err = os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, bytes.Replace(raw, []byte("Text string"), []byte("Count int"), 1), 0644); err != nil {
		t.Fatal(err)
	}
	changed := runCacheProcess(t, cfg)
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

func assertCacheResult(t *testing.T, cfg Config, hit bool) cacheRunResult {
	t.Helper()
	got := runCacheProcess(t, cfg)
	if got.Error != "" || (got.Loads == 0) != hit {
		t.Fatalf("hit=%t: %+v", hit, got)
	}
	for _, log := range got.Logs {
		if strings.Contains(log, "cache bypass") || strings.Contains(log, "write skipped") {
			t.Fatalf("cache not exercised: %s", log)
		}
	}
	return got
}
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

func TestGenerationCacheDependencyAndMembership(t *testing.T) {
	root, cfg := foreignFixture(t, cacheRemote, nil)
	cfg.CacheDir = t.TempDir()
	assertCacheResult(t, cfg, false)
	assertCacheResult(t, cfg, true)
	dep := filepath.Join(root, "wire/wire.go")
	cacheWrite(t, dep, "package wire\ntype Thing struct { Quantity int `json:\"quantity\"` }\n")
	assertCacheResult(t, cfg, false)
	assertCacheFile(t, filepath.Join(cfg.Web, "src/lib/skgo/example.com/wire/types.ts"), `"quantity": number`)
	extra := filepath.Join(cfg.Web, "src/data/added.go")
	cacheWrite(t, extra, "package data\nconst added = missingSymbol\n")
	broken := runCacheProcess(t, cfg)
	if broken.Error == "" || !strings.Contains(broken.Error, "missingSymbol") {
		t.Fatalf("invalid addition hidden: %+v", broken)
	}
	if err := os.Remove(extra); err != nil {
		t.Fatal(err)
	}
	assertCacheResult(t, cfg, true) // failed runs must not poison the prior successful entry
	if err := os.Remove(filepath.Join(cfg.Web, "src/data/data.remote.go")); err != nil {
		t.Fatal(err)
	}
	// Keep src/ itself; deletion must invalidate and prune old remote output.
	assertCacheResult(t, cfg, false)
	if _, err := os.Stat(filepath.Join(cfg.Web, "src/data/data.remote.ts")); !os.IsNotExist(err) {
		t.Fatalf("deleted remote remains: %v", err)
	}
	assertCacheFile(t, filepath.Join(cfg.Out, generatedGoFile), "package generated")
}

func TestGenerationCacheDisposableOutputsAndCorruption(t *testing.T) {
	_, cfg := foreignFixture(t, cacheRemote, nil)
	cfg.CacheDir = t.TempDir()
	assertCacheResult(t, cfg, false)
	binding := filepath.Join(cfg.Out, generatedGoFile)
	for _, cold := range []bool{false, true} {
		cacheWrite(t, binding, goHeader+"\nTHIS IS NOT GO\n")
		cacheWrite(t, filepath.Join(cfg.Web, "src/data/data.remote.ts"), "// Code generated by skgo. DO NOT EDIT.\nBROKEN PREVIOUS OUTPUT\n")
		if cold {
			entries, err := os.ReadDir(cfg.CacheDir)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				cacheWrite(t, filepath.Join(cfg.CacheDir, entry.Name()), "corrupt")
			}
		}
		assertCacheResult(t, cfg, !cold)
		assertCacheFile(t, binding, "Name:   \"query\"")
		assertCacheFile(t, filepath.Join(cfg.Web, "src/data/data.remote.ts"), "query")
	}
	authored := "package generated\nconst Mine = 7\n"
	cacheWrite(t, binding, authored)
	result := runCacheProcess(t, cfg)
	if !strings.Contains(result.Error, "authored") {
		t.Fatalf("authored takeover accepted: %+v", result)
	}
	assertCacheFile(t, binding, authored)
}

func TestGenerationCacheSourceIsCapturedBeforeAnalysis(t *testing.T) {
	root, cfg := foreignFixture(t, cacheRemote, nil)
	cfg.CacheDir = t.TempDir()
	dep := filepath.Join(root, "wire/wire.go")
	original, err := os.ReadFile(dep)
	if err != nil {
		t.Fatal(err)
	}
	loads := 0
	mutated, restored := false, false
	cfg.OnPackageLoad = func(load *packages.Config, patterns []string) {
		loads++
		if mutated && !restored {
			cacheWrite(t, dep, string(original))
			restored = true
		}
		if !mutated && strings.Contains(strings.Join(patterns, " "), "/web/src/data") {
			cacheWrite(t, dep, "package wire\ntype Thing struct { Poison bool `json:\"poison\"` }\n")
			mutated = true
			if !strings.Contains(load.Dir, "skgo-analysis-") {
				t.Fatal("analysis is reading the live tree")
			}
		}
	}
	var logs []string
	cfg.Logf = func(f string, a ...any) { logs = append(logs, fmt.Sprintf(f, a...)) }
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	if loads < 2 || !mutated || !restored {
		t.Fatalf("did not exercise mid-analysis edit: %d", loads)
	}
	for _, log := range logs {
		if strings.Contains(log, "cache bypass") {
			t.Fatalf("snapshot bypassed: %s", log)
		}
	}
	cfg.OnPackageLoad = nil
	cfg.Logf = nil
	assertCacheFile(t, filepath.Join(cfg.Web, "src/lib/skgo/example.com/wire/types.ts"), `"name": string`)
	assertCacheResult(t, cfg, true)
	assertCacheFile(t, filepath.Join(cfg.Web, "src/lib/skgo/example.com/wire/types.ts"), `"name": string`)
}

func TestGenerationCacheWorkspaceAndConfiguration(t *testing.T) {
	root, cfg := foreignFixture(t, cacheRemote, nil)
	cfg.CacheDir = t.TempDir()
	work := filepath.Join(root, "go.work")
	cacheWrite(t, work, "go 1.27.1\nuse (\n"+filepath.Join(root, "app")+"\n"+filepath.Join(root, "wire")+"\n"+filepath.Join(root, "skgo")+"\n)\n")
	t.Setenv("GOWORK", work)
	assertCacheResult(t, cfg, false)
	assertCacheResult(t, cfg, true)
	cfg.Language = LanguageJavaScript
	assertCacheResult(t, cfg, false)
	assertCacheFile(t, filepath.Join(cfg.Web, "src/data/data.remote.js"), "query")
	if _, err := os.Stat(filepath.Join(cfg.Web, "src/data/data.remote.ts")); !os.IsNotExist(err) {
		t.Fatalf("stale language output: %v", err)
	}
	assertCacheResult(t, cfg, true)
	t.Setenv("GOFLAGS", "-tags=cachetest")
	assertCacheResult(t, cfg, false)
	assertCacheResult(t, cfg, true)
}

func TestGenerationCacheUnavailableStorage(t *testing.T) {
	_, cfg := foreignFixture(t, cacheRemote, nil)
	cfg.CacheDir = filepath.Join(t.TempDir(), "not-directory")
	cacheWrite(t, cfg.CacheDir, "file")
	got := runCacheProcess(t, cfg)
	if got.Error != "" || got.Loads == 0 {
		t.Fatalf("cache became required state: %+v", got)
	}
	if !strings.Contains(strings.Join(got.Logs, "\n"), "write skipped") {
		t.Fatalf("write failure not observed: %+v", got)
	}
	assertCacheFile(t, filepath.Join(cfg.Out, generatedGoFile), "Name:   \"query\"")
}

func TestGenerationCacheEnvironmentBootstrapIsLive(t *testing.T) {
	root, cfg := foreignFixture(t, "package data", nil)
	cfg.CacheDir = t.TempDir()
	cacheWrite(t, filepath.Join(root, "skgo/env.go"), "package skgo\nfunc Environment[T any]() Marker{return Marker{}}\n")
	cacheWrite(t, filepath.Join(cfg.Web, "src/env.go"), "package appenv\nimport s \"github.com/tylergannon/skgo\"\ntype Config struct { Site string `env:\"SITE,public\"` }\nvar _=s.Environment[Config]()\n")
	values := filepath.Join(cfg.Web, ".svelte-kit/skgo-env-values.js")
	const current = "export const values = {SITE: 'live adapter value'};\n"
	cacheWrite(t, values, current)
	assertCacheResult(t, cfg, false)
	assertCacheFile(t, values, current)
	assertCacheResult(t, cfg, true)
	assertCacheFile(t, values, current)
	if err := os.Remove(values); err != nil {
		t.Fatal(err)
	}
	assertCacheResult(t, cfg, true)
	assertCacheFile(t, values, "export const values = {};")
	assertCacheFile(t, filepath.Join(cfg.Web, "src/env.ts"), `"SITE"`)
	cacheWrite(t, filepath.Join(cfg.Web, "src/env.ts"), "export const mine = 1;\n")
	result := runCacheProcess(t, cfg)
	if !strings.Contains(result.Error, "authored") {
		t.Fatalf("authored env overwritten: %+v", result)
	}
	assertCacheFile(t, filepath.Join(cfg.Web, "src/env.ts"), "export const mine = 1;")
}

func TestGenerationCachePrepareAndCheckRemainFresh(t *testing.T) {
	_, cfg := foreignFixture(t, cacheRemote, nil)
	cfg.CacheDir = t.TempDir()
	assertCacheResult(t, cfg, false)
	loads := 0
	cfg.OnPackageLoad = func(*packages.Config, []string) { loads++ }
	prepared, err := Prepare(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	if loads == 0 || len(prepared.Packages()) == 0 {
		t.Fatal("Prepare lost its discovery graph")
	}
	loads = 0
	if err := Check(cfg); err != nil {
		t.Fatal(err)
	}
	if loads == 0 {
		t.Fatal("Check skipped analysis")
	}
	cacheWrite(t, filepath.Join(cfg.Web, "src/data/broken.go"), "package data\nvar broken = notDeclared\n")
	if err := Check(cfg); err == nil || !strings.Contains(err.Error(), "notDeclared") {
		t.Fatalf("Check used stale result: %v", err)
	}
}

func TestGenerationCacheChecksConsumedGoEnvironment(t *testing.T) {
	_, cfg := foreignFixture(t, cacheRemote, nil)
	cfg.CacheDir = t.TempDir()
	assertCacheResult(t, cfg, false)
	entries, err := os.ReadDir(cfg.CacheDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("cache entries: %v %v", entries, err)
	}
	path := filepath.Join(cfg.CacheDir, entries[0].Name())
	entry, ok := readGenerationEntry(path)
	if !ok {
		t.Fatal("missing successful plan")
	}
	// Model a GOENV A->B->A during acquisition: the content fingerprint alone
	// matches A, but the computation consumed B's effective build configuration.
	entry.Sources.Environment["GOFLAGS"] = "-tags=anotherconfig"
	if err := writeGenerationEntry(path, entry); err != nil {
		t.Fatal(err)
	}
	assertCacheResult(t, cfg, false)
	assertCacheResult(t, cfg, true)
}

func TestGenerationCacheIgnoresToolingSymlinks(t *testing.T) {
	root, cfg := foreignFixture(t, cacheRemote, nil)
	cfg.CacheDir = t.TempDir()
	tools := filepath.Join(root, "app/.claude/skills")
	if err := os.MkdirAll(tools, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(tools, "outside")); err != nil {
		t.Fatal(err)
	}
	assertCacheResult(t, cfg, false)
	assertCacheResult(t, cfg, true)
	// Explicit imports change the boundary: such source is never silently omitted.
	cacheWrite(t, filepath.Join(root, "app/.claude/value/value.go"), "package value\ntype Value int\n")
	cacheWrite(t, filepath.Join(cfg.Web, "src/data/extra.go"), "package data\nimport \"example.com/app/.claude/value\"\nvar _ value.Value\n")
	result := runCacheProcess(t, cfg)
	if result.Error != "" || !strings.Contains(strings.Join(result.Logs, "\n"), "explicit import in tooling directory") {
		t.Fatalf("explicit source omitted: %+v", result)
	}
	assertCacheFile(t, filepath.Join(cfg.Out, generatedGoFile), "Name:   \"query\"")
}
