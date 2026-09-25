package example_test

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// The frontend type checks here and the generator check in generate_test.go
// share three sandboxes, each a copy of this module built once and read by
// every test that needs its state. `go generate` and `svelte-check` are the
// expensive steps, so each runs once per distinct source state and never
// again:
//
//   - regenerated: the untouched module, regenerated. TestNothingGeneratedWasWrittenByHand
//     compares it with the tree; TestGeneratedActionTypesRejectWrongUses reads
//     the action stub out of it.
//   - checked: the untouched module, type-checked. It is the passing baseline
//     both type-check tests start from: the check has to be able to fail.
//   - drifted: the module with every wrong use this file plants planted at
//     once — the Go wire field renamed and regenerated, and each suppressed
//     action-type error unsuppressed — type-checked once. Each diagnostic is
//     in its own file and line, and svelte-check reports every error in every
//     file, so one run answers for all of them.
//
// The three start together the first time any test asks for one.
//
// Everything happens in throwaway copies of this module. The tests used to
// edit the developer's own source and put it back in t.Cleanup, which meant a
// SIGKILL, a `-timeout` abort or a panic in the restore path left the checkout
// holding a type nobody wrote.

// actionTypeDirectives are the `@ts-expect-error` lines in
// action-types.check.ts, each guarding one wrong use of the generated action
// contract, and the diagnostic TypeScript gives once the directive is gone.
var actionTypeDirectives = []struct{ label, directive, diagnostic string }{
	{"success", "// @ts-expect-error validation fields exist only in failure data\n", "Property 'emailError' does not exist"},
	{"failure", "// @ts-expect-error receipts exist only in success data\n", "Property 'receipt' does not exist"},
}

const actionTypesCheck = "web/src/routes/actions/action-types.check.ts"

type generatedSandbox struct {
	app      string
	setupErr error
	out      string
	err      error
}

type checkedSandbox struct {
	setupErr error
	out      string
	err      error
}

type driftedSandbox struct {
	setupErr    error
	generateOut string
	generateErr error
	// types is the generated TypeScript for businesslogic after the rename.
	types []byte
	// missingDirectives lists directives action-types.check.ts no longer
	// carries, so nothing was unsuppressed for them.
	missingDirectives []string
	checkOut          string
	checkErr          error
}

var startSandboxes = sync.OnceFunc(func() {
	go regenerated()
	go checked()
	go drifted()
})

var regenerated = sync.OnceValue(func() generatedSandbox {
	app := filepath.Join(packageTemp, "generated")
	if err := newSandbox(app); err != nil {
		return generatedSandbox{setupErr: err}
	}
	out, err := generate(app)
	return generatedSandbox{app: app, out: out, err: err}
})

var checked = sync.OnceValue(func() checkedSandbox {
	app := filepath.Join(packageTemp, "checked")
	if err := newSandbox(app); err != nil {
		return checkedSandbox{setupErr: err}
	}
	out, err := svelteCheck(app)
	return checkedSandbox{out: out, err: err}
})

var drifted = sync.OnceValue(func() driftedSandbox {
	app := filepath.Join(packageTemp, "drifted")
	if err := newSandbox(app); err != nil {
		return driftedSandbox{setupErr: err}
	}
	var d driftedSandbox

	// The json tag of businesslogic.Todo.Text is renamed on the wire.
	source := filepath.Join(app, "businesslogic", "store.go")
	original, err := os.ReadFile(source)
	if err != nil {
		return driftedSandbox{setupErr: err}
	}
	const tag = "`json:\"text\"`"
	if !bytes.Contains(original, []byte(tag)) {
		return driftedSandbox{setupErr: fmt.Errorf("businesslogic/store.go no longer contains %s; update this test", tag)}
	}
	if err := os.WriteFile(source, bytes.Replace(original, []byte(tag), []byte("`json:\"label\"`"), 1), 0o644); err != nil {
		return driftedSandbox{setupErr: err}
	}
	d.generateOut, d.generateErr = generate(app)
	if d.generateErr != nil {
		return d
	}
	if d.types, err = os.ReadFile(filepath.Join(app, "web", "src", "lib", "skgo", "businesslogic", "types.ts")); err != nil {
		d.setupErr = fmt.Errorf("reading the generated types: %w", err)
		return d
	}

	// Every action-type directive is removed.
	path := filepath.Join(app, filepath.FromSlash(actionTypesCheck))
	checks, err := os.ReadFile(path)
	if err != nil {
		d.setupErr = err
		return d
	}
	for _, tc := range actionTypeDirectives {
		if !bytes.Contains(checks, []byte(tc.directive)) {
			d.missingDirectives = append(d.missingDirectives, tc.directive)
			continue
		}
		checks = bytes.Replace(checks, []byte(tc.directive), nil, 1)
	}
	if err := os.WriteFile(path, checks, 0o644); err != nil {
		d.setupErr = err
		return d
	}
	d.checkOut, d.checkErr = svelteCheck(app)
	return d
})

