package gen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testRealKitBuildAndRuntimeShareApplicationHooks(t *testing.T) {
	fixture := requireOptionsFixture(t)
	read := func(name string) string {
		t.Helper()
		body, err := os.ReadFile(filepath.Join(fixture.app, "ui/build/prerendered", name))
		if err != nil {
			t.Fatalf("%v; build output:\n%s", err, fixture.buildOutput)
		}
		return string(body)
	}
	body := read("shared-fetch.html")
	for _, want := range []string{"<h1>Shared configuration</h1>", "Go:literal-hook-content-278|literal-api-278|literal-hook-header-278", "Kit:literal-hook-content-278|literal-api-278|literal-hook-header-278|default-literal", "Boundary:/shared-fetch", `"x-config-hook":"literal-hook-header-278"`, `"x-default":"default-literal"`} {
		if !strings.Contains(body, want) {
			t.Errorf("built fetch page lacks %q: %s", want, body)
		}
	}
	if strings.Contains(body, "literal-private-header-275") {
		t.Error("private header escaped")
	}
	body = read("shared-error.html")
	if !strings.Contains(body, "500|literal-safe-error-278|literal-support-278|11") || strings.Contains(body, "literal-private-error-278") {
		t.Fatalf("build error shape: %s", body)
	}
	body = read("shared-component.html")
	if !strings.Contains(body, "<h1>Shared component policy</h1>") || !strings.Contains(body, "200|literal-safe-error-278|literal-support-278|11") || strings.Contains(body, "literal-private-error-278") {
		t.Fatalf("fresh Kit error public body: %s", body)
	}
	body = read("options.html")
	for _, want := range []string{">Resolve options</h1>", "<p>inner-outer</p>", `"x-public":"public-literal"`, `"x-late":"late-literal"`} {
		if !strings.Contains(body, want) {
			t.Errorf("request override lacks %q: %s", want, body)
		}
	}
	if strings.Contains(body, `"x-default"`) || strings.Contains(body, `"x-denied"`) {
		t.Errorf("request filter did not replace default: %s", body)
	}
	for _, want := range []string{"Order #42"} {
		if body := read("typed-load/00042.html"); !strings.Contains(body, want) {
			t.Errorf("selected matcher lost: %s", body)
		}
	}
	body = read("lifecycle/alpha.html")
	for _, want := range []string{"address unavailable", "Go after hook: /lifecycle/alpha cookies:scoped%20raw deleted cookie-forwarding:root", "/lifecycle/alpha:remote-13 cookies:scoped%20raw deleted"} {
		if !strings.Contains(body, want) {
			t.Errorf("generated request facilities lost %q: %s", want, body)
		}
	}
	fixture.run(t, "TestSharedApplicationConfiguration")
}

func TestApplicationConfigSelectionIsExplicitAndTyped(t *testing.T) {
	t.Parallel()
	imports := declarationImports{skgoPkg: declarationPackage(t, skgoPkg, "config.go", "package skgo;type ApplicationConfig struct{}", nil).Types}
	for _, tc := range []struct{ name, source, symbol, want string }{
		{"selected", "var Policies skgo.ApplicationConfig", "Policies", ""},
		{"wrong-type", "var Config = 1", "Config", "must have type skgo.ApplicationConfig"},
		{"function", "func Config()skgo.ApplicationConfig{return skgo.ApplicationConfig{}}", "Config", "exported ApplicationConfig value"},
		{"missing", "var Other skgo.ApplicationConfig", "Config", "exported ApplicationConfig value"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pkg := declarationPackage(t, "app/policy", "config.go", "package policy;import \"github.com/tylergannon/skgo\";var _ skgo.ApplicationConfig;"+tc.source, imports)
			err := validateApplicationConfigSymbol(pkg.Types, "app/policy", tc.symbol)
			if tc.want == "" && err != nil {
				t.Fatal(err)
			}
			if tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("wanted %q, got %v", tc.want, err)
			}
		})
	}
	dir := t.TempDir()
	writeSharedFixture(t, dir, "config.go", "package bindings\n//go:generate go tool skgo generate --config-package=app/policy --config-symbol Policies\n")
	cfg, err := DeclaredConfig(Config{Out: dir})
	if err != nil || cfg.ConfigPackage != "app/policy" || cfg.ConfigSymbol != "Policies" {
		t.Fatalf("selection: %+v %v", cfg, err)
	}
	writeSharedFixture(t, dir, "config.go", "package bindings\n//go:generate go tool skgo generate --config-package first --config-package second\n")
	if _, err := DeclaredConfig(Config{Out: dir}); err == nil || !strings.Contains(err.Error(), "multiple configured selections") {
		t.Fatalf("duplicate selection: %v", err)
	}
}
