package newapp

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestHighestVersionIncludesPrereleasesButNotAnotherMajor(t *testing.T) {
	registry := registryServer(t, `{"versions":{"0.17.0":{},"1.0.0-next.7":{},"1.0.0":{},"1.1.0-beta.2":{},"2.0.0-alpha.1":{}}}`)
	got, err := highestVersion(registry.Client(), registry.URL, "sv", func(version string) bool {
		return strings.HasPrefix(version, "1.")
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "1.1.0-beta.2" {
		t.Fatalf("highest sv 1.x = %q, want 1.1.0-beta.2", got)
	}
}

// upstream stands in for VitePlus, sv and create-storybook: each step leaves
// what the real installer leaves, so Create is judged on what it asks for and
// on what it concludes from the project it finds.
type upstream struct {
	t        *testing.T
	created  map[string]string // what `vp create` leaves in web/, beyond package.json
	devDeps  map[string]string
	scripts  map[string]string
	skipAdd  bool // sv add exits 0 having applied nothing
	skipBook bool // create-storybook exits 0 having applied nothing
	commands []command
}

func (u *upstream) run(c command) error {
	u.t.Helper()
	u.commands = append(u.commands, c)
	localVP := filepath.Join("node_modules", ".bin", "vp")
	switch {
	case c.Name == "vp-test":
		u.devDeps = merge(map[string]string{"@sveltejs/kit": "^3.0.0-next.0"}, u.devDeps)
		u.scripts = merge(map[string]string{"dev": "vp dev", "build": "vp build"}, u.scripts)
		files := merge(map[string]string{"src/routes/+page.svelte": "sv's page\n", "node_modules/.bin/vp": ""}, u.created)
		writeFiles(u.t, filepath.Join(c.Dir, "web"), files)
		u.writePackage(filepath.Join(c.Dir, "web"))
	case c.Name == localVP && c.Args[0] == "dlx" && !u.skipAdd:
		joined := strings.Join(c.Args, " ")
		if strings.Contains(joined, "vitest=") {
			u.devDeps["vitest"] = "^4.1.8"
			u.scripts["test:unit"] = "vitest"
			// sv's vitest add-on: Playwright comes with component testing only.
			if usages := regexp.MustCompile(`vitest=usages:(\S+)`).FindStringSubmatch(joined); usages != nil && slices.Contains(strings.Split(usages[1], ","), "component") {
				u.devDeps["@vitest/browser-playwright"] = "^4.1.8"
				u.devDeps["playwright"] = "^1.60.0"
				writeFiles(u.t, c.Dir, map[string]string{"node_modules/.bin/playwright": ""})
			}
			writeFiles(u.t, c.Dir, map[string]string{"src/lib/vitest-examples/greet.spec.ts": "test", "node_modules/.bin/vitest": ""})
		}
		u.devDeps[adapterPackage] = "file:/candidate/adapter"
		if strings.Contains(joined, "=starter:examples") {
			if err := os.RemoveAll(filepath.Join(c.Dir, "src", "routes", "sverdle")); err != nil {
				u.t.Fatal(err)
			}
			writeFiles(u.t, c.Dir, map[string]string{"src/routes/+page.svelte": "import { status } from './example.remote';\n"})
		}
		u.writePackage(c.Dir)
	case c.Name == "pnpm" && slices.Contains(c.Args, "playwright"):
		// What pnpm does for real: there is no such binary unless upstream
		// declared the dependency.
		if u.devDeps["playwright"] == "" {
			return fmt.Errorf(`ERR_PNPM_RECURSIVE_EXEC_FIRST_FAIL Command "playwright" not found`)
		}
	case c.Name == "pnpm" && slices.Contains(c.Args, "create-storybook") && !u.skipBook:
		u.scripts["storybook"] = "storybook dev -p 6006"
		writeFiles(u.t, c.Dir, map[string]string{".storybook/main.ts": "export default {}", "src/stories/Button.stories.svelte": "story", "node_modules/.bin/storybook": ""})
		u.writePackage(c.Dir)
	}
	return nil
}

func (u *upstream) writePackage(web string) {
	u.t.Helper()
	raw, err := json.Marshal(map[string]any{"scripts": u.scripts, "devDependencies": u.devDeps})
	if err != nil {
		u.t.Fatal(err)
	}
	writeFiles(u.t, web, map[string]string{"package.json": string(raw)})
}

func merge(base, over map[string]string) map[string]string {
	for k, v := range over {
		base[k] = v
	}
	return base
}

func (u *upstream) create() command { return u.commands[0] }

// svArgs is what VitePlus hands sv: everything after the "--" separator.
func svArgs(t *testing.T, create command) []string {
	t.Helper()
	separator := slices.Index(create.Args, "--")
	if separator < 0 || create.Args[separator+1] != "web" {
		t.Fatalf("sv's target must be web and must precede its options: %#v", create.Args)
	}
	return create.Args[separator+2:]
}

func (u *upstream) options(dir string, registry *httptest.Server) Options {
	return Options{
		Dir: dir, Module: "example.com/hello-go", SkgoVersion: "v0.4.1", SkgoReplace: u.t.TempDir(),
		SVAddonSpec: "@skgo/sv@0.4.0", AdapterSpec: "file:/candidate/adapter",
		RegistryURL: registry.URL, Client: registry.Client(), VP: "vp-test", run: u.run,
	}
}

func TestCreateWithoutATerminalSettlesTheMinimalTypeScriptApplication(t *testing.T) {
	registry := registryServer(t, `{"versions":{"1.0.0-next.6":{},"1.0.0-next.7":{},"2.0.0":{}}}`)
	dir := filepath.Join(t.TempDir(), "hello-go")
	u := &upstream{t: t}
	result, err := Create(u.options(dir, registry))
	if err != nil {
		t.Fatal(err)
	}
	if result.Starter != "minimal" || !strings.Contains(result.Instructions(), "just dev") {
		t.Fatalf("result = %+v; instructions = %q", result, result.Instructions())
	}
	commands := u.commands
	if len(commands) != 8 {
		t.Fatalf("commands = %#v; want VitePlus create, sv add, Storybook, VitePlus install, Playwright, go mod tidy, go generate, initial VitePlus build", commands)
	}
	if got := strings.Join(commands[4].Args, " "); got != "--dir web exec playwright install chromium" {
		t.Fatalf("the mandatory component tests need Chromium; command 5 = %q", got)
	}
	if readme := readFile(t, filepath.Join(dir, "README.md")); !strings.Contains(readme, "pnpm --dir web exec playwright install chromium") {
		t.Errorf("the README does not tell a fresh clone how to get the component tests' browser:\n%s", readme)
	}
	if got := commands[0].Args[1]; got != "svelte@1.0.0-next.7" {
		t.Fatalf("VitePlus template = %q", got)
	}
	if !slices.Contains(commands[0].Args, "--no-interactive") || !slices.Contains(commands[0].Env, "CI=1") {
		t.Fatalf("an unattended run may not leave upstream free to prompt: %#v", commands[0].Args)
	}
	if got := strings.Join(commands[0].Args, " "); !strings.Contains(got, "--package-manager pnpm") {
		t.Fatalf("VitePlus was not told the project's package manager: %s", got)
	}
	if got, want := svArgs(t, commands[0]), []string{"--template", "minimal", "--types", "ts", "--no-add-ons"}; !slices.Equal(got, want) {
		t.Fatalf("sv create options = %q, want %q", got, want)
	}
	add := strings.Join(commands[1].Args, " ")
	for _, want := range []string{"dlx sv@1.0.0-next.7 add vitest=usages:unit,component @skgo/sv@0.4.0=starter:minimal+adapter:file%3A%2Fcandidate%2Fadapter+name:hello-go", "--no-install"} {
		if !strings.Contains(add, want) {
			t.Errorf("sv add args do not contain %q:\n%s", want, add)
		}
	}
	if commands[1].Name != filepath.Join("node_modules", ".bin", "vp") || filepath.Base(commands[1].Dir) != "web" {
		t.Fatalf("the skgo integration was not added through the project's VitePlus: %#v", commands[1])
	}
	if got := strings.Join(commands[2].Args, " "); !strings.Contains(got, "dlx --allow-build esbuild --package create-storybook@10.6.0 --package @storybook/sveltekit@10.6.0 create-storybook") {
		t.Fatalf("Storybook did not use its upstream installer with pnpm approval: %s", got)
	}
	if joined := strings.Join(commands[2].Env, "\n"); strings.Contains(joined, "npm_config_force=") {
		t.Fatalf("Storybook installer received broad npm force override:\n%s", joined)
	}
	if filepath.Base(commands[2].Dir) != "web" {
		t.Fatalf("Storybook installer ran outside the generated frontend: %s", commands[2].Dir)
	}
	if commands[3].Name != filepath.Join("node_modules", ".bin", "vp") || filepath.Base(commands[3].Dir) != "web" || commands[3].Args[0] != "install" {
		t.Fatalf("Storybook dependencies were not installed by local VitePlus: %#v", commands[3])
	}
	if commands[7].Name != filepath.Join("node_modules", ".bin", "vp") || filepath.Base(commands[7].Dir) != "web" || commands[7].Args[0] != "build" {
		t.Fatalf("initial frontend build did not use generated project VitePlus: %#v", commands[7])
	}
	for _, name := range []string{"go.mod", "server.go", "cmd/main.go", "internal/skgo/config.go", "web/dist.go", "web/build/.gitkeep"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("generated %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "web", "src", "routes", "example.remote.go")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the minimal application was given the example's Go remote functions: %v", err)
	}
	page, err := os.ReadFile(filepath.Join(dir, "web", "src", "routes", "+page.svelte"))
	if err != nil {
		t.Fatal(err)
	}
	if string(page) != "sv's page\n" {
		t.Fatalf("Go rewrote the frontend page: %q", page)
	}
	justfile, err := os.ReadFile(filepath.Join(dir, "Justfile"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(justfile), "pnpm storybook --host 127.0.0.1") {
		t.Fatalf("generated Storybook instruction does not pass its host option correctly:\n%s", justfile)
	}
}

// The developer's explicit sv options reach sv as written, their own Vitest
// and Storybook selections are not applied a second time, and sv's demo becomes
// the skgo example rather than a JavaScript server application.
func TestCreatePassesExplicitChoicesThroughAndAppliesNothingTwice(t *testing.T) {
	registry := registryServer(t, `{"versions":{"1.0.0-next.7":{}}}`)
	dir := filepath.Join(t.TempDir(), "chosen")
	u := &upstream{
		t: t,
		created: map[string]string{
			"src/routes/sverdle/+page.server.js":    "export const actions = {}",
			"src/lib/vitest-examples/greet.spec.js": "test",
			".storybook/main.js":                    "export default {}",
			"src/stories/Button.stories.svelte":     "story",
			"node_modules/.bin/vitest":              "",
			"node_modules/.bin/storybook":           "",
		},
		devDeps: map[string]string{"vitest": "^4.1.8", "tailwindcss": "^4.3.0"},
		scripts: map[string]string{"test:unit": "vitest", "storybook": "storybook dev -p 6006"},
	}
	chosen := []string{"--template", "demo", "--types", "jsdoc", "--add", "tailwindcss=plugins:none", "vitest=usages:unit", "storybook"}
	o := u.options(dir, registry)
	o.SvArgs = chosen
	result, err := Create(o)
	if err != nil {
		t.Fatal(err)
	}
	if result.Starter != "examples" {
		t.Fatalf("sv's demo did not become the skgo example: %+v", result)
	}
	if got := svArgs(t, u.create()); !slices.Equal(got, chosen) {
		t.Fatalf("sv create options = %q, want exactly the developer's %q", got, chosen)
	}
	add := strings.Join(u.commands[1].Args, " ")
	if strings.Contains(add, "vitest") {
		t.Errorf("Vitest was applied again over the developer's selection: %s", add)
	}
	if !strings.Contains(add, "@skgo/sv@0.4.0=starter:examples") {
		t.Errorf("the skgo add-on was not asked for the example: %s", add)
	}
	for _, c := range u.commands {
		if slices.Contains(c.Args, "create-storybook") {
			t.Errorf("Storybook was installed again over the developer's selection: %#v", c)
		}
	}
	readme := readFile(t, filepath.Join(dir, "README.md"))
	if !strings.Contains(readme, "installed the frontend dependencies") || strings.Contains(strings.ToLower(readme), "chromium") {
		t.Errorf("a unit-only project's README must describe the install without a browser:\n%s", readme)
	}
	if browserInstalls(u.commands) != 0 {
		t.Errorf("unit-only Vitest has no Playwright dependency, yet a browser install was asked for: %#v", u.commands)
	}
	if _, err := os.Stat(filepath.Join(dir, "web", "src", "routes", "example.remote.go")); err != nil {
		t.Errorf("the example's Go remote functions: %v", err)
	}
	pkg, err := readPackage(filepath.Join(dir, "web"))
	if err != nil {
		t.Fatal(err)
	}
	if pkg.DevDependencies["tailwindcss"] != "^4.3.0" {
		t.Errorf("the developer's tailwindcss selection did not survive: %#v", pkg.DevDependencies)
	}
}

func browserInstalls(commands []command) int {
	n := 0
	for _, c := range commands {
		if c.Name == "pnpm" && slices.Contains(c.Args, "playwright") {
			n++
		}
	}
	return n
}

// A developer who chose component testing got Playwright from sv, and its
// browser still has to be installed although Vitest is not applied again.
func TestCreateInstallsChromiumForAChosenComponentVitest(t *testing.T) {
	registry := registryServer(t, `{"versions":{"1.0.0-next.7":{}}}`)
	u := &upstream{
		t: t,
		created: map[string]string{
			"src/lib/vitest-examples/greet.spec.ts": "test",
			"node_modules/.bin/vitest":              "",
			"node_modules/.bin/playwright":          "",
		},
		devDeps: map[string]string{"vitest": "^4.1.8", "@vitest/browser-playwright": "^4.1.8", "playwright": "^1.60.0"},
		scripts: map[string]string{"test:unit": "vitest"},
	}
	o := u.options(filepath.Join(t.TempDir(), "component"), registry)
	o.SvArgs = []string{"--add", "vitest=usages:unit,component"}
	if _, err := Create(o); err != nil {
		t.Fatal(err)
	}
	if add := strings.Join(u.commands[1].Args, " "); strings.Contains(add, "vitest") {
		t.Errorf("Vitest was applied again over the developer's selection: %s", add)
	}
	if browserInstalls(u.commands) != 1 {
		t.Errorf("component Vitest needs Chromium installed exactly once: %#v", u.commands)
	}
}

// In a terminal the questions are sv's. skgo answers none of them in advance,
// because sv shows its add-on picker only when no add-on was named.
func TestCreateInATerminalLeavesEveryChoiceToSv(t *testing.T) {
	registry := registryServer(t, `{"versions":{"1.0.0-next.7":{}}}`)
	u := &upstream{t: t}
	o := u.options(filepath.Join(t.TempDir(), "asked"), registry)
	o.Interactive = true
	if _, err := Create(o); err != nil {
		t.Fatal(err)
	}
	create := u.create()
	if got := svArgs(t, create); len(got) != 0 {
		t.Fatalf("skgo answered sv's questions for the developer: %q", got)
	}
	if slices.Contains(create.Args, "--no-interactive") || slices.Contains(create.Env, "CI=1") {
		t.Fatalf("VitePlus was told not to prompt: %#v", create.Args)
	}
	if got := strings.Join(create.Args, " "); !strings.Contains(got, "--package-manager pnpm") {
		t.Fatalf("VitePlus was not told the project's package manager: %s", got)
	}

	u = &upstream{t: t}
	o = u.options(filepath.Join(t.TempDir(), "partly-asked"), registry)
	o.Interactive, o.SvArgs = true, []string{"--types", "jsdoc"}
	if _, err := Create(o); err != nil {
		t.Fatal(err)
	}
	if got, want := svArgs(t, u.create()), []string{"--types", "jsdoc"}; !slices.Equal(got, want) {
		t.Fatalf("sv create options = %q, want %q and the rest asked", got, want)
	}
}

func TestCreateRejectsWhatIsNotASkgoApplication(t *testing.T) {
	registry := registryServer(t, `{"versions":{"1.0.0-next.7":{}}}`)
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"library":    {[]string{"--template", "library"}, "library template is a package"},
		"library=":   {[]string{"--template=library"}, "library template is a package"},
		"addon":      {[]string{"--template", "addon"}, "addon template is a package"},
		"install":    {[]string{"--install", "npm"}, "VitePlus installs the project with pnpm"},
		"no install": {[]string{"--no-install"}, "VitePlus installs the project with pnpm"},
		"playground": {[]string{"--from-playground", "https://svelte.dev/playground/x"}, "--from-playground does not describe"},
		"target":     {[]string{"elsewhere", "--template", "minimal"}, "always created in web"},
	} {
		t.Run(name, func(t *testing.T) {
			u := &upstream{t: t}
			o := u.options(filepath.Join(t.TempDir(), "rejected"), registry)
			o.SvArgs = tc.args
			_, err := Create(o)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Create error = %v; want %q", err, tc.want)
			}
			if len(u.commands) != 0 {
				t.Fatalf("upstream ran before the refusal: %#v", u.commands)
			}
		})
	}

	// An interactive answer is only visible in what sv wrote.
	for name, tc := range map[string]struct {
		u    *upstream
		want string
	}{
		"library chosen at the prompt": {&upstream{devDeps: map[string]string{"@sveltejs/package": "^2.0.0"}}, "library template is a package"},
		"add-on wrote a server":        {&upstream{created: map[string]string{"src/hooks.server.ts": "", "src/lib/server/db/index.ts": ""}}, "src/hooks.server.ts, src/lib/server"},
	} {
		t.Run(name, func(t *testing.T) {
			tc.u.t = t
			dir := filepath.Join(t.TempDir(), "rejected")
			o := tc.u.options(dir, registry)
			o.Interactive = true
			_, err := Create(o)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Create error = %v; want %q", err, tc.want)
			}
			if len(tc.u.commands) != 1 {
				t.Fatalf("setup continued on a project skgo cannot serve: %#v", tc.u.commands)
			}
			if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("Go setup ran: %v", statErr)
			}
		})
	}
}

