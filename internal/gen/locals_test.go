package gen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalsConfigurationRejectsInvalidOwnershipAndTypes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, source, want string }{

		{"valid", "package app\ntype Locals struct{ Receipt int }", ""},
		{"missing-type", "package app\ntype Different struct{}", "locals type"},
		{"non-struct", "package app\ntype Locals string", "concrete named struct"},
		{"alias", "package app\ntype Other struct{}\ntype Locals = Other", "concrete named struct"},
		{"generic", "package app\ntype Locals[T any] struct{Value T}", "concrete named struct"},
		{"variable", "package app\ntype State struct{}\nvar Locals State", "must name a type"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pkg := declarationPackage(t, "example.com/app/internal/app", "locals.go", tc.source, nil)
			err := validateLocalsType(pkg.Types, "Locals")
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("wanted %q: %v", tc.want, err)
			}

		})
	}
}

func TestSelectedHookConfigurationAndAliases(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, source, want string }{
		{"function", `package hooks
import("context";"net/http";"example.com/app/generated/params")
type Event = params.RequestEvent
type Resolve = params.Resolve
func Handle(ctx context.Context,event Event,resolve Resolve)(*http.Response,error){return resolve(ctx,event)}
`, ""},
		{"variable", `package hooks
import "example.com/app/generated/params"
var Handle params.Middleware
`, ""},
		{"missing", "package hooks\nvar Other = 1", "is missing"},
		{"type-instead-of-value", "package hooks\nimport \"example.com/app/generated/params\"\ntype Handle params.Middleware", "must be a function or variable"},
		{"wrong-signature", "package hooks\nfunc Handle(){}", "convertible to params.Middleware"},
		{"wrong-event-locals", `package hooks
import("context";"net/http";"github.com/tylergannon/skgo";"example.com/app/generated/params")
func Handle(ctx context.Context,event skgo.RequestEvent[params.Params,struct{}],resolve params.Resolve)(*http.Response,error){return nil,nil}
`, "convertible to params.Middleware"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			{
				_, _, route, _ := markerDeclaration(t, "fixture.go", `package route
 import("context";"github.com/tylergannon/skgo";"example.com/app/internal/app";"example.com/app/generated/params")
 var _=context.Background;var _=params.Params{};var _=app.Locals{}
 func target(context.Context,params.RequestEvent)(string,error){return "",nil}
 var _=skgo.Command(target)
 `)
				imports := declarationImports{}
				for _, dep := range route.Types.Imports() {
					imports[dep.Path()] = dep
				}
				pkg := declarationPackage(t, "example.com/app/hooks", "handle.go", tc.source, imports)
				middleware := imports["example.com/app/generated/params"].Scope().Lookup("Middleware").Type()
				err := validateHookSymbol(pkg.Types, middleware, "Handle")
				if tc.want == "" {
					if err != nil {
						t.Fatal(err)
					}
				} else if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("wanted %q: %v", tc.want, err)
				}
				return
			}
		})
	}
}

func TestDeclaredLocalsAndSingleHookSelection(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.go")
	if err := os.WriteFile(path, []byte("package binding\n//go:generate go tool skgo generate --locals-package=example.com/app/internal/app -hook-package example.com/app/hooks --hook-symbol=Serve\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := DeclaredConfig(Config{Out: dir})
	if err != nil || cfg.LocalsPackage != "example.com/app/internal/app" || cfg.HookPackage != "example.com/app/hooks" || cfg.HookSymbol != "Serve" {
		t.Fatalf("declared config: %+v %v", cfg, err)
	}
	if err := os.WriteFile(path, []byte("package binding\n//go:generate go tool skgo generate --hook-package first --hook-package second\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := DeclaredConfig(Config{Out: dir}); err == nil || !strings.Contains(err.Error(), "multiple configured selections") {
		t.Fatalf("duplicate declaration: %v", err)
	}
}
