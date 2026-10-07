package advice

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/gen"
)

func TestCatalogRepairExamplesCompileAgainstThisSkgo(t *testing.T) {
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	mod := "module example.com/repairs\n\ngo 1.27.1\n\nrequire github.com/tylergannon/skgo v0.0.0\n\nreplace github.com/tylergannon/skgo => " + repo + "\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	var routePath, routeSource string
	for i, entry := range Catalog() {
		dir := filepath.Join(root, fmt.Sprintf("rule%03d", i+1))
		if entry.Code == RouteParam {
			dir = filepath.Join(root, "web", "src", "routes", "[slug]")
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		imports := "skgo \"github.com/tylergannon/skgo\""
		if strings.Contains(entry.Example, "params.Params") {
			imports += "; params \"example.com/repairs/generated/params\""
		}
		if strings.Contains(entry.Example, "context.") {
			imports += "; \"context\""
		}
		source := "package repair\n\nimport (" + imports + ")\n\n" + entry.Example + "\n"
		name := "repair.go"
		if entry.Code == RouteParam {
			routePath, routeSource = filepath.Join(dir, "page.server.go"), source
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// The route-param repair is authored against a generated route-local
	// event, just like an application load. Compile actual generator output,
	// rather than a hand-written stand-in for that public API.
	if err := os.WriteFile(filepath.Join(root, "web", "package.json"), []byte(`{"name":"skgo-advice-repair","private":true,"type":"module"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(repo, "example", "web", "node_modules"), filepath.Join(root, "web", "node_modules")); err != nil {
		t.Fatal(err)
	}
	// Bootstrap the shared params leaf before tidy loads illustrative consumers.
	if err := gen.Run(gen.Config{Web: filepath.Join(root, "web"), Out: filepath.Join(root, "generated")}); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"mod", "tidy"}, {"test", "./..."}} {
		if args[0] == "test" {
			// Tidy the ordinary repair packages first, while the route tree
			// is empty. Generation then creates its module boundary and links.
			if err := os.WriteFile(routePath, []byte(routeSource), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := gen.Run(gen.Config{Web: filepath.Join(root, "web"), Out: filepath.Join(root, "generated")}); err != nil {
				t.Fatalf("generate route-param repair: %v", err)
			}
		}
		cmd := exec.Command("go", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GOWORK=off")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("repair examples: go %v: %v\n%s", args, err, output)
		}
	}
}
