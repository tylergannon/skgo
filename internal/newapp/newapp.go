// Package newapp scaffolds a project that runs.
//
// It writes the shape a skgo app has — an ordinary SvelteKit frontend, the Go
// server that owns the socket, and the one generated package that joins them —
// and nothing else. Everything it writes is either something SvelteKit
// requires, something Go requires, or the demo route that proves the two halves
// reached each other.
//
// The scaffolded project consumes skgo as an ordinary module dependency. It
// carries no `replace` directive, and `go tool skgo` builds the generator out
// of the module cache, where every file is read-only. That is the only
// configuration in which "works for us" and "works for anyone" can differ, so
// it is the one the template is written for.
package newapp

import (
	"embed"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"text/template"

	"github.com/tylergannon/skgo/internal/adapter"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

//go:embed template
var templateFS embed.FS

// skgoModule is the module the generated project depends on. It is spelled out
// rather than derived, because a scaffold that pointed at whatever module
// happened to build it would be one `go build` in a fork away from writing a
// go.mod nobody can resolve.
const skgoModule = "github.com/tylergannon/skgo"

// polytypeModule projects the Go types that cross to TypeScript. skgo drives
// it as a library, so the generated project requires it only to pin the
// version.
const polytypeModule = "github.com/tylergannon/polytype"

// defaultPolytypeVersion is the polytype the example is built against. A
// generated project pins the same one: the projection is what the browser
// receives, so it is not a version to be casual about.
const defaultPolytypeVersion = "v1.0.0-rc.11"

// defaultOrigin is where a new app is served in development. It is the value
// the frontend is built with and the value the binary trusts.
const defaultOrigin = "http://127.0.0.1:8080"

const defaultBuildTool = "mise"

// appName is what a project may be called: the name becomes a binary, a Go
// string literal and half a package.json, so it stays boring on purpose.
var appName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Options describes the project to write.
type Options struct {
	// Dir is where the project goes. It is created if it does not exist, and
	// must be empty if it does.
	Dir string
	// Module is the Go module path. It defaults to App.
	Module string
	// App is the project's name: the binary, the heading on its home page.
	// Defaults to the base name of Dir.
	App string
	// Origin is the URL a browser reaches the app at. Defaults to
	// http://127.0.0.1:8080.
	Origin string
	// SkgoVersion is the version of skgo the project requires. Empty means
	// "work it out": the version this binary was built from if it has one,
	// otherwise whatever the module proxy calls latest.
	SkgoVersion string
	// PolytypeVersion is the version of polytype the project requires.
	PolytypeVersion string
	// AdapterSpec is what package.json asks pnpm for when it asks for
	// `@skgo/sveltekit-adapter`. Empty means the exact npm version paired with
	// SkgoVersion, which is the only pairing a project can serve. A test passes
	// a `file:` tarball here.
	AdapterSpec string
	// GoVersion is the language version in go.mod. Defaults to the toolchain's.
	GoVersion string
	// BuildTool selects the generated build entry point: mise, just, or scripts.
	// Empty preserves the original scaffold behavior and selects mise.
	BuildTool string
	// Logf receives one line per file written. It may be nil.
	Logf func(format string, args ...any)
}

// data is what the templates see.
type data struct {
	App             string
	Module          string
	Origin          string
	OriginHostPort  string
	SkgoVersion     string
	PolytypeVersion string
	AdapterSpec     string
	GoVersion       string
	BuildTool       string
	BuildCommand    string
	DevWebCommand   string
	DevGoCommand    string
	BuildConfig     string
	ToolRequirement string
}

// Create writes the project described by o.
func Create(o Options) error {
	if o.Logf == nil {
		o.Logf = func(string, ...any) {}
	}
	if o.Dir == "" {
		return fmt.Errorf("skgo: new needs a directory to write the project into")
	}
	dir, err := filepath.Abs(o.Dir)
	if err != nil {
		return err
	}

	d, err := resolve(o, dir)
	if err != nil {
		return err
	}
	if err := emptyDir(dir); err != nil {
		return err
	}

	tmplRoot, err := fs.Sub(templateFS, "template")
	if err != nil {
		return err
	}
	err = fs.WalkDir(tmplRoot, ".", func(p string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		if !templateForBuildTool(p, d.BuildTool) {
			return nil
		}
		target := filepath.Join(dir, filepath.FromSlash(outputPath(p)))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		body, err := fs.ReadFile(tmplRoot, p)
		if err != nil {
			return err
		}
		if strings.HasSuffix(p, ".tmpl") {
			if body, err = render(p, body, d); err != nil {
				return err
			}
		}
		mode := fs.FileMode(0o644)
		if strings.HasPrefix(p, "scripts/") {
			mode = 0o755
		}
		if err := os.WriteFile(target, body, mode); err != nil {
			return err
		}
		o.Logf("wrote %s", outputPath(p))
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func templateForBuildTool(p, buildTool string) bool {
	switch {
	case p == "mise.toml.tmpl":
		return buildTool == "mise"
	case p == "Justfile.tmpl":
		return buildTool == "just"
	case strings.HasPrefix(p, "scripts/"):
		return buildTool == "scripts"
	default:
		return true
	}
}

// outputPath is the path a template file is written to: `.tmpl` is a marker for
// "substitute into this", not part of the name, and a `dot-` prefix stands for
// the leading dot a file cannot have inside skgo's own tree — a real
// `.gitignore` there would be a .gitignore for skgo.
func outputPath(p string) string {
	segments := strings.Split(strings.TrimSuffix(p, ".tmpl"), "/")
	for i, s := range segments {
		if rest, ok := strings.CutPrefix(s, "dot-"); ok {
			segments[i] = "." + rest
		}
	}
	return path.Join(segments...)
}

func render(name string, body []byte, d data) ([]byte, error) {
	t, err := template.New(name).Option("missingkey=error").Parse(string(body))
	if err != nil {
		return nil, fmt.Errorf("skgo: %s: %w", name, err)
	}
	var out strings.Builder
	if err := t.Execute(&out, d); err != nil {
		return nil, fmt.Errorf("skgo: %s: %w", name, err)
	}
	return []byte(out.String()), nil
}

// resolve fills in every default and refuses the values that would produce a
// project that does not build.
func resolve(o Options, dir string) (data, error) {
	d := data{
		App:             o.App,
		Module:          o.Module,
		Origin:          o.Origin,
		SkgoVersion:     o.SkgoVersion,
		PolytypeVersion: o.PolytypeVersion,
		AdapterSpec:     o.AdapterSpec,
		GoVersion:       o.GoVersion,
		BuildTool:       o.BuildTool,
	}
	if d.BuildTool == "" {
		d.BuildTool = defaultBuildTool
	}
	switch d.BuildTool {
	case "mise":
		d.BuildCommand = "mise run build"
		d.DevWebCommand = "mise run dev:web"
		d.DevGoCommand = "mise run dev:go"
		d.BuildConfig = "mise.toml"
		d.ToolRequirement = "mise (it installs the pinned Node and Vite+ versions)"
	case "just":
		d.BuildCommand = "just build"
		d.DevWebCommand = "just dev-web"
		d.DevGoCommand = "just dev-go"
		d.BuildConfig = "Justfile"
		d.ToolRequirement = "Node 24, pnpm 11, and just"
	case "scripts":
		d.BuildCommand = "./scripts/build.sh"
		d.DevWebCommand = "./scripts/dev-web.sh"
		d.DevGoCommand = "./scripts/dev-go.sh"
		d.BuildConfig = "scripts/env.sh"
		d.ToolRequirement = "Node 24 and pnpm 11"
	default:
		return data{}, fmt.Errorf("skgo: %q is not a build tool: choose mise, just, or scripts", d.BuildTool)
	}
	if d.App == "" {
		d.App = filepath.Base(dir)
	}
	if !appName.MatchString(d.App) {
		return data{}, fmt.Errorf("skgo: %q is not a usable app name: it becomes a binary and a package.json name, so it must start with a letter or digit and hold only letters, digits, dot, dash and underscore", d.App)
	}
	if d.Module == "" {
		d.Module = d.App
	}
	if err := module.CheckImportPath(d.Module); err != nil {
		return data{}, fmt.Errorf("skgo: %q is not a usable module path: %w", d.Module, err)
	}
	if d.Origin == "" {
		d.Origin = defaultOrigin
	}
	u, err := url.Parse(d.Origin)
	if err != nil || u.Scheme == "" || u.Host == "" || u.Path != "" {
		return data{}, fmt.Errorf("skgo: %q is not an origin: it needs a scheme and a host and nothing else, like %s", d.Origin, defaultOrigin)
	}
	d.OriginHostPort = u.Host
	if d.PolytypeVersion == "" {
		d.PolytypeVersion = defaultPolytypeVersion
	}
	if d.GoVersion == "" {
		d.GoVersion = strings.TrimPrefix(runtime.Version(), "go")
	}
	if d.SkgoVersion == "" {
		if d.SkgoVersion, err = skgoVersion(); err != nil {
			return data{}, err
		}
	}
	if !semver.IsValid(d.SkgoVersion) {
		return data{}, fmt.Errorf("skgo: %q is not a version go.mod can require", d.SkgoVersion)
	}
	if d.AdapterSpec == "" {
		// The adapter and the Go module are one contract released together. The
		// fingerprint in the manifest is the real gate; this exact npm version
		// only names the same release the go.mod line does.
		d.AdapterSpec = adapter.RegistrySpec(d.SkgoVersion)
	}
	return d, nil
}

// skgoVersion is the version of skgo the generated project requires.
//
// The honest answer is the version of the skgo that is writing the project, and
// when this binary was itself built from the module cache — `go tool skgo`, `go
// install` — the build info says so. A binary built from a checkout reports
// `(devel)`, which no go.mod can require, so the fallback is to ask the module
// proxy what the latest release is. Either way the generated project points at
// a module anyone can fetch, which is the whole point of scaffolding one.
func skgoVersion() (string, error) {
	if bi, ok := debug.ReadBuildInfo(); ok {
		if v := versionOf(bi, skgoModule); semver.IsValid(v) {
			return v, nil
		}
	}
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Version}}", skgoModule+"@latest")
	// Outside any module: the question is about the proxy, not about whatever
	// module the developer happens to be standing in.
	cmd.Dir = os.TempDir()
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("skgo: this skgo was built from a checkout, so it does not know its own version, and asking the module proxy for the latest %s failed: %w\n\tPass --skgo-version to say which one the new project should require", skgoModule, err)
	}
	v := strings.TrimSpace(string(out))
	if !semver.IsValid(v) {
		return "", fmt.Errorf("skgo: the module proxy answered %q for %s@latest, which is not a version", v, skgoModule)
	}
	return v, nil
}

func versionOf(bi *debug.BuildInfo, path string) string {
	if bi.Main.Path == path {
		return bi.Main.Version
	}
	for _, dep := range bi.Deps {
		if dep.Path == path {
			return dep.Version
		}
	}
	return ""
}

// emptyDir makes dir, and refuses to write into one that already holds
// something. Scaffolding over an existing project would overwrite the files a
// developer had already changed.
func emptyDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return os.MkdirAll(dir, 0o755)
	}
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("skgo: %s is not empty; new writes a whole project and will not write it over one that is already there", dir)
	}
	return nil
}
