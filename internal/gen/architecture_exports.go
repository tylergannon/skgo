//go:build skgo_architecture_experiment

package gen

import "golang.org/x/tools/go/packages"

// ArchitectureLoadExports exposes the existing loader only to an explicitly
// tagged architecture experiment. It is not a production API addition.
func ArchitectureLoadExports(cfg *packages.Config, patterns ...string) ([]*packages.Package, error) {
	cleanup, err := preserveDependencyExports(cfg)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return packages.Load(cfg, patterns...)
}
