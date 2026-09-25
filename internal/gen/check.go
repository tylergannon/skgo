package gen

import (
	"fmt"
	"os"
	"path/filepath"
)

// Check validates the source contracts generation enforces without writing
// authored files, route links, or generated bindings.
func Check(cfg Config) error {
	web, err := filepath.Abs(cfg.Web)
	if err != nil {
		return err
	}
	out, err := filepath.Abs(cfg.Out)
	if err != nil {
		return err
	}
	cfg.Web, cfg.Out, cfg.ReadOnly = web, out, true
	if cfg.Package == "" {
		cfg.Package = filepath.Base(out)
	}
	if fi, err := os.Stat(filepath.Join(web, "src")); err != nil || !fi.IsDir() {
		return fmt.Errorf("skgo: %s does not look like a vite root: no src/ directory", web)
	}
	if err := checkInstalledAdapter(cfg); err != nil {
		return err
	}
	files, err := findSourceFiles(web)
	if err != nil || len(files) == 0 {
		return err
	}
	a, err := loadApp(cfg, files)
	if err != nil {
		return err
	}
	if err := a.checkEndpointDuplicates(); err != nil {
		return err
	}
	if err := a.checkFileUsage(); err != nil {
		return err
	}
	if err := a.checkWireFields(); err != nil {
		return err
	}
	if err := a.checkPrerenderedLoads(); err != nil {
		return err
	}
	if err := a.declareLoadTypes(); err != nil {
		return err
	}
	for _, fn := range a.remotes {
		if fn.in != nil {
			if _, err := a.project(fn.in); err != nil {
				return fmt.Errorf("skgo: %s: argument of %s: %w", fn.pos, fn.name, err)
			}
		}
		if _, err := a.project(fn.out); err != nil {
			return fmt.Errorf("skgo: %s: result of %s: %w", fn.pos, fn.name, err)
		}
	}
	if err := a.links.verify(); err != nil {
		return err
	}
	if err := a.generateTypes(); err != nil {
		return err
	}
	if err := a.planCodecs(); err != nil {
		return err
	}
	if err := a.generateCodecs(); err != nil {
		return err
	}
	return nil
}
