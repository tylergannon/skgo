//go:build skgo_qualification

package gen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"golang.org/x/tools/go/packages"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
