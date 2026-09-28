package gen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/envspec"
)

func TestEnvironmentGenerationUsesDeclaredGoTypes(t *testing.T) {
	t.Parallel()
	root, cfg := foreignFixture(t, `package data`, nil)
	stub := filepath.Join(root, "skgo", "env.go")
	if err := os.WriteFile(stub, []byte("package skgo\nfunc Environment[T any]() Marker { return Marker{} }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	source := `package appenv
import s "github.com/tylergannon/skgo"
type Config struct {
 ID string ` + "`env:\"ID,public\"`" + `
 Enabled bool ` + "`env:\"ENABLED,public\"`" + `
 Release string ` + "`env:\"RELEASE,static\"`" + `
 Count *int ` + "`env:\"COUNT\"`" + `
}
var _ = s.Environment[Config]()
`
	if err := os.WriteFile(filepath.Join(cfg.Web, "src", "env.go"), []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	var schema envspec.Schema
	if err := json.Unmarshal([]byte(readFixtureFile(t, root, "app/web/skgo.env.json")), &schema); err != nil {
		t.Fatal(err)
	}
	if len(schema.Fields) != 4 || schema.Fields[0].Type != "string" || schema.Fields[1].Type != "bool" || !schema.Fields[2].Static || !schema.Fields[3].Optional {
		t.Fatalf("wrong declaration: %#v", schema)
	}
	sourceTS := readFixtureFile(t, root, "app/web/src/env.ts")
	for _, want := range []string{"defineEnvVars", `: boolean =>`, `: number | undefined =>`, `static: true`} {
		if !strings.Contains(sourceTS, want) {
			t.Errorf("generated declaration missing %s", want)
		}
	}
	if strings.Contains(sourceTS, "process.env") {
		t.Fatal("generation inferred environment from ambient process")
	}
	if err := Check(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Language = LanguageJavaScript
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	js := readFixtureFile(t, root, "app/web/src/env.js")
	if strings.Contains(js, " as ") || strings.Contains(js, "_value:") {
		t.Fatalf("TypeScript leaked into JS declaration: %s", js)
	}
	if _, err := os.Stat(filepath.Join(cfg.Web, "src", "env.ts")); !os.IsNotExist(err) {
		t.Fatal("language switch retained env.ts")
	}
	if err := os.Remove(filepath.Join(cfg.Web, "src", "env.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(cfg.Web, "src", "data", "data.remote.go")); err != nil {
		t.Fatal(err)
	}
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cfg.Web, "skgo.env.json")); !os.IsNotExist(err) {
		t.Fatal("deleted declaration retained environment schema")
	}

}
