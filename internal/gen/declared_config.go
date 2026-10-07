package gen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DeclaredConfig reads the application's explicit generation selection. Dev
// and check share that declaration instead of inferring an application type.
func DeclaredConfig(cfg Config) (Config, error) {
	seen := map[string]bool{}
	entries, _ := os.ReadDir(cfg.Out)
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_gen.go") {
			continue
		}
		source, err := os.ReadFile(filepath.Join(cfg.Out, entry.Name()))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(source), "\n") {
			if !strings.HasPrefix(line, "//go:generate ") {
				continue
			}
			args := strings.Fields(strings.TrimPrefix(line, "//go:generate "))
			generate := false
			for _, arg := range args {
				generate = generate || arg == "generate"
			}
			if !generate {
				continue
			}
			for i := 0; i < len(args); i++ {
				name, value, inline := strings.Cut(strings.TrimLeft(args[i], "-"), "=")
				switch name {
				case "locals-package", "locals-type", "hook-package", "hook-symbol":
					if seen[name] {
						return cfg, fmt.Errorf("skgo: multiple configured selections for --%s", name)
					}
					seen[name] = true
					if !inline {
						if i+1 == len(args) {
							return cfg, fmt.Errorf("skgo: missing value for --%s", name)
						}
						i++
						value = args[i]
					}
					switch name {
					case "locals-package":
						cfg.LocalsPackage = value
					case "locals-type":
						cfg.LocalsType = value
					case "hook-package":
						cfg.HookPackage = value
					case "hook-symbol":
						cfg.HookSymbol = value
					}
				}
			}
		}
	}
	return cfg, nil
}