// sv exits 0 when it reaches the end of its input before applying an add-on.
func TestCreateRefusesAnSvAddThatAppliedNothing(t *testing.T) {
	registry := registryServer(t, `{"versions":{"1.0.0-next.7":{}}}`)
	u := &upstream{t: t, skipAdd: true, created: map[string]string{"src/routes/sverdle/+page.server.ts": ""}}
	dir := filepath.Join(t.TempDir(), "unapplied")
	_, err := Create(u.options(dir, registry))
	if err == nil || !strings.Contains(err.Error(), "sv did not apply the skgo integration") {
		t.Fatalf("Create error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("Go setup ran after incomplete frontend: %v", statErr)
	}
}

func TestEscapeAddonOptionRoundTripsSpacesAndPlusSigns(t *testing.T) {
	got := escapeAddonOption("file:/tmp/adapter with+plus")
	if got != "file%3A%2Ftmp%2Fadapter%20with%2Bplus" {
		t.Fatalf("escapeAddonOption = %q", got)
	}
	if strings.Contains(got, "+") {
		t.Fatalf("escaped add-on option contains sv's separator: %q", got)
	}
}

func TestCreateRefusesSwallowedStorybookFailure(t *testing.T) {
	registry := registryServer(t, `{"versions":{"1.0.0-next.7":{}}}`)
	dir := filepath.Join(t.TempDir(), "incomplete")
	u := &upstream{t: t, skipBook: true}
	_, err := Create(u.options(dir, registry))
	if err == nil || !strings.Contains(err.Error(), "Storybook") {
		t.Fatalf("Create error = %v; want clear Storybook failure", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("Go setup ran after incomplete frontend: %v", statErr)
	}
}

func TestCreateReportsVitePlusCancellation(t *testing.T) {
	registry := registryServer(t, `{"versions":{"1.0.0-next.7":{}}}`)
	dir := filepath.Join(t.TempDir(), "cancelled")
	_, err := Create(Options{
		Dir: dir, SkgoVersion: "v0.4.1", SVAddonSpec: "@skgo/sv@0.4.0", AdapterSpec: "file:/candidate/adapter",
		RegistryURL: registry.URL, Client: registry.Client(), VP: "vp-test",
		run: func(command) error { return errors.New("cancelled") },
	})
	if err == nil || !strings.Contains(err.Error(), "VitePlus project creation failed") {
		t.Fatalf("Create error = %v", err)
	}
}

func TestResolveSelectsExactIndependentCompatiblePackageVersions(t *testing.T) {
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		switch r.URL.Path {
		case "/sv":
			fmt.Fprint(w, `{"versions":{"1.0.0-next.7":{},"2.0.0":{}}}`)
		case "/@skgo/sv":
			fmt.Fprint(w, `{"versions":{"0.3.0":{},"0.4.0":{},"0.5.0":{}}}`)
		case "/@skgo/sveltekit-adapter":
			fmt.Fprint(w, `{"versions":{"0.3.7":{},"0.4.2":{},"0.5.0":{}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(registry.Close)

	p, err := resolve(Options{
		Dir: filepath.Join(t.TempDir(), "paired"), SkgoVersion: "v0.4.1",
		RegistryURL: registry.URL, Client: registry.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.SVAddonSpec != "@skgo/sv@0.4.0" {
		t.Errorf("add-on = %q; want exact newest compatible @skgo/sv version", p.SVAddonSpec)
	}
	if p.AdapterDependency != "0.3.7" {
		t.Errorf("adapter option = %q; want its independently selected exact compatible version", p.AdapterDependency)
	}
}

func registryServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		fmt.Fprint(w, body)
	}))
	t.Cleanup(server.Close)
	return server
}

// The generated project's go:embed needs one tracked file in web/build. Three
// parties must name the same file: the generator tracks it, the add-on exempts
// it from sv's ignore rule, and the adapter restores it after clearing the
// output tree. When they disagreed, the documented production build deleted a
// tracked file and the next commit produced a clone that did not compile.
func TestGeneratorAddonAndAdapterAgreeOnTheEmbedKeepFile(t *testing.T) {
	const keep = ".gitkeep"
	if _, err := goFiles.ReadFile("gofiles/web/build/dot-gitkeep"); err != nil {
		t.Errorf("generator does not track web/build/%s: %v", keep, err)
	}
	entries, err := goFiles.ReadDir("gofiles/web/build")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("generator tracks %d files in web/build, want only %s", len(entries), keep)
	}
	for file, want := range map[string]string{
		"../adapter/skgo-adapter.js": "write(`${out}/" + keep + "`, '')",
		"../sv/sv-addon.source.js":   `\n/build/*\n!/build/` + keep + `\n`,
	} {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), want) {
			t.Errorf("%s does not contain %q", file, want)
		}
	}
}

// sv installs a file: add-on as a symlink inside its own directory in the
// shared pnpm store, and extracts every later registry add-on through whatever
// is there. A link to the live checkout therefore let an ordinary `skgo new`
// overwrite tracked files in it. The checkout may be packed from, never linked.
func TestCreateHandsSvAnIsolatedPackedAddonNotTheCheckout(t *testing.T) {
	registry := registryServer(t, `{"versions":{"1.0.0-next.7":{}}}`)
	checkout := filepath.Join(t.TempDir(), "internal", "sv")
	writeFiles(t, checkout, map[string]string{"sv-addon.js": "live checkout bytes", "package.json": `{"name":"@skgo/sv"}`})
	stage := filepath.Join(t.TempDir(), "stage")
	writeFiles(t, stage, map[string]string{"package/left-by-an-earlier-run.js": "stale"})

	u := &upstream{t: t}
	runner := func(c command) error {
		switch {
		case c.Name == "pnpm" && c.Args[0] == "pack":
			if c.Dir != checkout {
				t.Errorf("pnpm pack ran in %s, want the add-on source %s", c.Dir, checkout)
			}
			destination := c.Args[slices.Index(c.Args, "--pack-destination")+1]
			writeTarball(t, filepath.Join(destination, "skgo-sv-0.0.0-dev.tgz"), map[string]string{
				"package/sv-addon.js":  "packed bytes",
				"package/package.json": `{"name":"@skgo/sv","version":"0.0.0-dev"}`,
			})
		}
		return u.run(c)
	}
	_, err := Create(Options{
		Dir: filepath.Join(t.TempDir(), "qualified"), SkgoVersion: "v0.4.1",
		SVAddonSpec: "file:" + checkout, AdapterSpec: "0.3.7",
		RegistryURL: registry.URL, Client: registry.Client(), VP: "vp-test", run: runner, addonStage: stage,
	})
	if err != nil {
		t.Fatal(err)
	}

	commands := u.commands
	var linked string
	for _, c := range commands {
		for _, arg := range c.Args {
			if spec, _, ok := strings.Cut(arg, "=starter:"); ok {
				linked = strings.TrimPrefix(spec, "file:")
			}
		}
	}
	want := filepath.Join(stage, "package")
	if linked != want {
		t.Fatalf("sv was handed %q, want the staged package %q", linked, want)
	}
	if got, err := os.ReadFile(filepath.Join(linked, "sv-addon.js")); err != nil || string(got) != "packed bytes" {
		t.Fatalf("staged add-on = %q, %v; want the bytes pnpm packed", got, err)
	}
	if _, err := os.Stat(filepath.Join(linked, "left-by-an-earlier-run.js")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("an earlier qualification's file survived into this stage: %v", err)
	}
	installed := slices.ContainsFunc(commands, func(c command) bool {
		return c.Name == "pnpm" && c.Dir == want && slices.Equal(c.Args, []string{"install", "--prod", "--ignore-workspace"})
	})
	if !installed {
		t.Errorf("the staged add-on was not given its own sv peer: %#v", commands)
	}

	// What sv does on the next registry run: extract the published package
	// through its link. The checkout must not be where that lands.
	if err := os.WriteFile(filepath.Join(linked, "sv-addon.js"), []byte("published bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(checkout, "sv-addon.js")); err != nil || string(got) != "live checkout bytes" {
		t.Fatalf("checkout add-on = %q, %v; a write through sv's link reached the checkout", got, err)
	}
}

// A clone of a generated project has no node_modules and no adapter build,
// and the Go dev server reads the adapter manifest at startup. The documented
// first command has to produce both, once.
func TestGeneratedDevRecipeBootstrapsAFreshCloneOnce(t *testing.T) {
	just, err := exec.LookPath("just")
	if err != nil {
		t.Fatalf("the generated Justfile cannot be exercised without just: %v", err)
	}
	root := t.TempDir()
	clone := filepath.Join(root, "clone")
	if err := writeGoFiles(project{Dir: clone, App: "clone", Module: "clone", Origin: defaultOrigin, OriginHostPort: "127.0.0.1:8080", GoVersion: "1.25", BindingsImport: "clone/internal/skgo", SkgoVersion: "v0.4.1"}); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(root, "log")
	bin := filepath.Join(root, "bin")
	vp := "#!/bin/sh\necho \"vp $1\" >> " + log + "\nif [ \"$1\" = build ]; then mkdir -p build && echo '{}' > build/skgo.manifest.json; fi\n"
	writeFiles(t, bin, map[string]string{
		// go run stands in for the server: it returns once vite has started.
		"go":   "#!/bin/sh\necho \"go $1\" >> " + log + "\nif [ \"$1\" = run ]; then for i in 1 2 3 4 5 6 7 8 9 10; do grep -q 'vp dev' " + log + " && exit 0; sleep 0.2; done; exit 1; fi\n",
		"pnpm": "#!/bin/sh\necho \"pnpm $*\" >> " + log + "\nmkdir -p node_modules/.bin\ncat > node_modules/.bin/vp <<'VP'\n" + vp + "VP\nchmod +x node_modules/.bin/vp\n",
	})
	for _, name := range []string{"go", "pnpm"} {
		if err := os.Chmod(filepath.Join(bin, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	dev := func() []string {
		t.Helper()
		if err := os.RemoveAll(log); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(just, "dev")
		cmd.Dir = clone
		cmd.Env = []string{"PATH=" + bin + ":/usr/bin:/bin", "HOME=" + root}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("just dev: %v\n%s", err, out)
		}
		body, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Split(strings.TrimSpace(string(body)), "\n")
	}

	first := dev()
	slices.Sort(first[3:]) // vite runs beside the server; their order is not the contract
	if want := []string{"go generate", "pnpm install --frozen-lockfile", "vp build", "go run", "vp dev"}; !slices.Equal(first, want) {
		t.Fatalf("first start in a fresh clone ran %q, want %q", first, want)
	}
	second := dev()
	slices.Sort(second[1:])
	if want := []string{"go generate", "go run", "vp dev"}; !slices.Equal(second, want) {
		t.Fatalf("second start ran %q, want %q: nothing was missing, so nothing is reinstalled or rebuilt", second, want)
	}
}

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func writeTarball(t *testing.T, name string, files map[string]string) {
	t.Helper()
	var buffer bytes.Buffer
	zw := gzip.NewWriter(&buffer)
	tw := tar.NewWriter(zw)
	for file, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: file, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, buffer.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
