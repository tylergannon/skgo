// Package generate exposes SKGO's application generator to Go build tools.
//
// Run and Check use the same implementation as the skgo CLI. Embedding callers
// can hand off immutable go/packages bundles with NewPackageSnapshot and
// Config.Packages. Plain domain packages can reuse their type projections.
// Disk-loaded packages containing skgo_gen.go generally differ from the
// generator's intermediate overlays and must be loaded again. Use Prepare
// to share its route/remote discovery graph with the caller's own analysis.
//
// Reuse conservatively requires matching source, flags, and load configuration.
// Polytype projections request BuildFlags: []string{"-tags=jsonschema"}.
// Config.OnPackageLoad observes fallback patterns and configuration without
// changing loading; it does not report mismatch reasons. Config.LoadPackages
// replaces the loader and must honor the requested configuration. Preloading
// solely for generation adds work; total generation time need not improve.
package generate

import (
	"github.com/tylergannon/skgo/internal/gen"
	"golang.org/x/tools/go/packages"
)

type Prepared = gen.Prepared

// Prepare analyzes the app once so embedding tools may inspect Packages before
// publishing with Prepared.Run. Always call Prepared.Close. Source files must
// remain unchanged until Run returns.
func Prepare(cfg Config) (*Prepared, error) { return gen.Prepare(cfg) }

type Config = gen.Config
type Language = gen.Language
type PackageLoader = gen.PackageLoader
type PackageSnapshot = gen.PackageSnapshot

const (
	LanguageAuto       = gen.LanguageAuto
	LanguageTypeScript = gen.LanguageTypeScript
	LanguageJavaScript = gen.LanguageJavaScript
)

// Run generates an application's Go bindings and frontend modules.
func Run(cfg Config) error { return gen.Run(cfg) }

// Check validates an application without publishing generated files.
func Check(cfg Config) error { return gen.Check(cfg) }

// DeclaredConfig reads generation selections from the application's go:generate declaration.
func DeclaredConfig(cfg Config) (Config, error) { return gen.DeclaredConfig(cfg) }

// NewPackageSnapshot records an immutable, freshly loaded package bundle.
// config must be the exact configuration that produced packages, and source
// files must not change between loading and capture. Imports are included.
// Plain domain packages are useful candidates. Packages containing generated
// Go usually differ from the current generation phase's overlay and fall back
// to normal loading. Installed toolchain and module-cache source must remain
// unchanged for the invocation; mutable application source is checked.
func NewPackageSnapshot(config *packages.Config, roots ...*packages.Package) (*PackageSnapshot, error) {
	return gen.NewPackageSnapshot(config, roots...)
}
