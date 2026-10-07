package gen

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestGenerationOwnsOneFileAndIgnoresPreviousDeclarations(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	app := t.TempDir()
	if err := stageTypedLoadEvolutionFixture(root, app); err != nil {
		t.Fatal(err)
	}
	cfg := fixtureConfig(Config{Web: filepath.Join(app, "web"), Out: filepath.Join(app, "internal", "skgo")})
	route := filepath.Join(cfg.Web, "src", "routes", "typed-load", "[number=Order]")
	source := filepath.Join(route, "save.remote.go")
	authored := `package typedload
import (
 "context"
 "github.com/tylergannon/skgo"
 "github.com/tylergannon/skgo/example/internal/skgo/params"
)
type Draft struct { Text string }
type Receipt struct { Text string }
func save(_ context.Context, _ params.RequestEvent, in Draft) (Receipt,error) { return Receipt{Text:in.Text},nil }
var _ = skgo.Form(save)
`
	if err := os.WriteFile(source, []byte(authored), 0644); err != nil {
		t.Fatal(err)
	}
	run := func() {
		t.Helper()
		if err := Run(cfg); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := func() map[string][]byte {
		t.Helper()
		out := map[string][]byte{}
		for _, base := range []string{filepath.Join(cfg.Web, "src"), cfg.Out} {
			err := filepath.WalkDir(base, func(path string, e os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
					return nil
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				if !bytes.HasPrefix(data, []byte(goHeader)) {
					return nil
				}
				if e.Name() != "skgo_gen.go" {
					t.Errorf("unexpected generated filename %s", path)
				}
				out[path] = data
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		}
		return out
	}
	initial := start(func() error { return Run(cfg) })
	t.Parallel()
	if err := initial(); err != nil {
		t.Fatal(err)
	}
	expected := snapshot()
	// Two authored routes and their encoded copies, plus server, shared params,
	// form client, and prerender command. This count comes from the fixture.
	if len(expected) != 8 {
		t.Fatalf("got %d generated files, want 8", len(expected))
	}
	// Exercise every previous-output state in one regeneration. Initial Run
	// already exercised the all-absent bootstrap; each owned file must return
	// byte for byte regardless of its old declarations.
	paths := make([]string, 0, len(expected))
	for path := range expected {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for i, path := range paths {
		switch i % 3 {
		case 0:
			if err := os.WriteFile(path, []byte(goHeader+"this is not Go syntax\n"), 0644); err != nil {
				t.Fatal(err)
			}
		case 1:
			if err := os.WriteFile(path, []byte(goHeader+"package stale\nvar _ = RemovedRuntimeSymbol\n"), 0644); err != nil {
				t.Fatal(err)
			}
		case 2:
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		}
	}
	run()
	if !reflect.DeepEqual(snapshot(), expected) {
		t.Fatal("generation depended on broken or absent previous declarations")
	}
	build := exec.Command("go", "test", "./internal/skgo/...")
	build.Dir = app
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("generated consumers: %v\n%s", err, output)
	}
	// Genuine authored errors retain diagnostics and every previous Go output.
	if err := os.WriteFile(source, []byte(authored+"\nvar _ = AuthoredTypeError\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Run(cfg); err == nil || !strings.Contains(err.Error(), "save.remote.go") || !strings.Contains(err.Error(), "AuthoredTypeError") {
		t.Fatalf("authored error: %v", err)
	}
	if !reflect.DeepEqual(snapshot(), expected) {
		t.Fatal("failed generation changed output")
	}
	if err := os.WriteFile(source, []byte(authored), 0644); err != nil {
		t.Fatal(err)
	}
	// A user-owned destination is a refusal, even at the reserved filename.
	target := filepath.Join(cfg.Out, "client", "skgo_gen.go")
	const userSource = "package client\nconst Authored = 42\n"
	if err := os.WriteFile(target, []byte(userSource), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Run(cfg); err == nil || !strings.Contains(err.Error(), "authored") {
		t.Fatalf("authored output collision: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != userSource {
		t.Fatalf("authored source changed: %v", err)
	}
	for path, want := range expected {
		if path == target {
			continue
		}
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("collision changed %s: %v", path, err)
		}
	}
	// Remove the form capability; its generated client must disappear.
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	// Remove a load in the same transition. The modules demonstrably existed
	// before removal; Run must invoke pruning, not merely drop registrations.
	for _, name := range []string{"save.remote.ts", "+page.server.ts", "types.ts"} {
		if _, err := os.Stat(filepath.Join(route, name)); err != nil {
			t.Fatalf("missing removal prerequisite %s: %v", name, err)
		}
	}
	if err := os.Remove(filepath.Join(route, "page.server.go")); err != nil {
		t.Fatal(err)
	}
	run()
	for _, name := range []string{"save.remote.ts", "+page.server.ts", "types.ts"} {
		if _, err := os.Stat(filepath.Join(route, name)); !os.IsNotExist(err) {
			t.Errorf("orphaned %s survived generation: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(route, "+page.svelte")); err != nil {
		t.Fatalf("authored component removed: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("removed form left client: %v", err)
	}
	build = exec.Command("go", "test", "./internal/skgo/...")
	build.Dir = app
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("consumers after form removal: %v\n%s", err, output)
	}
}

func TestMergeGoKeepsFileLocalImportsAndLocalShadows(t *testing.T) {
	t.Parallel()
	first := `package fixture
import value "strings"
func Upper(s string) string { return value.ToUpper(s) }
`
	second := `package fixture
import value "strconv"
func Number() string { return value.Itoa(42) }
func Shadow() int { value:=struct{N int}{N:7}; return value.N }
`
	merged, err := mergeGo([]string{first, second})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for name, content := range map[string]string{
		"go.mod": "module merged\n\ngo 1.27\n", "skgo_gen.go": merged,
		"consumer_test.go": `package fixture
import "testing"
func TestValues(t *testing.T){if Upper("go")!="GO" || Number()!="42" || Shadow()!=7 {t.Fatal("merged declarations changed behavior")}}
`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("go", "test", "-v", ".")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil || !strings.Contains(string(output), "--- PASS: TestValues") {
		t.Fatalf("merged consumer: %v\n%s", err, output)
	}
}

func TestFormatterFailureDoesNotPublishGeneratedOutput(t *testing.T) {
	t.Parallel()
	root, cfg := foreignFixture(t, `package data
import("context";"github.com/tylergannon/skgo")
func answer(context.Context)(string,error){return "initial",nil}
var _ = skgo.Query(answer)
`, nil)
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	authored := filepath.Join(cfg.Web, "src", "data", "data.remote.go")
	data, err := os.ReadFile(authored)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(authored, bytes.ReplaceAll(data, []byte("answer"), []byte("changed")), 0644); err != nil {
		t.Fatal(err)
	}
	before := checkSourceSnapshot(t, filepath.Join(cfg.Web, "src"), filepath.Join(cfg.Web, "skgo.remotes.json"), cfg.Out)
	helper := filepath.Join(root, "formatter.go")
	if err := os.WriteFile(helper, []byte(`package main
import("fmt";"os")
func main(){fmt.Fprintln(os.Stderr,"fixture formatter failure");os.Exit(1)}
`), 0644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(cfg.Web, "node_modules", ".bin", "vp")
	if err := os.MkdirAll(filepath.Dir(bin), 0755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", bin, helper)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("formatter fixture: %v\n%s", err, output)
	}
	if err := Run(cfg); err == nil || !strings.Contains(err.Error(), "fixture formatter failure") {
		t.Fatalf("formatter failure: %v", err)
	}
	if after := checkSourceSnapshot(t, filepath.Join(cfg.Web, "src"), filepath.Join(cfg.Web, "skgo.remotes.json"), cfg.Out); !reflect.DeepEqual(before, after) {
		t.Fatal("formatter failure published generated output")
	}
	if err := os.Remove(bin); err != nil {
		t.Fatal(err)
	}
	if err := Run(cfg); err != nil {
		t.Fatalf("recovery after formatter failure: %v", err)
	}
	generated, err := os.ReadFile(filepath.Join(cfg.Out, "skgo_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(generated, []byte("Skgo_changed")) {
		t.Fatal("recovered generation did not publish changed remote")
	}
}
