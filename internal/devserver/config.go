package devserver

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/tylergannon/skgo/internal/gen"
)

// Options describes a local development session. Application selections come
// from its existing go:generate declaration, shared with production generation.
type Options struct {
	Root, Web, Out, Listen, GoTool, VitePlus, GeneratorConfig string
	StrictPort                                                bool
	Output                                                    io.Writer
}

func applicationConfig(o Options) (gen.Config, error) {
	root, e := filepath.Abs(o.Root)
	if e != nil {
		return gen.Config{}, e
	}
	web := o.Web
	if web == "" {
		web = "web"
	}
	if !filepath.IsAbs(web) {
		web = filepath.Join(root, web)
	}
	out := o.Out
	if out == "" {
		out = "internal/skgo"
		if s, e := os.Stat(filepath.Join(root, out)); e != nil || !s.IsDir() {
			out = "generated"
		}
	}
	if !filepath.IsAbs(out) {
		out = filepath.Join(root, out)
	}
	cfg, e := gen.DeclaredConfig(gen.Config{Web: web, Out: out, CacheDir: "off"})
	if e != nil {
		return cfg, e
	}
	if cfg.LocalsPackage == "" {
		return cfg, fmt.Errorf("skgo dev: no --locals-package in the generation declaration under %s; select the existing bindings with --out", out)
	}
	if _, e = os.Stat(filepath.Join(web, "package.json")); e != nil {
		return cfg, fmt.Errorf("skgo dev frontend: %w", e)
	}
	return cfg, nil
}
