// Package generate exposes SKGO's application generator to Go build tools.
//
// Run and Check use the same implementation as the skgo CLI. Embedding callers
// can hand off immutable go/packages bundles with NewPackageSnapshot and
// Config.Packages. Reuse is checked against the current source view at every
// phase; generated overlays, edited source, build tags, and load configuration
// can require a fresh load. Use Config.LoadPackages to observe or customize
// those remaining loads, while honoring the requested configuration.
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
func NewPackageSnapshot(config *packages.Config, roots ...*packages.Package) (*PackageSnapshot, error) {
	return gen.NewPackageSnapshot(config, roots...)
}
