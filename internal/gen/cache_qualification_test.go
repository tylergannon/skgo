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
	"reflect"
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
		Web, Out, Package, LocalsPackage, LocalsType, HookPackage, HookSymbol, ConfigPackage, ConfigSymbol, CacheDir string
		Language                                                                                                     Language
	}{cfg.Web, cfg.Out, cfg.Package, cfg.LocalsPackage, cfg.LocalsType, cfg.HookPackage, cfg.HookSymbol, cfg.ConfigPackage, cfg.ConfigSymbol, cfg.CacheDir, cfg.Language})
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
	const remoteSource = `package data
import("context";"github.com/tylergannon/skgo")
type Reply struct { Text string }
func query(context.Context)(Reply,error){return Reply{},nil}
var _=skgo.Query(query)
`
	const pageSource = `package data
import "github.com/tylergannon/skgo"
type PageData struct { Label string }
func load(PageRequestEvent)(PageData,error){return PageData{Label:"published page"},nil}
var _=skgo.Load(load)
`
	_, cfg := foreignFixture(t, "", map[string]string{
		"app/web/src/routes/about/data.remote.go": remoteSource,
		"app/web/src/routes/about/page.server.go": pageSource,
	})
	route := filepath.Join(cfg.Web, "src/routes/about")
	cfg.CacheDir = t.TempDir()
	first := runCacheProcess(t, cfg)
	if first.Error != "" || first.Loads == 0 {
		t.Fatalf("cold: %+v", first)
	}
	typesPath := filepath.Join(route, "types.ts")
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
	source := filepath.Join(route, "data.remote.go")
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

	// Route packages are committed relative directory links, not source copies.
	// A failed analysis must leave declarations and link targets unchanged;
	// edits to authored files remain visible through those links.
	snapshot := func() map[string]string {
		t.Helper()
		published := map[string]string{}
		for _, base := range []string{cfg.Out, route} {
			err := filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
				if err != nil || entry.IsDir() {
					return err
				}
				if entry.Type()&os.ModeSymlink != 0 {
					target, err := os.Readlink(path)
					published[path] = "link:" + target
					return err
				}
				data, err := os.ReadFile(path)
				if path == filepath.Join(cfg.Out, inventoryName) || bytes.HasPrefix(data, []byte(goHeader)) || bytes.HasPrefix(data, []byte(tsHeader)) {
					published[path] = "file:" + string(data)
				}
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
		}
		return published
	}
	linkDir := filepath.Join(cfg.Out, linkRootName, encodeLinkName("src/routes/about"))
	link := filepath.Join(linkDir, "page.server.go")
	target, err := os.Readlink(linkDir)
	if err != nil || filepath.IsAbs(target) || filepath.Clean(filepath.Join(filepath.Dir(linkDir), target)) != route {
		t.Fatalf("published route link: %q, %v", target, err)
	}
	assertCacheFile(t, filepath.Join(linkDir, generatedGoFile), "Skgo_query = query")
	before := snapshot()
	invalid := pageSource + "\nvar _ int = \"unrelated demo consumer is broken\"\n"
	cacheWrite(t, filepath.Join(route, "page.server.go"), invalid)
	cacheWrite(t, filepath.Join(route, "pending.go"), "package data\nconst PendingPublication = 541\n")
	cfg.OnPackageLoad = func(*packages.Config, []string) {
		if !reflect.DeepEqual(snapshot(), before) {
			t.Fatal("published declarations or route links changed before analysis completed")
		}
	}
	err = Run(cfg)
	if err == nil || !strings.Contains(err.Error(), "page.server.go") || !strings.Contains(err.Error(), "cannot use") || !strings.Contains(err.Error(), "unrelated demo consumer is broken") {
		t.Fatalf("authored page failure: %v", err)
	}
	if !reflect.DeepEqual(snapshot(), before) {
		t.Fatal("failed analysis changed published declarations or route links")
	}
	if body, err := os.ReadFile(link); err != nil || string(body) != invalid {
		t.Fatalf("authored edit must remain visible through its link: %q, %v", body, err)
	}
	cfg.OnPackageLoad = nil
	cacheWrite(t, filepath.Join(route, "page.server.go"), pageSource)
	if err := Run(cfg); err != nil {
		t.Fatalf("generation after authored repair: %v", err)
	}
	assertCacheFile(t, filepath.Join(linkDir, generatedGoFile), "Skgo_query = query")
}
