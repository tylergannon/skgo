---
title: "skgo/nativeapp"
summary: "The native application configuration shared by the add-on, plugins and skgo native build."
---

The configuration a macOS or iOS application is built from: the presets,
the package identity and the view the shell presents. The built-in
`native-app` add-on writes it, application plugins extend it, and
`skgo native build` reads it.

## Config

```go
type Config struct {
	SwiftRemotes      []string          `json:"swiftRemotes,omitempty"`
	DeploymentTargets map[string]string `json:"deploymentTargets,omitempty"`
	Version           int               `json:"version"`
	BundleID          string            `json:"bundleId"`
	Packages          []Package         `json:"packages,omitempty"`
	Sources           []string          `json:"sources,omitempty"`
	Resources         []string          `json:"resources,omitempty"`
	Info              map[string]any    `json:"info,omitempty"`
	IOSInfo           map[string]any    `json:"iosInfo,omitempty"`
	MacOSInfo         map[string]any    `json:"macosInfo,omitempty"`
	Presets           map[string]Preset `json:"presets,omitempty"`
}
```

Config is stored at native/skgo-native.json. Paths are project-root relative. The build writes only disposable artifacts under native/build.

### Validate

```go
func (c Config) Validate() error
```

Validate checks identity and contribution paths before a build changes files.

## DefaultApplicationView

```go
var DefaultApplicationView string
```

DefaultApplicationView is the exact replaceable application-owned Swift slot. An add-on must compare all bytes before replacing it, preserving user edits.

## Package

```go
type Package struct {
	Name     string   `json:"name"`
	Path     string   `json:"path"`
	Products []string `json:"products"`
}
```

## Plugin

```go
func Plugin(version string) templateapi.Plugin
```

## Preset

```go
type Preset struct {
	GoTags       []string       `json:"goTags,omitempty"`
	BundleSuffix string         `json:"bundleSuffix,omitempty"`
	Info         map[string]any `json:"info,omitempty"`
	Resources    []string       `json:"resources"`
}
```

Preset selects an output/bundle suffix and optional Go tags, Info.plist values, and resources. Swift packages remain linked for every preset. An omitted Resources slice inherits Config.Resources; an explicit empty slice clears it.
