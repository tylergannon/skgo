package gen

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

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
	pkg := declarationPackage(t, "fixture", "skgo_gen.go", merged, nil)
	for _, name := range []string{"Upper", "Number", "Shadow"} {
		if pkg.Types.Scope().Lookup(name) == nil {
			t.Fatalf("merge lost function %s:\n%s", name, merged)
		}
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
