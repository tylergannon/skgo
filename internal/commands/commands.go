// Package commands is the table of the skgo command's subcommands: name,
// arguments, flags and the sentences that describe them. The command prints
// its usage from this table. A test in cmd/skgo keeps the flag entries equal
// to the flag sets the commands actually define.
package commands

import (
	"fmt"
	"strings"
)

// Flag describes one flag of a command.
type Flag struct {
	Name    string
	Default string
	Usage   string
}

// Command describes one subcommand.
type Command struct {
	// Name is the word after skgo, or two words for a nested command such as
	// "native build".
	Name string
	// Args is the argument syntax after the name and flags, for the synopsis.
	Args string
	// Summary is one sentence without a trailing period, for the usage listing.
	Summary string
	// Description is one or more markdown paragraphs for the reference page.
	Description string
	// Flags in the order the command defines them. Nil when the command takes none.
	Flags []Flag
	// Internal commands are dispatched but not listed or documented.
	Internal bool
}

// Synopsis is the one-line invocation: skgo NAME [flags] ARGS.
func (c Command) Synopsis() string {
	parts := []string{"skgo", c.Name}
	if len(c.Flags) > 0 {
		parts = append(parts, "[flags]")
	}
	if c.Args != "" {
		parts = append(parts, c.Args)
	}
	return strings.Join(parts, " ")
}

// All lists every public command in the order the usage prints them.
func All() []Command {
	var out []Command
	for _, c := range table {
		if !c.Internal {
			out = append(out, c)
		}
	}
	return out
}

// Lookup finds a command by name, including internal ones.
func Lookup(name string) (Command, bool) {
	for _, c := range table {
		if c.Name == name {
			return c, true
		}
	}
	return Command{}, false
}

// Usage is the text `skgo help` prints: every public command with its
// synopsis and summary.
func Usage() string {
	var b strings.Builder
	b.WriteString("usage:\n")
	for _, c := range All() {
		fmt.Fprintf(&b, "\t%s\n\t\t%s\n", c.Synopsis(), c.Summary)
	}
	b.WriteString("\nRun skgo COMMAND --help for the command's flags.\n")
	return b.String()
}

