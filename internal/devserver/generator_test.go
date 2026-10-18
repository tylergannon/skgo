package devserver

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func generatorFixture(t *testing.T) (*generator, string, *bytes.Buffer) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/generatorfixture\n\ngo 1.27.1\n")
	write("tool/main.go", `package main
import("encoding/json";"os")
func main(){dec:=json.NewDecoder(os.Stdin);enc:=json.NewEncoder(os.Stdout);for {var r struct{ID uint64};if dec.Decode(&r)!=nil{return};enc.Encode(struct{ID uint64 `+"`json:\"id\"`"+`;Generated bool `+"`json:\"generated\"`"+`}{r.ID,false})}}
`)
	write("generator.json", `{"package":"./tool"}`)
	var log bytes.Buffer
	g, err := newGenerator(Options{Root: root, GoTool: "go", GeneratorConfig: "generator.json", Output: &log}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(g.close)
	return g, root, &log
}
func TestGeneratorRefreshRetainsWorkersAndRejectsStaleCode(t *testing.T) {
	g, root, log := generatorFixture(t)
	ctx := context.Background()
	changed, err := g.refresh(ctx)
	if err != nil || !changed {
		t.Fatalf("initial refresh=%t %v %s", changed, err, log)
	}
	first := g.child.cmd.Process.Pid
	if changed, err = g.refresh(ctx); err != nil || changed || g.child.cmd.Process.Pid != first {
		t.Fatalf("warm refresh=%t %v", changed, err)
	}
	path := filepath.Join(root, "tool", "main.go")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("package main\ninvalid syntax"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = g.refresh(ctx); err == nil || g.child != nil || !g.rebuild {
		t.Fatalf("stale helper survived failure: %v", err)
	}
	if err = os.WriteFile(path, source, 0644); err != nil {
		t.Fatal(err)
	}
	if changed, err = g.refresh(ctx); err != nil || !changed || g.child.cmd.Process.Pid == first {
		t.Fatalf("repair refresh=%t %v %s", changed, err, log)
	}
	// New selected file membership must refresh even though all old bytes match.
	if err = os.WriteFile(filepath.Join(root, "tool", "extra.go"), []byte("package main\nvar newlySelected=17\n"), 0644); err != nil {
		t.Fatal(err)
	}
	previous := g.child.cmd.Process.Pid
	if changed, err = g.refresh(ctx); err != nil || !changed || g.child.cmd.Process.Pid == previous {
		t.Fatalf("new membership refresh=%t %v", changed, err)
	}
}
func TestGeneratorTargetedPrerequisiteRepairsBeforeBuild(t *testing.T) {
	g, root, log := generatorFixture(t)
	prereq := `package main
import("os")
func main(){if err:=os.WriteFile("../tool/bootstrap.go",[]byte("package main\nconst bootstrap=\"literal-ready\"\n"),0644);err!=nil{panic(err)}}`
	if err := os.MkdirAll(filepath.Join(root, "prerequisite"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "prerequisite", "main.go"), []byte(prereq), 0644); err != nil {
		t.Fatal(err)
	}
	helper := `package main
import("fmt")
func main(){fmt.Println(bootstrap)}`
	if err := os.WriteFile(filepath.Join(root, "tool", "main.go"), []byte(helper), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "generator.json"), []byte(`{"package":"./tool","beforeBuild":{"dir":"prerequisite","argv":["go","run","."]}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := g.refresh(context.Background()); err != nil {
		t.Fatalf("bootstrap did not precede compilation: %v %s", err, log)
	}
	got, err := os.ReadFile(filepath.Join(root, "tool", "bootstrap.go"))
	if err != nil || !strings.Contains(string(got), "literal-ready") {
		t.Fatalf("bootstrap=%s %v", got, err)
	}
}

func TestGeneratorDiscoversSelectedAssemblyInputs(t *testing.T) {
	g, root, _ := generatorFixture(t)
	if err := os.MkdirAll(filepath.Join(root, "native"), 0755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("native/native.go", "package native\nfunc Literal() int64\n")
	selected := "native/literal_" + runtime.GOARCH + ".s"
	other := "arm64"
	if runtime.GOARCH == other {
		other = "amd64"
	}
	ignored := "native/literal_" + other + ".s"
	// Assembly execution belongs to Go. SKGO must retain the compiler-selected
	// assembly path in the helper's input inventory, and omit the other platform.
	write(selected, "// selected assembly input\n")
	write(ignored, "// ignored assembly input\n")
	write("tool/main.go", "package main\nimport _ \"example.com/generatorfixture/native\"\nfunc main(){}\n")
	cfg, _, err := g.readConfig()
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := g.discover(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(inputs, filepath.Join(root, filepath.FromSlash(selected))) || slices.Contains(inputs, filepath.Join(root, filepath.FromSlash(ignored))) {
		t.Fatalf("generator input inventory must contain %s and omit %s: %v", selected, ignored, inputs)
	}
}
