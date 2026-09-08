// Package gen turns Go remote-function declarations into the files SvelteKit
// and Go each need: the `.remote.ts` module kit compiles, the TypeScript types
// its callers see, and the Go registration that answers the calls.
//
// The authoring model is the one in the brief. A developer writes an ordinary
// Go function in a `*.remote.go` file, colocated with the routes that use it,
// and marks it:
//
//	func getTodos(ctx context.Context) ([]Todo, error) { ... }
//
//	var _ = skgo.Query(getTodos)
//
// Nothing else is hand-written. `skgo generate` reads the marker with go/types
// — never by matching source text — takes the argument and result types out of
// the marked function's own signature, and emits everything else. The argument
// is optional there because it is optional in kit: `query(fn)` accepts
// `(arg?) => Output`, so a query that takes nothing is written taking nothing.
package gen

import (
	"fmt"
	"go/parser"
	"go/token"
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
func Run(cfg Config) (err error) {
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

	// The adapter kit builds the frontend with is an ordinary npm package the
	// app installed, and it is the same contract as the stubs and the remote
	// list below: what the adapter writes, this module's Go reads. Generating
	// against one this module cannot read the output of is the mistake worth
	// catching before a full frontend build rather than after it.
	if err := checkInstalledAdapter(cfg); err != nil {
		return err
	}

	files, err := findSourceFiles(web)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("skgo: no *.remote.go, page.server.go, layout.server.go, server.go or hooks.go files under %s", filepath.Join(web, "src"))
	}

	// A skgo_remotes_gen.go left by an older skgo can call a runtime symbol
	// this version renamed or removed (#91): loadApp demands its whole
	// package compile, and polytype, loading the same package again to
	// project its wire types before any stub is written, offers no way to
	// tolerate an error in one file and go on. writePackageBindings
	// overwrites every one of these files later in this same run regardless
	// of what it finds them holding now, so blanking them first, before
	// anything loads the package they are in, loses nothing a developer
	// wrote — only what a previous run of this same generator wrote, which
	// this run is already committed to replacing. The restore func below
	// undoes it if this run fails before reaching that point, so a failed
	// run leaves the tree exactly as it found it.
	restore, err := resetStaleGeneratedFiles(files)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			restore()
		}
	}()

	app, err := loadApp(cfg, files)
	if err != nil {
		return err
	}
	if len(app.remotes) == 0 && len(app.loads) == 0 && len(app.endpoints) == 0 && len(app.transported) == 0 {
		return fmt.Errorf("skgo: found %d source file(s) but no skgo.Query, skgo.Command, skgo.LiveQuery, skgo.Load or skgo.GET declaration in any of them", len(files))
	}
	if err := app.checkEndpointDuplicates(); err != nil {
		return err
	}

	if err := app.checkFileUsage(); err != nil {
		return err
	}
	if err := app.checkPrerenderedLoads(); err != nil {
		return err
	}

	// Types first: the stubs import what polytype emits, so a type that
	// cannot be projected must stop generation before any stub is written.
	if err := app.declareLoadTypes(); err != nil {
		return err
	}
	if err := app.generateTypes(); err != nil {
		return err
	}
	if err := app.writeStubs(); err != nil {
		return err
	}
	if err := app.writeLoadStubs(); err != nil {
		return err
	}
	if err := app.writeEndpointStubs(); err != nil {
		return err
	}
	app.planNames()
	if err := app.writePackageBindings(); err != nil {
		return err
	}
	// The per-package registration files are new Go source in authored route
	// directories; the route root reaches Go only through per-file links, so
	// the tree has to take one more pass before anything can compile.
	if err := app.links.sync(); err != nil {
		return err
	}
	// Codecs after the package bindings, and for the same reason the bindings
	// come after the links: polytype loads each package that names a type on
	// the wire, and what it loads is the tree as it now stands.
	if err := app.planCodecs(); err != nil {
		return err
	}
	if err := app.generateCodecs(); err != nil {
		return err
	}
	if err := app.writeAppBindings(); err != nil {
		return err
	}
	return app.writeRemoteList()
}

