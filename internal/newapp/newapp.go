// Package newapp orchestrates upstream project creation and adds skgo's Go half.
//
// SvelteKit project files are created by VitePlus through sv. Frontend
// integration is owned by the separately published native @skgo/sv add-on.
// This package deliberately owns only orchestration and Go-specific project
// files.
package newapp

import (
	"archive/tar"
	"compress/gzip"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"text/template"
	"time"

	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

const (
	skgoModule      = "github.com/tylergannon/skgo"
	adapterPackage  = "@skgo/sveltekit-adapter"
	svAddonPackage  = "@skgo/sv"
	defaultRegistry = "https://registry.npmjs.org"
	defaultOrigin   = "http://127.0.0.1:8080"
)

var appName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

//go:embed gofiles
var goFiles embed.FS

// Options describes one project creation.
type Options struct {
	Dir         string
	Module      string
	App         string
	Origin      string
	Starter     string
	SkgoVersion string

	// SVAddonSpec overrides registry selection for the add-on. A file: spec
	// qualifies a checkout such as file:/path/to/internal/sv; sv never sees that
	// directory, only an isolated copy of what pnpm would pack from it.
	SVAddonSpec string
	// AdapterSpec independently overrides registry selection for the runtime
	// adapter the add-on installs.
	AdapterSpec string
	// SkgoReplace adds a local replace directive to the generated go.mod. It is
	// useful when qualifying a checkout; released generators leave it empty.
	SkgoReplace string

	RegistryURL string
	VP          string
	Client      *http.Client
	Stdout      io.Writer
	Stderr      io.Writer
	run         func(command) error
	addonStage  string
}

type command struct {
	Dir  string
	Name string
	Args []string
	Env  []string
}

// Result contains the instructions printed only after every setup stage has
// completed and the generated bindings have been produced.
type Result struct {
	Dir     string
	App     string
	Origin  string
	Starter string
}

func (r Result) Instructions() string {
	return fmt.Sprintf("skgo: created the %s project in %s. Now:\n\n\tcd %s\n\tjust dev\n\nThen open %s. Run `just storybook` for Storybook and `just test` for Vitest.\n", r.Starter, r.Dir, r.Dir, r.Origin)
}

type project struct {
	Dir               string
	App               string
	Module            string
	Origin            string
	OriginHostPort    string
	Starter           string
	SkgoVersion       string
	SkgoReplace       string
	GoVersion         string
	BindingsImport    string
	Examples          bool
	SVAddonSpec       string
	AdapterDependency string
	SVVersion         string
}

const storybookVersion = "10.6.0"

// escapeAddonOption produces an sv community add-on option value. sv uses "+"
// between options, while JavaScript's decodeURIComponent does not translate
// QueryEscape's space spelling back from "+". Keep literal plus signs and
// spaces percent-encoded so neither can become an option separator.
func escapeAddonOption(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}

// Create asks VitePlus to create an sv project, verifies the required upstream
// add-ons completed, then writes and initializes skgo's Go-specific files.
func Create(options Options) (Result, error) {
	p, err := resolve(options)
	if err != nil {
		return Result{}, err
	}
	if err := emptyDir(p.Dir); err != nil {
		return Result{}, err
	}

	run := options.run
	if run == nil {
		run = realRunner(options.Stdout, options.Stderr)
	}
	if source, ok := strings.CutPrefix(p.SVAddonSpec, "file:"); ok {
		staged, err := stageAddon(run, source, options.addonStage)
		if err != nil {
			return Result{}, fmt.Errorf("skgo: staging the sv add-on from %s failed: %w", source, err)
		}
		p.SVAddonSpec = "file:" + staged
	}

	addonArg := p.SVAddonSpec + "=starter:" + escapeAddonOption(p.Starter) +
		"+adapter:" + escapeAddonOption(p.AdapterDependency) +
		"+name:" + escapeAddonOption(p.App)
	vp := options.VP
	if vp == "" {
		vp = "vp"
	}
	create := command{
		Dir:  p.Dir,
		Name: vp,
		Args: []string{
			"create", "svelte@" + p.SVVersion,
			"--no-interactive", "--no-git", "--no-agent", "--no-editor", "--no-hooks",
			"--approve-builds", "--package-manager", "pnpm", "--",
			"web", "--template", "minimal", "--types", "ts", "--add",
			"vitest=usages:unit,component", addonArg, "--no-download-check",
		},
		Env: append(os.Environ(), "CI=1"),
	}
	if err := run(create); err != nil {
		return Result{}, fmt.Errorf("skgo: VitePlus project creation failed: %w", err)
	}
	if err := run(command{
		Dir: filepath.Join(p.Dir, "web"), Name: "pnpm",
		Args: []string{
			"dlx", "--allow-build", "esbuild",
			"--package", "create-storybook@" + storybookVersion,
			"--package", "@storybook/sveltekit@" + storybookVersion,
			"create-storybook", "--package-manager", "pnpm", "--skip-install",
			"--no-dev", "--no-features", "--yes", "--disable-telemetry",
		},
		// Supplying the exact framework package in pnpm's isolated dlx
		// environment lets create-storybook resolve its own templates locally,
		// instead of falling back to an npm registry lookup from the project.
		Env: append(os.Environ(), "CI=1"),
	}); err != nil {
		return Result{}, fmt.Errorf("skgo: Storybook's upstream installer failed: %w", err)
	}
	if err := run(command{
		Dir: filepath.Join(p.Dir, "web"), Name: filepath.Join("node_modules", ".bin", "vp"),
		Args: []string{"install"}, Env: os.Environ(),
	}); err != nil {
		return Result{}, fmt.Errorf("skgo: VitePlus could not install Storybook's dependencies: %w", err)
	}
	if err := verifyFrontend(p.Dir); err != nil {
		return Result{}, fmt.Errorf("skgo: upstream frontend setup was incomplete: %w", err)
	}
	if err := run(command{Dir: p.Dir, Name: "pnpm", Args: []string{"--dir", "web", "exec", "playwright", "install", "chromium"}, Env: os.Environ()}); err != nil {
		return Result{}, fmt.Errorf("skgo: installing the browser required by Vitest failed: %w", err)
	}
	if err := writeGoFiles(p); err != nil {
		return Result{}, err
	}
	if err := run(command{Dir: p.Dir, Name: "go", Args: []string{"mod", "tidy"}, Env: os.Environ()}); err != nil {
		return Result{}, fmt.Errorf("skgo: initializing the Go module failed: %w", err)
	}
	if err := run(command{Dir: p.Dir, Name: "go", Args: []string{"generate", "./..."}, Env: os.Environ()}); err != nil {
		return Result{}, fmt.Errorf("skgo: generating the Go bindings failed: %w", err)
	}
	if err := run(command{
		Dir: filepath.Join(p.Dir, "web"), Name: filepath.Join("node_modules", ".bin", "vp"),
		Args: []string{"build"}, Env: append(os.Environ(), "ORIGIN="+p.Origin),
	}); err != nil {
		return Result{}, fmt.Errorf("skgo: creating the initial frontend build failed: %w", err)
	}
	return Result{Dir: p.Dir, App: p.App, Origin: p.Origin, Starter: p.Starter}, nil
}

// stageAddon packs a local add-on directory the way a publication would and
// unpacks it somewhere sv may safely link. sv installs a file: add-on as a
// symlink inside its own shared pnpm-store directory and later extracts registry
// add-ons through whatever is already there, so a link to a live checkout lets
// an unrelated `skgo new` overwrite tracked files. The stage is a fixed
// per-user directory rather than a temporary one: a dangling link would break
// that later extraction instead.
func stageAddon(run func(command) error, source, stage string) (string, error) {
	source, err := filepath.Abs(source)
	if err != nil {
		return "", err
	}
	if stage == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		stage = filepath.Join(cache, "skgo", "sv-addon-qualification")
	}
	if err := os.RemoveAll(stage); err != nil {
		return "", err
	}
	if err := os.MkdirAll(stage, 0o755); err != nil {
		return "", err
	}
	if err := run(command{Dir: source, Name: "pnpm", Args: []string{"pack", "--pack-destination", stage}, Env: os.Environ()}); err != nil {
		return "", err
	}
	tarballs, err := filepath.Glob(filepath.Join(stage, "*.tgz"))
	if err != nil {
		return "", err
	}
	if len(tarballs) != 1 {
		return "", fmt.Errorf("pnpm pack left %d tarballs in %s, want 1", len(tarballs), stage)
	}
	if err := unpackTarball(tarballs[0], stage); err != nil {
		return "", err
	}
	// npm tarballs hold their files under package/.
	addon := filepath.Join(stage, "package")
	// Node resolves the add-on's imports from the link's target, not from sv's
	// directory, so the staged copy needs its own sv peer.
	if err := run(command{Dir: addon, Name: "pnpm", Args: []string{"install", "--prod", "--ignore-workspace"}, Env: os.Environ()}); err != nil {
		return "", err
	}
	return addon, nil
}

