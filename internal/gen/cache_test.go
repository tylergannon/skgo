package gen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
