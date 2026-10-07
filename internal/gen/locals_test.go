package gen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalsConfigurationRejectsInvalidOwnershipAndTypes(t *testing.T) {
	for _, tc := range []struct{ name, source, want string }{
		{"missing-config", "", "locals package is required"},
		{"missing-type", "package app\ntype Different struct{}", "locals type"},
		{"non-struct", "package app\ntype Locals string", "concrete named struct"},
		{"alias", "package app\ntype Other struct{}\ntype Locals = Other", "concrete named struct"},
		{"generic", "package app\ntype Locals[T any] struct{Value T}", "concrete named struct"},
		{"variable", "package app\ntype State struct{}\nvar Locals State", "must name a type"},
		{"params-cycle", "package app\nimport \"example.com/app/generated/params\"\ntype Locals struct{P params.Params}", "loadable without generated routing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, cfg := foreignFixture(t, `package data
import("context";"github.com/tylergannon/skgo")
func query(context.Context)(string,error){return "fixture",nil}
var _=skgo.Query(query)
`, nil)
			host, _, err := moduleOf(cfg.Out)
			if err != nil {
				t.Fatal(err)
			}
			if tc.name == "missing-config" {
				cfg.LocalsPackage = ""
			} else {
				if err := os.WriteFile(filepath.Join(host, "internal/app/locals.go"), []byte(tc.source), 0644); err != nil {
					t.Fatal(err)
				}
			}
			for _, check := range []func(Config) error{Run, Check} {
				if err := check(cfg); err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("wanted %q, got %v", tc.want, err)
				}
			}
		})
	}
}

func TestSelectedHookConfigurationAndAliases(t *testing.T) {
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
		{"bindings-dependency", `package hooks
import _ "example.com/app/generated"
func Handle(){}
`, "selected hook"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, cfg := foreignFixture(t, `package data
import("context";"github.com/tylergannon/skgo")
func query(context.Context)(string,error){return "fixture",nil}
var _=skgo.Query(query)
`, nil)
			host, _, err := moduleOf(cfg.Out)
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(host, "hooks")
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "handle.go"), []byte(tc.source), 0644); err != nil {
				t.Fatal(err)
			}
			cfg.HookPackage = "example.com/app/hooks"
			err = Run(cfg)
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("wanted %q, got %v", tc.want, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := Check(cfg); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDeclaredLocalsAndSingleHookSelection(t *testing.T) {
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

func TestRootLocalsRegenerationPreservesOtherApplicationOutput(t *testing.T) {
	_, cfg := foreignFixture(t, `package data
import("context";"github.com/tylergannon/skgo")
func query(context.Context)(string,error){return "fixture",nil}
var _=skgo.Query(query)
`, nil)
	host, mod, err := moduleOf(cfg.Out)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(host, "locals.go"), []byte("package app\ntype Locals struct{}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	sibling := filepath.Join(host, "other-app", generatedGoFile)
	if err := os.MkdirAll(filepath.Dir(sibling), 0755); err != nil {
		t.Fatal(err)
	}
	receipt := goHeader + "package other\nconst OwnedByAnotherApplication = 42\n"
	if err := os.WriteFile(sibling, []byte(receipt), 0644); err != nil {
		t.Fatal(err)
	}
	cfg.LocalsPackage = mod
	for range 2 {
		if err := Run(cfg); err != nil {
			t.Fatal(err)
		}
	}
	if err := Check(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(sibling)
	if err != nil || string(got) != receipt {
		t.Fatalf("unrelated output changed: %q %v", got, err)
	}
}