func unpackTarball(tarball, dir string) error {
	file, err := os.Open(tarball)
	if err != nil {
		return err
	}
	defer file.Close()
	zr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	tr := tar.NewReader(zr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		if !filepath.IsLocal(header.Name) {
			return fmt.Errorf("%s contains the non-local path %q", tarball, header.Name)
		}
		target := filepath.Join(dir, filepath.FromSlash(header.Name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
	}
}

func realRunner(stdout, stderr io.Writer) func(command) error {
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	return func(c command) error {
		cmd := exec.Command(c.Name, c.Args...)
		cmd.Dir = c.Dir
		cmd.Env = c.Env
		cmd.Stdin = os.Stdin
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				return fmt.Errorf("%s exited with status %d", c.Name, exit.ExitCode())
			}
			return err
		}
		return nil
	}
}

func resolve(o Options) (project, error) {
	if o.Dir == "" {
		return project{}, fmt.Errorf("skgo: new needs a directory")
	}
	dir, err := filepath.Abs(o.Dir)
	if err != nil {
		return project{}, err
	}
	p := project{Dir: dir, App: o.App, Module: o.Module, Origin: o.Origin, Starter: o.Starter, SkgoVersion: o.SkgoVersion, SkgoReplace: o.SkgoReplace}
	if p.App == "" {
		p.App = filepath.Base(dir)
	}
	if !appName.MatchString(p.App) {
		return project{}, fmt.Errorf("skgo: %q is not a usable application name", p.App)
	}
	if p.Module == "" {
		p.Module = p.App
	}
	if err := module.CheckImportPath(p.Module); err != nil {
		return project{}, fmt.Errorf("skgo: %q is not a usable Go module path: %w", p.Module, err)
	}
	if p.Origin == "" {
		p.Origin = defaultOrigin
	}
	origin, err := url.Parse(p.Origin)
	if err != nil || origin.Scheme == "" || origin.Host == "" || origin.Path != "" {
		return project{}, fmt.Errorf("skgo: %q is not an origin like %s", p.Origin, defaultOrigin)
	}
	p.OriginHostPort = origin.Host
	if p.Starter == "" {
		p.Starter = "minimal"
	}
	switch p.Starter {
	case "minimal":
	case "examples":
		p.Examples = true
	default:
		return project{}, fmt.Errorf("skgo: %q is not a starting point: choose minimal or examples", p.Starter)
	}
	if p.SkgoVersion == "" {
		p.SkgoVersion, err = currentSkgoVersion()
		if err != nil {
			return project{}, err
		}
	}
	if !semver.IsValid(p.SkgoVersion) {
		return project{}, fmt.Errorf("skgo: %q is not a valid skgo version", p.SkgoVersion)
	}
	if p.SkgoReplace != "" {
		p.SkgoReplace, err = filepath.Abs(p.SkgoReplace)
		if err != nil {
			return project{}, err
		}
	}
	p.GoVersion = strings.TrimPrefix(runtime.Version(), "go")
	p.BindingsImport = p.Module + "/internal/skgo"

	registry := strings.TrimRight(o.RegistryURL, "/")
	if registry == "" {
		registry = defaultRegistry
	}
	client := o.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	p.SVVersion, err = highestVersion(client, registry, "sv", func(v string) bool {
		return semver.Major("v"+v) == "v1"
	})
	if err != nil {
		return project{}, fmt.Errorf("skgo: selecting sv 1.x: %w", err)
	}
	max := strings.TrimPrefix(p.SkgoVersion, "v")
	compatible := func(v string) bool { return semver.Compare("v"+v, "v"+max) <= 0 }
	if o.SVAddonSpec != "" {
		p.SVAddonSpec = o.SVAddonSpec
	} else {
		addonVersion, err := highestVersion(client, registry, svAddonPackage, compatible)
		if err != nil {
			return project{}, fmt.Errorf("skgo: selecting the sv add-on for %s: %w", p.SkgoVersion, err)
		}
		p.SVAddonSpec = svAddonPackage + "@" + addonVersion
	}
	if o.AdapterSpec != "" {
		p.AdapterDependency = o.AdapterSpec
	} else {
		adapterVersion, err := highestVersion(client, registry, adapterPackage, compatible)
		if err != nil {
			return project{}, fmt.Errorf("skgo: selecting the adapter for %s: %w", p.SkgoVersion, err)
		}
		p.AdapterDependency = adapterVersion
	}
	return p, nil
}

