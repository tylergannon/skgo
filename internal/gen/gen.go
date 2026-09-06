// Package gen turns Go remote-function declarations into the files SvelteKit
// and Go each need: the `.remote.ts` module kit compiles, the TypeScript types
// its callers see, and the Go registration that answers the calls.
//
// The authoring model is the one in the brief. A developer writes an ordinary
// Go function in a `*.remote.go` file, colocated with the routes that use it,
// and marks it:
//
//	func getTodos(ctx context.Context, _ skgo.None) ([]Todo, error) { ... }
//
//	var _ = skgo.Query(getTodos)
//
// Nothing else is hand-written. `skgo generate` reads the marker with go/types
// — never by matching source text — takes the argument and result types out of
// the generic instantiation, and emits everything else.
package gen

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Config describes one app.
type Config struct {
	// Web is the vite root: the directory holding `src/` and the app's
	// package.json. Every `.remote.ts` path is relative to it, because that is
	// what kit hashes into a remote function's id.
	Web string
	// Out is the directory of the generated bindings package, normally
	// `<app>/generated`. It must be inside the same Go module as the remote
	// functions.
	Out string
	// Package is the name of the generated bindings package. It defaults to
	// the base name of Out.
	Package string
	// Logf receives one line per file written. It may be nil.
	Logf func(format string, args ...any)
}

// Run scans, generates, and reports what it wrote.
func Run(cfg Config) error {
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	web, err := filepath.Abs(cfg.Web)
	if err != nil {
		return err
	}
	out, err := filepath.Abs(cfg.Out)
	if err != nil {
		return err
	}
	cfg.Web, cfg.Out = web, out
	if cfg.Package == "" {
		cfg.Package = filepath.Base(out)
	}
	if fi, err := os.Stat(filepath.Join(web, "src")); err != nil || !fi.IsDir() {
		return fmt.Errorf("skgo: %s does not look like a vite root: no src/ directory", web)
	}

	files, err := findRemoteFiles(web)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("skgo: no *.remote.go files under %s", filepath.Join(web, "src"))
	}

	app, err := loadApp(cfg, files)
	if err != nil {
		return err
	}
	if len(app.remotes) == 0 {
		return fmt.Errorf("skgo: found %d *.remote.go file(s) but no skgo.Query, skgo.Command or skgo.LiveQuery declaration in any of them", len(files))
	}

	// Types first: the stubs import what polytype emits, so a type that
	// cannot be projected must stop generation before any stub is written.
	if err := app.generateTypes(); err != nil {
		return err
	}
	if err := app.writeStubs(); err != nil {
		return err
	}
	if err := app.writePackageBindings(); err != nil {
		return err
	}
	if err := app.writeAppBindings(); err != nil {
		return err
	}
	return app.writeRemoteList()
}

// findRemoteFiles collects every `*.remote.go` under `<web>/src`, skipping the
// directories neither Go nor a developer means to be scanned.
func findRemoteFiles(web string) ([]string, error) {
	var found []string
	src := filepath.Join(web, "src")
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path == src {
				return nil
			}
			if name == "node_modules" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".remote.go") || strings.HasSuffix(name, "_gen.go") {
			return nil
		}
		found = append(found, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(found)
	return found, nil
}

// webRel is the posix path of file relative to the vite root. Kit hashes this
// string, so it must use forward slashes on every platform.
func webRel(web, file string) (string, error) {
	rel, err := filepath.Rel(web, file)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("skgo: %s is outside the vite root %s", file, web)
	}
	return filepath.ToSlash(rel), nil
}
