// Package templateapi is the development-time interface for skgo application
// add-ons and templates. Native Go plugins export SKGoPluginV1() Plugin and must
// be built for the exact invoking skgo host. Plugins are trusted local code;
// they are never loaded by generated applications.
package templateapi

import "context"

// Plugin describes reusable additions and applies a single configured addition.
// Describe and package initialization must not modify projects or install tools.
type Plugin interface {
	Describe() Descriptor
	Apply(context.Context, Request) (Result, error)
}

type Descriptor struct {
	ID, Version, SkgoVersion string
	Addons                   []Addon
	Templates                []Template
}

type Option struct {
	Name, Help, Default string
	Required            bool
}

type Addon struct {
	Name, Summary string
	Options       []Option
}

// Template applies its steps to the ordinary minimal TypeScript skgo scaffold.
type Template struct {
	Name, Summary string
	Options       []Option
	Steps         []Step
}

type Step struct {
	Addon    string
	Options  map[string]string // Literal add-on values.
	Bindings map[string]string // Add-on option name to template option name.
}

type Project struct {
	Root, Module, Name, Web string // Root and Web are absolute paths.
}

type Request struct {
	Addon   string
	Project Project
	Options map[string]string // Defaults and user values resolved by skgo.
}

// Result must include partial changes even when Apply returns an error. Changes
// are project-relative paths. Next contains instructions, never commands for
// skgo to automatically execute. Apply must honor cancellation, preserve user
// edits and reject incompatible existing files before writing.
type Result struct {
	Changed []string
	Next    []string
}