func currentSkgoVersion() (string, error) {
	if bi, ok := debug.ReadBuildInfo(); ok {
		if bi.Main.Path == skgoModule && semver.IsValid(bi.Main.Version) {
			return bi.Main.Version, nil
		}
		for _, dep := range bi.Deps {
			if dep.Path == skgoModule && dep.Replace == nil && semver.IsValid(dep.Version) {
				return dep.Version, nil
			}
		}
	}
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Version}}", skgoModule+"@latest")
	cmd.Dir = os.TempDir()
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("skgo: this checkout does not identify a release and the Go proxy lookup failed: %w; pass --skgo-version when qualifying a checkout", err)
	}
	v := strings.TrimSpace(string(out))
	if !semver.IsValid(v) {
		return "", fmt.Errorf("skgo: the Go proxy returned invalid version %q", v)
	}
	return v, nil
}

type npmMetadata struct {
	Versions map[string]json.RawMessage `json:"versions"`
}

func highestVersion(client *http.Client, registry, name string, accept func(string) bool) (string, error) {
	endpoint := registry + "/" + strings.ReplaceAll(name, "/", "%2f")
	resp, err := client.Get(endpoint)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("registry GET %s returned %s: %s", endpoint, resp.Status, strings.TrimSpace(string(body)))
	}
	var metadata npmMetadata
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return "", fmt.Errorf("decoding registry metadata: %w", err)
	}
	versions := make([]string, 0, len(metadata.Versions))
	for version := range metadata.Versions {
		if semver.IsValid("v"+version) && accept(version) {
			versions = append(versions, version)
		}
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("registry metadata for %s contains no compatible version", name)
	}
	slices.SortFunc(versions, func(a, b string) int { return semver.Compare("v"+a, "v"+b) })
	return versions[len(versions)-1], nil
}

func emptyDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return os.MkdirAll(dir, 0o755)
	}
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return fmt.Errorf("skgo: %s is not empty", dir)
	}
	return nil
}

type packageJSON struct {
	Scripts         map[string]string `json:"scripts"`
	DevDependencies map[string]string `json:"devDependencies"`
}

func verifyFrontend(root string) error {
	web := filepath.Join(root, "web")
	raw, err := os.ReadFile(filepath.Join(web, "package.json"))
	if err != nil {
		return fmt.Errorf("VitePlus did not produce web/package.json: %w", err)
	}
	var pkg packageJSON
	if err := json.Unmarshal(raw, &pkg); err != nil {
		return fmt.Errorf("reading web/package.json: %w", err)
	}
	for _, script := range []string{"dev", "build", "test:unit", "storybook"} {
		if pkg.Scripts[script] == "" {
			return fmt.Errorf("the upstream %s setup did not add the %q script", owner(script), script)
		}
	}
	if pkg.DevDependencies[adapterPackage] == "" {
		return fmt.Errorf("the skgo sv add-on did not add %s", adapterPackage)
	}
	requiredGlobs := []struct {
		claim string
		glob  string
	}{
		{"Vitest", filepath.Join(web, "src", "**", "*.spec.ts")},
		{"Storybook configuration", filepath.Join(web, ".storybook", "main.*")},
		{"Storybook stories", filepath.Join(web, "src", "**", "*.stories.*")},
	}
	for _, required := range requiredGlobs {
		matches, err := doublestar(web, required.glob)
		if err != nil {
			return err
		}
		if len(matches) == 0 {
			return fmt.Errorf("%s produced no files matching %s", required.claim, required.glob)
		}
	}
	for _, executable := range []string{"vp", "vitest", "storybook"} {
		if _, err := os.Stat(filepath.Join(web, "node_modules", ".bin", executable)); err != nil {
			return fmt.Errorf("the installed project has no %s executable: %w", executable, err)
		}
	}
	return nil
}

