---
title: "skgo/generate"
summary: "The application generator as a Go package, for build tools that embed it."
---

`skgo generate` as a library. A build tool that already loads the
application's packages can run the same generation in process, share its
`go/packages` bundle, and keep the persistent plan cache the command uses.
Most applications never import this package; the command and the
`go:generate` directive in the bindings package are enough.

## Check

```go
func Check(cfg Config) error
```

Check validates an application without publishing generated files.

## Config

```go
type Config = gen.Config
```

### DeclaredConfig

```go
func DeclaredConfig(cfg Config) (Config, error)
```

DeclaredConfig reads generation selections from the application's go:generate declaration.

## Language

```go
type Language = gen.Language
```

## LanguageAuto, LanguageTypeScript, LanguageJavaScript

```go
const (
	LanguageAuto       = gen.LanguageAuto
	LanguageTypeScript = gen.LanguageTypeScript
	LanguageJavaScript = gen.LanguageJavaScript
)
```

## PackageLoader

```go
type PackageLoader = gen.PackageLoader
```

## PackageSnapshot

```go
type PackageSnapshot = gen.PackageSnapshot
```

### NewPackageSnapshot

```go
func NewPackageSnapshot(config *packages.Config, roots ...*packages.Package) (*PackageSnapshot, error)
```

NewPackageSnapshot records an immutable, freshly loaded package bundle. config must be the exact configuration that produced packages, and source files must not change between loading and capture. Imports are included. Plain domain packages are useful candidates. Packages containing generated Go usually differ from the current generation phase's overlay and fall back to normal loading. Installed toolchain and module-cache source must remain unchanged for the invocation; mutable application source is checked.

## Prepared

```go
type Prepared = gen.Prepared
```

### Prepare

```go
func Prepare(cfg Config) (*Prepared, error)
```

Prepare analyzes the app once so embedding tools may inspect Packages before publishing with Prepared.Run. Always call Prepared.Close. Source files must remain unchanged until Run returns.

## Run

```go
func Run(cfg Config) error
```

Run generates an application's Go bindings and frontend modules.
