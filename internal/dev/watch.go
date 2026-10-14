package dev

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Authored inputs are what a developer writes: Go source, the module files, and
// the shape of the route tree. Kit watches the same kind of thing in
// exports/vite/dev/index.js (routes, assets, params, remote modules) and
// rebuilds its manifest when a file is added, removed or changed there.
//
// What the generator and the compiler write is deliberately not an input. A
// watcher that counted its own output would regenerate forever.
type inputs struct {
	root, web, out string
	extra          []string
	roots, files   []string
}

// fingerprint summarises every authored input. Two equal fingerprints mean
// nothing a build depends on has changed.
func (in inputs) fingerprint() (string, error) {
	scan := &digestScan{}
	return in.fingerprintScan(scan)
}

func (in inputs) fingerprintScan(scan *digestScan) (string, error) {
	var lines []string
	srcDir := filepath.Join(in.web, "src")
	skipped := map[string]bool{in.out: true, filepath.Join(in.web, "build"): true}
	err := filepath.WalkDir(in.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path != in.root && (skipped[path] || name == "node_modules" || strings.HasPrefix(name, ".") || name == "test-results" || name == "playwright-report") {
				return filepath.SkipDir
			}
			if path != in.root && !withinDir(srcDir, path) {
				control := filepath.Join(path, "go.mod")
				if _, err := os.Stat(control); err == nil {
					// A nested module is independent unless the generator selected it
					// as a mutable dependency root. Preserve the boundary itself.
					if err := in.stat(&lines, control, scan); err != nil {
						return err
					}
					return filepath.SkipDir
				}
			}
			return nil
		}
		rel, _ := filepath.Rel(in.root, path)
		switch {
		case strings.HasSuffix(name, "_gen.go") || strings.HasSuffix(name, "_test.go"):
			return nil
		case withinDir(srcDir, path) && name == "go.mod":
			// The route tree's module boundary is written by the generator.
			return nil
		case strings.HasSuffix(name, ".go") || name == "go.mod" || name == "go.sum":
			return in.stat(&lines, rel, scan)
		case withinDir(srcDir, path) && isRouteFile(name):
			if routeModule(name) {
				return in.stat(&lines, rel, scan)
			}
			// A component's contents are Vite's to hot-update. Only its
			// presence changes what routes exist.
			lines = append(lines, rel)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	for _, root := range in.roots {
		other := inputs{root: root, web: in.web, out: in.out}
		sum, err := other.fingerprintScan(scan)
		if err != nil {
			return "", err
		}
		lines = append(lines, "module "+root+" "+sum)
	}
	extra, err := fingerprintPathsScan(in.root, append(append([]string(nil), in.extra...), in.files...), scan)
	if err != nil {
		return "", err
	}
	lines = append(lines, extra)
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:]), nil
}

func (in inputs) stat(lines *[]string, rel string, scan *digestScan) error {
	path := rel
	if !filepath.IsAbs(path) {
		path = filepath.Join(in.root, path)
	}
	sum, err := scan.sum(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	*lines = append(*lines, rel+" "+sum)
	return nil
}

func withinDir(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// isRouteFile is a file kit gives meaning to by its name. Server modules
// (`+page.server.ts`, `+server.ts`) are left out: beside a Go file they are the
// stub the generator wrote.
func isRouteFile(name string) bool {
	if !strings.HasPrefix(name, "+") {
		return false
	}
	return strings.HasSuffix(name, ".svelte") || routeModule(name)
}

// routeModule is a universal module whose own exports (prerender, ssr, csr,
// trailingSlash) change how the generator and the server treat a route.
func routeModule(name string) bool {
	for _, kind := range []string{"+page", "+layout"} {
		if name == kind+".ts" || name == kind+".js" {
			return true
		}
	}
	return false
}

// Explicit extra inputs use contents, including equal-size, preserved-mtime
// writes. Missing files are represented so their later creation is noticed.
func fingerprintPaths(root string, paths []string) (string, error) {
	return fingerprintPathsScan(root, paths, &digestScan{})
}

func fingerprintPathsScan(root string, paths []string, scan *digestScan) (string, error) {
	var lines []string
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		matches := []string{path}
		if strings.ContainsAny(path, "*?[") {
			var err error
			matches, err = filepath.Glob(path)
			if err != nil {
				return "", err
			}
			lines = append(lines, "glob "+path)
		}
		for _, match := range matches {
			err := filepath.WalkDir(match, func(name string, d fs.DirEntry, err error) error {
				if os.IsNotExist(err) {
					lines = append(lines, "missing "+name)
					return nil
				}
				if err != nil {
					return err
				}
				if d.IsDir() {
					return nil
				}
				sum, err := scan.sum(name)
				if err != nil {
					return err
				}
				lines = append(lines, name+" "+sum)
				return nil
			})
			if err != nil {
				return "", err
			}
		}
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:]), nil
}