var table = []Command{
	{
		Name:    "new",
		Args:    "DIR [-- SV_CREATE_OPTIONS]",
		Summary: "create a SvelteKit application served by Go",
		Description: `VitePlus delegates creation to sv; skgo adds the Go application server,
the generated bindings package and a Justfile. Options after ` + "`--`" + ` belong
to ` + "`sv create`" + `; without a terminal the base is minimal TypeScript.
Templates registered by installed plugins are applied after that scaffold.

` + "```sh\nskgo new myapp -- --template demo --types jsdoc --add tailwindcss=plugins:none\n```",
		Flags: []Flag{
			{"module", "", "Go module path; defaults to the project name"},
			{"name", "", "application name; defaults to the target directory name"},
			{"origin", "http://127.0.0.1:8080", "public browser origin"},
			{"skgo-version", "", "skgo module version; defaults to this command's release"},
			{"sv-addon", "", "sv add-on package override for checkout qualification"},
			{"adapter", "", "adapter package override for checkout qualification"},
			{"skgo-replace", "", "local skgo module replacement for checkout qualification"},
			{"template", "", "application template applied after the ordinary minimal TypeScript scaffold"},
			{"set", "", "template option NAME=VALUE (repeatable)"},
		},
	},
	{
		Name:    "plugin",
		Args:    "install MODULE[@QUERY] | remove MODULE",
		Summary: "install or remove trusted plugin source for this executable",
		Description: `Installs trusted Git source for this executable, which contributes templates
and add-ons to ` + "`skgo new`" + ` and ` + "`skgo add`" + `. Requires Git, Go and cgo.
Source generation and plugin initialization execute code. Removal affects only
this host's managed selection.`,
	},
	{
		Name:    "add",
		Args:    "ADDON",
		Summary: "apply a reusable application change",
		Description: `Applies an add-on registered by an installed plugin to the application at
` + "`--root`" + `. ` + "`skgo add --help`" + ` lists the add-ons the executable knows.`,
		Flags: []Flag{
			{"root", ".", "skgo application root"},
			{"name", "", "explicit application identity for older projects; must match an existing identity"},
			{"set", "", "add-on option NAME=VALUE (repeatable)"},
		},
	},
	{
		Name:    "update",
		Summary: "align CLI and application toolchain",
		Description: `Brings the application's skgo module, adapter and frontend tool environment to
the versions this executable was released with, then re-runs generation. When
the application has native configuration, the native target is rebuilt too.`,
		Flags: []Flag{
			{"root", "", "application root; otherwise locate the current application"},
			{"vp", "", "global vp executable; defaults to PATH"},
			{"native-platform", "macos", "native build target when native configuration exists: macos, iphone or simulator"},
			{"native-preset", "", "native build preset from the application configuration"},
			{"complete-version", "", "internal completion phase: must match this executable"},
		},
	},
	{
		Name:    "native build",
		Summary: "build the macOS or iOS application",
		Description: `Prepares bindings, native libraries and the Xcode project for the selected
platform and compiles a Debug application. ` + "`--prepare-only`" + ` stops before
Xcode, which is what an editor or CI step that owns the Xcode invocation wants.`,
		Flags: []Flag{
			{"root", ".", "application root"},
			{"platform", "macos", "macos, iphone or simulator"},
			{"preset", "", "application-defined build preset"},
			{"prepare-only", "false", "prepare bindings, native libraries and Xcode project without compiling a Debug app"},
			{"jobs", "0", "maximum Zig/Xcode build workers; zero uses the tool defaults"},
		},
	},
	{
		Name:    "build",
		Summary: "incrementally generate and build the frontend",
		Description: `Runs generation and the frontend build only when their inputs changed since the
last build, recorded under ` + "`.skgo/`" + ` in the application root. The Go
binary is built by ` + "`go build`" + ` as usual; the Justfile a new application
gets chains the two.`,
		Flags: []Flag{{"root", ".", "application root"}},
	},
	{
		Name:    "buildinfo",
		Summary: "identify this executing CLI and its Go build",
		Description: `Prints the executable's version, the Go release it was built with and the
module build settings, so a bug report or a toolchain check can name the exact
binary.`,
		Flags: []Flag{{"json", "false", "write executing binary and recorded Go build identity as JSON"}},
	},
	{
		Name:    "dev",
		Summary: "develop with Go and Vite",
		Description: `Runs the development server: the Go application with hot reload of the Go
side and Vite serving the frontend, on one public listener. Generation runs
again whenever a ` + "`*.remote.go`" + ` or load file changes.`,
		Flags: []Flag{
			{"root", ".", "application root containing go.mod"},
			{"web", "web", "frontend root relative to --root"},
			{"out", "", "generated bindings directory; detected when omitted"},
			{"listen", "127.0.0.1:8080", "public listener address (next free port when occupied)"},
			{"strict-port", "false", "fail when the requested port is occupied"},
			{"go", "go", "Go executable"},
			{"generator-config", "", "optional application generator JSON configuration, relative to --root"},
			{"vp", "vp", "VitePlus executable"},
		},
	},
	{
		Name:    "generate",
		Summary: "generate the glue between the Go server and the SvelteKit app",
		Description: `Reads every ` + "`*.remote.go`" + `, ` + "`page.server.go`" + `, ` + "`layout.server.go`" + ` and
` + "`server.go`" + ` under the application's ` + "`src/routes`" + ` and writes the
` + "`.remote.ts`" + ` modules Kit compiles, the TypeScript declarations their callers
see, and the Go registration the server mounts. New applications run it from a
` + "`go:generate`" + ` directive in the bindings package:

` + "```go\n//go:generate go tool skgo generate --web ../web\n```",
		Flags: []Flag{
			{"web", "../web", "the vite root: the directory holding src/ and package.json"},
			{"out", ".", "the directory of the generated bindings package"},
			{"package", "", "the name of the generated bindings package; defaults to the base name of --out"},
			{"locals-package", "", "application locals Go import path (required)"},
			{"locals-type", "Locals", "application locals named struct"},
			{"hook-package", "", "optional request hook Go import path"},
			{"hook-symbol", "Handle", "selected request hook symbol"},
			{"config-package", "", "optional shared ApplicationConfig Go import path"},
			{"config-symbol", "Config", "selected ApplicationConfig value"},
			{"cache-dir", "", "generation cache directory; defaults to the user cache; off disables caching"},
			{"quiet", "false", "do not list the files written"},
			{"swift-out", "", "generated Swift source file"},
			{"swift-remote", "", "select module#export for Swift and the shared safe-number contract (ordinary query/command; repeatable)"},
		},
	},
	{
		Name:    "check",
		Summary: "check Go, Svelte, lint and formatting without edits",
		Description: `Runs the Go analyzers, ` + "`svelte-check`" + `, lint and formatting over the
application and reports every finding without changing a file. The
` + "`--json`" + ` report is what editors and agents read; ` + "`skgo mcp`" + ` serves the
same check as a tool.`,
		Flags: []Flag{
			{"root", ".", "project root containing go.mod"},
			{"web", "web", "frontend root, relative to --root"},
			{"out", "", "generated Go bindings directory, relative to --root; detected when omitted"},
			{"locals-package", "", "application locals Go import path (required)"},
			{"locals-type", "Locals", "application locals named struct"},
			{"hook-package", "", "optional request hook Go import path"},
			{"hook-symbol", "Handle", "selected request hook symbol"},
			{"config-package", "", "optional shared ApplicationConfig Go import path"},
			{"config-symbol", "Config", "selected ApplicationConfig value"},
			{"json", "false", "write a structured JSON report"},
		},
	},
	{
		Name:    "kit-patch",
		Args:    "(--apply | --check)",
		Summary: "configure or verify Kit's declared-Inputs queue correction",
		Description: `skgo ships a correction to Kit's post-build prerender queue as a project-local
pnpm patch. ` + "`--apply`" + ` pins the supported Kit release and configures the
patch without installing; run the frontend's VitePlus install, then
` + "`--check`" + ` verifies the configured files and the installed Kit bytes.
` + "`--workspace`" + `, when supplied, must equal ` + "`--web`" + `; parent workspaces are
never modified.`,
		Flags: []Flag{
			{"web", "web", "selected frontend root containing package.json"},
			{"workspace", "", "assert the selected frontend is this root; parent workspaces are never modified"},
			{"apply", "false", "pin the supported Kit release and configure the project-local pnpm patch (does not install)"},
			{"check", "false", "verify configured files and actual installed Kit queue bytes"},
		},
	},
	{
		Name:    "advice",
		Args:    "[SKGO001..SKGO008]",
		Summary: "show installed rule guidance and repair examples",
		Description: `Prints the analyzer rules this executable enforces, each with its consequence,
repair and an example of valid skgo code. Name one code to see one rule.`,
		Flags: []Flag{{"json", "false", "write structured JSON guidance"}},
	},
	{
		Name:    "env",
		Summary: "resolve declared environment variables from JSON on stdin",
		Description: `Reads the generated environment declaration and a JSON object of raw values
on stdin, applies defaults and validation, and writes the resolved values as
JSON. Deployment tooling uses it to check an environment before starting the
server.`,
		Flags: []Flag{{"schema", "skgo.env.json", "generated environment declaration"}},
	},
	{
		Name:    "mcp",
		Summary: "serve check and advice tools over stdio MCP",
		Description: `Serves ` + "`skgo check`" + ` and ` + "`skgo advice`" + ` as Model Context Protocol tools
on standard input and output, for editors and agents. Standard output carries
only newline-delimited JSON-RPC messages; the server exits when its input
closes. It takes no flags or arguments.`,
	},
	{Name: "_plugin-probe", Internal: true},
}