// TestChangingAGoTypeBreaksTheComponentThatUsesIt is the end-to-end types
// claim, checked rather than asserted.
//
// It renames one field on the wire — the json tag of businesslogic.Todo.Text —
// regenerates, and requires the SvelteKit type check to fail naming the
// component that reads the old field. Without generated TypeScript coming from
// the Go type, the change would sail through to the browser and show up as an
// empty todo.
func TestChangingAGoTypeBreaksTheComponentThatUsesIt(t *testing.T) {
	t.Parallel()
	requireFrontendToolchain(t)
	startSandboxes()

	// The check has to be able to fail, so start from a passing state.
	base := checked()
	if base.setupErr != nil {
		t.Fatal(base.setupErr)
	}
	if base.err != nil {
		t.Fatalf("the app does not type-check before the change: %v\n%s", base.err, base.out)
	}

	d := drifted()
	if d.setupErr != nil {
		t.Fatal(d.setupErr)
	}
	if d.generateErr != nil {
		t.Fatalf("regenerating after the change: %v\n%s", d.generateErr, d.generateOut)
	}
	if !bytes.Contains(d.types, []byte(`"label": string`)) {
		t.Fatalf("the generated TypeScript did not follow the Go type:\n%s", d.types)
	}
	if d.checkErr == nil {
		t.Fatal("the app still type-checks after the Go type changed; the components are not typed by Go")
	}
	if !diagnosed(d.checkOut, "TodoList.svelte", "'text'") {
		t.Fatalf("the type check failed, but not on the component that reads the renamed field:\n%s", d.checkOut)
	}
	t.Logf("caught before the browser:\n%s", d.checkOut)
}