func owner(script string) string {
	if strings.HasPrefix(script, "test") {
		return "Vitest"
	}
	if script == "storybook" {
		return "Storybook"
	}
	return "SvelteKit"
}

// doublestar implements the two patterns verification needs without adding a
// glob dependency to the generator: ** means any descendant directory.
func doublestar(root, pattern string) ([]string, error) {
	pattern = filepath.Clean(pattern)
	var matches []string
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && entry.Name() == "node_modules" {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		parts := strings.Split(pattern, string(filepath.Separator)+"**"+string(filepath.Separator))
		if len(parts) == 1 {
			ok, err := filepath.Match(pattern, name)
			if err != nil {
				return err
			}
			if ok {
				matches = append(matches, name)
			}
			return nil
		}
		if !strings.HasPrefix(name, parts[0]+string(filepath.Separator)) {
			return nil
		}
		ok, err := filepath.Match(parts[1], filepath.Base(name))
		if err != nil {
			return err
		}
		if ok {
			matches = append(matches, name)
		}
		return nil
	})
	return matches, err
}

func writeGoFiles(p project) error {
	root, err := fs.Sub(goFiles, "gofiles")
	if err != nil {
		return err
	}
	return fs.WalkDir(root, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		if strings.HasPrefix(name, "examples/") && !p.Examples {
			return nil
		}
		targetName := strings.TrimPrefix(name, "examples/")
		targetName = strings.TrimSuffix(targetName, ".tmpl")
		if base, ok := strings.CutPrefix(path.Base(targetName), "dot-"); ok {
			targetName = path.Join(path.Dir(targetName), "."+base)
		}
		target := filepath.Join(p.Dir, filepath.FromSlash(targetName))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		body, err := fs.ReadFile(root, name)
		if err != nil {
			return err
		}
		if strings.HasSuffix(name, ".tmpl") {
			tmpl, err := template.New(name).Option("missingkey=error").Parse(string(body))
			if err != nil {
				return err
			}
			var rendered strings.Builder
			if err := tmpl.Execute(&rendered, p); err != nil {
				return err
			}
			body = []byte(rendered.String())
		}
		return os.WriteFile(target, body, 0o644)
	})
}
