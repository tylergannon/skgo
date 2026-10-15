package devserver

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplicationUsesExistingGenerationDeclaration(t *testing.T) {
	root := t.TempDir()
	bindings := filepath.Join(root, "internal/skgo")
	os.MkdirAll(bindings, 0755)
	os.MkdirAll(filepath.Join(root, "web"), 0755)
	os.WriteFile(filepath.Join(root, "web/package.json"), []byte(`{}`), 0644)
	os.WriteFile(filepath.Join(bindings, "config.go"), []byte("package skgo\n//go:generate go tool skgo generate --web ../../web --locals-package example.com/store/internal/app --locals-type Session --hook-package example.com/store/internal/hooks\n"), 0644)
	cfg, e := applicationConfig(Options{Root: root})
	if e != nil {
		t.Fatal(e)
	}
	if cfg.LocalsPackage != "example.com/store/internal/app" || cfg.LocalsType != "Session" || cfg.HookPackage != "example.com/store/internal/hooks" || cfg.Out != bindings {
		t.Fatalf("declared app selection lost: %+v", cfg)
	}
}
