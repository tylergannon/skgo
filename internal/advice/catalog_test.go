package advice

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
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
	for i, entry := range Catalog() {
		dir := filepath.Join(root, fmt.Sprintf("rule%03d", i+1))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		source := "package repair\n\nimport (\"context\"; skgo \"github.com/tylergannon/skgo\")\n\n" + entry.Example + "\n"
		if err := os.WriteFile(filepath.Join(dir, "repair.go"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"mod", "tidy"}, {"test", "./..."}} {
		cmd := exec.Command("go", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GOWORK=off")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("repair examples: go %v: %v\n%s", args, err, output)
		}
	}
}