// Each suppressed error is proved by removing its directive. This proves the
// annotations guard real type errors in the generated action contract, while
// the passing baseline checks the valid success, failure and Money uses
// beside them. Each diagnostic has to be reported against the check file, on
// its own, so one type check with every directive removed answers for each.
func TestGeneratedActionTypesRejectWrongUses(t *testing.T) {
	t.Parallel()
	requireFrontendToolchain(t)
	startSandboxes()

	g := regenerated()
	if g.setupErr != nil {
		t.Fatal(g.setupErr)
	}
	if g.err != nil {
		t.Fatalf("generating action types: %v\n%s", g.err, g.out)
	}
	stub, err := os.ReadFile(filepath.Join(g.app, "web", "src", "routes", "actions", "+page.server.ts"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ActionFailure<", "price: Money", "throw new Error('skgo: action implemented in Go')"} {
		if !bytes.Contains(stub, []byte(want)) {
			t.Fatalf("generated action export lacks %q:\n%s", want, stub)
		}
	}

	base := checked()
	if base.setupErr != nil {
		t.Fatal(base.setupErr)
	}
	if base.err != nil {
		t.Fatalf("valid generated action types did not compile: %v\n%s", base.err, base.out)
	}

	d := drifted()
	if d.setupErr != nil {
		t.Fatal(d.setupErr)
	}
	for _, tc := range actionTypeDirectives {
		t.Run(tc.label, func(t *testing.T) {
			for _, missing := range d.missingDirectives {
				if missing == tc.directive {
					t.Fatalf("missing %q", tc.directive)
				}
			}
			if d.checkErr == nil || !diagnosed(d.checkOut, filepath.Base(actionTypesCheck), tc.diagnostic) {
				t.Fatalf("wrong %s use was not rejected for the intended reason: %v\n%s", tc.label, d.checkErr, d.checkOut)
			}
		})
	}
}

// diagnosed reports whether svelte-check's human output carries an error
// against file whose message contains message. The human format writes each
// diagnostic as its location on one line and "Error: <message>" on the next,
// so both have to be in the same diagnostic, not merely somewhere in the run.
func diagnosed(out, file, message string) bool {
	lines := strings.Split(out, "\n")
	for i := 0; i+1 < len(lines); i++ {
		if strings.Contains(lines[i], file) && strings.HasPrefix(strings.TrimSpace(lines[i+1]), "Error") && strings.Contains(lines[i+1], message) {
			return true
		}
	}
	return false
}

// generate runs the app's own `go generate` inside a sandbox.
//
// GOWORK is off because the sandbox is not part of the workspace; the copied
// go.mod carries an absolute replace back to the real skgo module instead.
func generate(app string) (string, error) {
	cmd := exec.Command("go", "generate", "./...")
	cmd.Dir = filepath.Join(app, "internal", "skgo")
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// svelteCheck runs the app's own type check, which is what `svelte-check`
// exists for: TypeScript alone cannot read a `.svelte` file.
func svelteCheck(app string) (string, error) {
	cmd := exec.Command("mise", "x", "--",
		"node", filepath.Join("node_modules", "svelte-check", "bin", "svelte-check"),
		"--tsconfig", "./tsconfig.json", "--output", "human")
	cmd.Dir = filepath.Join(app, "web")
	// svelte-check colours its file names whenever CI is set, and the
	// diagnostics are matched against plain paths.
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// newSandbox copies this module into app, so a test that has to mutate
// application source can do so without touching the tree the developer is
// working in.
func newSandbox(app string) error {
	root, err := filepath.Abs("..")
	if err != nil {
		return fmt.Errorf("locating the repository root: %w", err)
	}

	// node_modules is 166MB of pnpm store links and e2e is a second browser
	// install; neither is an input to what these tests change. .svelte-kit's
	// build output is regenerated by vite, not read by svelte-check.
	skip := map[string]bool{
		filepath.Join("web", "node_modules"):          true,
		filepath.Join("web", ".svelte-kit", "output"): true,
		"e2e": true,
		"tmp": true,
	}
	if err := copyTree(".", app, skip); err != nil {
		return fmt.Errorf("copying the module into %s: %w", app, err)
	}
	if err := linkNodeModules(filepath.Join("web", "node_modules"), filepath.Join(app, "web", "node_modules")); err != nil {
		return fmt.Errorf("linking node_modules into the sandbox: %w", err)
	}
	if err := absoluteReplace(filepath.Join(app, "go.mod"), root); err != nil {
		return fmt.Errorf("rewriting the sandbox go.mod: %w", err)
	}
	return nil
}

// copyTree copies src to dst, skipping the src-relative paths in skip.
func copyTree(src, dst string, skip map[string]bool) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if skip[rel] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// linkNodeModules gives the sandbox its own node_modules whose entries point
// at the installed ones.
//
// It cannot be a single symlink to the whole directory. `node_modules/$app` is
// written by kit and holds the tsconfig every `.svelte` file inherits, whose
// `rootDirs` are relative — resolved through a symlink they name the original
// checkout, and svelte-check then type-checks the sandbox's components against
// the original's generated types. So $app is copied and everything else,
// which is position-independent, is linked.
func linkNodeModules(src, dst string) error {
	absSrc, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(absSrc)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == "$app" {
			if err := copyTree(filepath.Join(absSrc, "$app"), filepath.Join(dst, "$app"), nil); err != nil {
				return err
			}
			continue
		}
		if err := os.Symlink(filepath.Join(absSrc, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

// absoluteReplace points the sandbox's `replace` directive back at the real
// skgo module, which is no longer one directory up.
func absoluteReplace(gomod, root string) error {
	raw, err := os.ReadFile(gomod)
	if err != nil {
		return err
	}
	const relative = "replace github.com/tylergannon/skgo => ../"
	if !bytes.Contains(raw, []byte(relative)) {
		return fmt.Errorf("%s no longer contains %q; update this test", gomod, relative)
	}
	rewritten := bytes.Replace(raw, []byte(relative), []byte("replace github.com/tylergannon/skgo => "+root), 1)
	return os.WriteFile(gomod, rewritten, 0o644)
}

// requireFrontendToolchain fails when the JavaScript side is not installed.
//
// It used to skip. `go test ./...` then printed ok on CI and on every fresh
// clone while the only check that a Go type reaches TypeScript did not run at
// all, and nothing in the output said so.
func requireFrontendToolchain(t *testing.T) {
	t.Helper()
	for _, path := range []string{
		filepath.Join("web", "node_modules", "svelte-check"),
		filepath.Join("web", "node_modules", "$app", "tsconfig.json"),
		filepath.Join("web", ".svelte-kit", "types"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s is missing, so the Go-to-TypeScript check cannot run. Run `mise x -- vp install` and `mise x -- vp build` in example/web.", path)
		}
	}
	if _, err := exec.LookPath("mise"); err != nil {
		t.Fatal("mise is not installed, so the Go-to-TypeScript check cannot run")
	}
}
