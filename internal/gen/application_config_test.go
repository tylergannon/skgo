package gen

import (
	"strings"
	"testing"
)

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
