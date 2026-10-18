---
title: "skgo/templateapi"
summary: "The development-time interface a plugin implements to add templates and add-ons."
---

What a plugin exports. A native Go plugin exports `SKGoPluginV1() Plugin`
and is built for the exact `skgo` host that invokes it; `skgo plugin install`
builds it and `skgo new --template` and `skgo add` run it. Plugins are
trusted local code and are never loaded by a generated application.

## Addon

```go
type Addon struct {
	Name, Summary string
	Options       []Option
}
```

## Descriptor

```go
type Descriptor struct {
	ID, Version, SkgoVersion string
	Addons                   []Addon
	Templates                []Template
}
```

## Option

```go
type Option struct {
	Name, Help, Default string
	Required            bool
}
```

## Plugin

```go
type Plugin interface {
	Describe() Descriptor
	Apply(context.Context, Request) (Result, error)
}
```

Plugin describes reusable additions and applies a single configured addition. Describe and package initialization must not modify projects or install tools.

## Project

```go
type Project struct {
	Root, Module, Name, Web string // Root and Web are absolute paths.
}
```

## Request

```go
type Request struct {
	Addon   string
	Project Project
	Options map[string]string // Defaults and user values resolved by skgo.
}
```

## Result

```go
type Result struct {
	Changed []string
	Next    []string
}
```

Result must include partial changes even when Apply returns an error. Changes are project-relative paths. Next contains instructions, never commands for skgo to automatically execute. Apply must honor cancellation, preserve user edits and reject incompatible existing files before writing.

## Step

```go
type Step struct {
	Addon    string
	Options  map[string]string // Literal add-on values.
	Bindings map[string]string // Add-on option name to template option name.
}
```

## Template

```go
type Template struct {
	Name, Summary string
	Options       []Option
	Steps         []Step
}
```

Template applies its steps to the ordinary minimal TypeScript skgo scaffold.
