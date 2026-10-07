package gen

import (
	"os"
	"path/filepath"
)

// Every generation fixture declares its application-owned locals explicitly.
// The copied example already owns its domain type; miniature fixtures own an
// empty named struct. This is test setup, never generator type inference.
func fixtureConfig(cfg Config) Config {
	if cfg.Out == "" || cfg.LocalsPackage != "" {
		return cfg
	}
	host, mod, err := moduleOf(cfg.Out)
	if err != nil {
		return cfg
	}
	cfg.LocalsPackage = mod + "/internal/app"
	cfg.LocalsType = "Locals"
	dir := filepath.Join(host, "internal", "app")
	path := filepath.Join(dir, "locals.go")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			panic(err)
		}
		if err := os.WriteFile(path, []byte("package app\ntype Locals struct{}\n"), 0644); err != nil {
			panic(err)
		}
	}
	return cfg
}
