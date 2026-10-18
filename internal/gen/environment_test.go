package gen

import (
	"go/ast"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvironmentGenerationUsesDeclaredGoTypes(t *testing.T) {
	t.Parallel()
	imports := declarationImports{skgoPkg: declarationPackage(t, skgoPkg, "env.go", "package skgo;func Environment[T any]()int{return 0}", nil).Types}
	const source = `package appenv
import s "github.com/tylergannon/skgo"
type Config struct {
 ID string ` + "`env:\"ID,public\"`" + `
 Enabled bool ` + "`env:\"ENABLED,public\"`" + `
 Release string ` + "`env:\"RELEASE,static\"`" + `
 Count *int ` + "`env:\"COUNT\"`" + `
}
var _ = s.Environment[Config]()
`
	web := t.TempDir()
	path := filepath.Join(web, "src/env.go")
	pkg := declarationPackage(t, "environment.test", path, source, imports)
	a := &app{cfg: Config{Web: web, Language: LanguageTypeScript, Logf: func(string, ...any) {}}}
	var call *ast.CallExpr
	ast.Inspect(pkg.Syntax[0], func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			call = c
		}
		return true
	})
	ident := call.Fun.(*ast.IndexExpr).X.(*ast.SelectorExpr).Sel
	if err := a.readEnvironment(pkg, call, ident, path); err != nil {
		t.Fatal(err)
	}
	fields := a.environment.Fields
	if len(fields) != 4 || fields[0].Name != "ID" || fields[0].Type != "string" || !fields[0].Public || fields[1].Type != "bool" || !fields[2].Static || !fields[3].Optional || fields[3].Type != "int" {
		t.Fatalf("declared schema: %+v", fields)
	}
	if err := a.writeEnvironment(); err != nil {
		t.Fatal(err)
	}
	ts, err := os.ReadFile(filepath.Join(web, "src/env.ts"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"defineEnvVars", `: boolean =>`, `: number | undefined =>`, `"RELEASE": { public: false, static: true`} {
		if !strings.Contains(string(ts), want) {
			t.Errorf("missing %q: %s", want, ts)
		}
	}
	a.cfg.Language = LanguageJavaScript
	if err := a.writeEnvironment(); err != nil {
		t.Fatal(err)
	}
	assertCacheFile(t, filepath.Join(web, "src/env.js"), "@returns {number | undefined}")
	if _, err := os.Stat(filepath.Join(web, "src/env.ts")); !os.IsNotExist(err) {
		t.Fatalf("obsolete declaration: %v", err)
	}
}