// generatedFileSuffix marks a file this module writes and overwrites on every
// run — skgo_remotes_gen.go beside the authored source, plus this package's
// own skgo_bindings_gen.go and skgo_devalue_gen.go. findSourceFiles uses it to
// skip generated files when choosing what to scan for markers.
const generatedFileSuffix = "_gen.go"

// generatedRemotesFileName is the one generated file writePackageBindings
// puts in every authored route directory. It is the file a stale skgo
// upgrade leaves calling a runtime symbol that no longer exists (#91): see
// resetStaleGeneratedFiles.
const generatedRemotesFileName = "skgo_remotes_gen.go"

// resetStaleGeneratedFiles blanks every generatedRemotesFileName beside a
// wanted source file, before loadApp or polytype gets a chance to load the
// package it is in.
//
// A stale one — left by an older skgo whose runtime API it calls this
// version renamed or removed — would otherwise stop the very run that would
// rewrite it: every package load this run performs, this generator's own and
// polytype's, demands the whole package compile, with no way to tolerate an
// error confined to one file it is about to overwrite anyway. The
// placeholder keeps the file's own package clause — read from another file in
// the same directory, since the stale file's own may be the very thing
// broken — and declares nothing else, which always compiles.
//
// The returned func restores every file this touched to what it held before,
// for Run to call if it returns before writePackageBindings replaces the
// placeholder with real content.
func resetStaleGeneratedFiles(files []string) (restore func(), err error) {
	dirs := map[string]bool{}
	for _, f := range files {
		dirs[filepath.Dir(f)] = true
	}

	type saved struct {
		path    string
		content []byte
		mode    os.FileMode
	}
	var touched []saved
	restore = func() {
		for _, s := range touched {
			_ = os.WriteFile(s.path, s.content, s.mode)
		}
	}

	for dir := range dirs {
		path := filepath.Join(dir, generatedRemotesFileName)
		info, statErr := os.Stat(path)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				continue // a package this generator has never written to yet
			}
			restore()
			return nil, statErr
		}
		original, readErr := os.ReadFile(path)
		if readErr != nil {
			restore()
			return nil, readErr
		}
		pkgName, nameErr := packageNameOf(dir)
		if nameErr != nil {
			restore()
			return nil, nameErr
		}
		placeholder := goHeader + "package " + pkgName + "\n"
		if writeErr := os.WriteFile(path, []byte(placeholder), info.Mode()); writeErr != nil {
			restore()
			return nil, writeErr
		}
		touched = append(touched, saved{path: path, content: original, mode: info.Mode()})
	}
	return restore, nil
}

// packageNameOf reads the package clause of a directory's Go source, off
// whichever file states it plainly. It parses the package clause alone,
// never the rest of the file, so a file whose body cannot type-check — the
// very case this exists to work around — still answers.
func packageNameOf(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, generatedFileSuffix) || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.PackageClauseOnly)
		if err != nil {
			continue
		}
		return f.Name.Name, nil
	}
	return "", fmt.Errorf("skgo: no source file in %s states its own package name", dir)
}

// loadFileNames are the two file names that carry a server load. A route
// directory holds one `+page.server.ts` and one `+layout.server.ts` at most, so
// the Go file that generates each is named for it — minus the `+`, which Go
// refuses in a file name.
var loadFileNames = map[string]string{
	"page.server.go":   "+page.server.ts",
	"layout.server.go": "+layout.server.ts",
}

// hooksFileName is the app's universal hooks, beside kit's own `src/hooks.ts`.
// Kit's `transport` is a universal hook — the browser needs `decode` and the
// server needs `encode` — so the Go half is declared where the TypeScript half
// is, and one file per app is all kit allows.
const hooksFileName = "hooks.go"

// findSourceFiles collects every `*.remote.go`, `page.server.go`,
// `layout.server.go` and `server.go` under `<web>/src`, skipping the
// directories neither Go nor a developer means to be scanned.
func findSourceFiles(web string) ([]string, error) {
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
		if strings.HasSuffix(name, generatedFileSuffix) {
			return nil
		}
		_, isLoad := loadFileNames[name]
		if !isLoad && name != serverFileName && name != hooksFileName && !strings.HasSuffix(name, ".remote.go") {
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
