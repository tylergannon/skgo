// Package templateapi is the development-time interface for skgo application
// add-ons, templates, generators and commands. Native Go plugins export
// SKGoPluginV1() Plugin and must
// be built for the exact invoking skgo host. Plugins are trusted local code;
// they are never loaded by generated applications.
//
// skgo plugin install builds a plugin from its module source for the
// skgo executable that loads it; skgo new --template applies its templates and
// skgo add applies its add-ons. Exact prebuilt releases are installed with a
// checksum and restored from skgo-plugins.json. Explicit skgo generate --plugins
// runs generators; skgo plugin <ID> dispatches plugin commands.
package templateapi

import "context"

// Plugin describes a trusted development-time extension. Describe and package
// initialization must not modify projects or install tools. Implement AddonPlugin,
// Generator, or CommandPlugin to provide the advertised capabilities.
type Plugin interface {
	Describe() Descriptor
}

// AddonPlugin applies a single configured addition. Existing template/add-on
// providers implement this optional capability.
type AddonPlugin interface {
	Plugin
	Apply(context.Context, Request) (Result, error)
}

type Descriptor struct {
	ID, Version, SkgoVersion string
	Addons                   []Addon
	Templates                []Template
	// Generator advertises the optional Generator capability.
	Generator bool
	// Command is the help summary for `skgo plugin <ID> ...`; empty disables it.
	Command string
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
