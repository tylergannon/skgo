//go:build skgo_architecture_experiment && skgo_browser_experiment

package hotreload

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/tylergannon/devalue/v5"
	"go/types"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tylergannon/polytype/goload"
	"github.com/tylergannon/skgo/generate"
	"github.com/tylergannon/skgo/internal/gen"
	"golang.org/x/tools/go/packages"
)

// Diagnostic copy of Compile's stages, intentionally separate from production.
// Used to identify the boundary hidden by the browser fixture's compile timer.
func TestArchitectureGoPreparation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	t.Setenv("GOWORK", "off")
	t.Setenv("CGO_ENABLED", "0")
	root, _ := filepath.Abs("../..")
	work := t.TempDir()
	work, _ = filepath.EvalSymlinks(work)
	app := filepath.Join(work, "application")
	fixture := filepath.Join(root, "ephemeral/experiments/20261015-vite-cooperation/browser")
	write := func(name string, b []byte) {
		t.Helper()
		path := filepath.Join(app, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, b, 0644); err != nil {
			t.Fatal(err)
		}
	}
	read := func(path string) []byte {
		t.Helper()
		b, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	write("go.mod", []byte(strings.ReplaceAll(strings.ReplaceAll(string(read(filepath.Join(root, "example/go.mod"))), "module github.com/tylergannon/skgo/example", "module example.com/nativefixture"), "=> ../", "=> "+filepath.ToSlash(root))))
	write("go.sum", read(filepath.Join(root, "go.sum")))
	write("internal/app/locals.go", []byte("package app\ntype Locals struct{}\n"))
	write("web/package.json", read(filepath.Join(root, "example/web/package.json")))
	write("web/src/app.html", read(filepath.Join(root, "example/web/src/app.html")))
	write("web/vite.config.ts", read(filepath.Join(fixture, "vite.config.ts")))
	write("web/src/routes/orders/+page.svelte", read(filepath.Join(fixture, "page.svelte")))
	write("web/src/routes/+layout.ts", []byte("export const ssr=false;\n"))
	source := strings.Replace(orderSource, `import("context";`, `import("context";"example.com/nativefixture/dependency";`, 1)
	source = strings.Replace(source, "return total", "return total+dependency.Offset", 1)
	write("dependency/value.go", []byte("package dependency\nconst Offset = 0\n"))
	write("web/src/routes/orders/orders.remote.go", []byte(source))
	if err := os.Symlink(filepath.Join(root, "example/web/node_modules"), filepath.Join(app, "web/node_modules")); err != nil {
		t.Fatal(err)
	}
	tc, err := PrepareToolchain(ctx, "go", filepath.Join(work, "toolchain"))
	if err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) []byte {
		t.Helper()
		raw, err := tc.command(ctx, app, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("go %v: %v\n%s", args, err, raw)
		}
		return raw
	}
	run("mod", "download")
	prepared, err := generate.Prepare(generate.Config{Web: filepath.Join(app, "web"), Out: filepath.Join(app, "generated"), LocalsPackage: "example.com/nativefixture/internal/app", LocalsType: "Locals", CacheDir: "off"})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	if err = prepared.Run(); err != nil {
		t.Fatal(err)
	}
	links, err := os.ReadDir(filepath.Join(app, "generated/links"))
	if err != nil || len(links) != 1 {
		t.Fatalf("links: %v %v", links, err)
	}
	rel := filepath.Join("generated/links", links[0].Name())
	dir := filepath.Join(app, rel)
	pkg := "example.com/nativefixture/" + filepath.ToSlash(rel)
	run("mod", "tidy")
	pp, err := packages.Load(&packages.Config{Context: ctx, Dir: app, Env: tc.command(ctx, app).Env, Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports | packages.NeedDeps | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedModule}, pkg)
	if err != nil || len(pp) != 1 {
		t.Fatalf("load: %v %v", pp, err)
	}
	sources := map[string][]byte{}
	for _, file := range pp[0].CompiledGoFiles {
		sources[file] = read(file)
	}
	lowered, err := Lower(pp[0], sources)
	if err != nil {
		t.Fatal(err)
	}
	for name, raw := range lowered.Host {
		write(filepath.Join(rel, name), raw)
	}
	write("cmd/main.go", []byte(strings.ReplaceAll(strings.Replace(browserHost, "SkgoHotInit(8)", "SkgoHotInit(32)", 1), "PACKAGE", pkg)))
	if _, err = tc.PrepareLoader(ctx, app, filepath.Join(work, "native")); err != nil {
		t.Fatal(err)
	}
	run("mod", "tidy")
	bin := filepath.Join(app, "host")
	run("build", "-ldflags=-s=false", "-o", bin, "./cmd")
	write("importcfg", run("list", "-export", "-deps", "-f", "{{if .Export}}packagefile {{.ImportPath}}={{.Export}}{{end}}", pkg))
	baseline := map[string][]byte{}
	for file, raw := range sources {
		baseline[file] = raw
	}
	baseline[filepath.Join(dir, "skgo_hot.go")] = []byte("package orders\n")
	host := exec.CommandContext(ctx, bin)
	host.Dir = app
	in, _ := host.StdinPipe()
	out, _ := host.StdoutPipe()
	host.Stderr = os.Stderr
	if err = host.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { in.Close(); host.Process.Kill(); host.Wait() }()
	dec, enc := json.NewDecoder(out), json.NewEncoder(in)
	var ready struct {
		URL string
		PID int
	}
	if err = dec.Decode(&ready); err != nil {
		t.Fatal(err)
	}

	fetch := func(expected int) {
		t.Helper()
		res, err := http.Get(ready.URL + "/_app/remote/1fvm1sx/order")
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		var envelope struct{ Type, Data string }
		if err = json.Unmarshal(raw, &envelope); err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != 200 || envelope.Type != "result" {
			t.Fatalf("HTTP %d: %s", res.StatusCode, raw)
		}
		tree, err := devalue.Parse(envelope.Data, nil)
		if err != nil {
			t.Fatal(err)
		}
		value, _ := tree.(*devalue.Object).Get("_")
		total, _ := value.(*devalue.Object).Get("total")
		if total != float64(expected) {
			t.Fatalf("want %d: %s", expected, raw)
		}
		t.Logf("literalHTTP=%d", expected)
	}
	install := func(c Candidate, expected int) {
		t.Helper()
		start := time.Now()
		if err := enc.Encode(c); err != nil {
			t.Fatal(err)
		}
		var installed struct {
			PID   int
			Error string
		}
		if err := dec.Decode(&installed); err != nil || installed.Error != "" {
			t.Fatalf("install: %v %s", err, installed.Error)
		}
		if installed.PID != ready.PID {
			t.Fatal("changed host PID")
		}
		t.Logf("stage=install elapsed=%s pid=%d", time.Since(start), installed.PID)
		fetch(expected)
	}
	changed := func(n int) map[string][]byte {
		next := map[string][]byte{}
		for path, raw := range baseline {
			next[path] = raw
		}
		next[filepath.Join(dir, "orders.remote.go")] = []byte(strings.Replace(source, "return total+dependency.Offset", fmt.Sprintf("return total*0+dependency.Offset+%d", n), 1))
		return next
	}
	var session goload.Session
	session.SetMaxUniverses(3)
	// The fixture tree is private. Only captured Go/control inputs below change
	// during view ownership; module dependencies and this checkout stay immutable.
	viewFiles := map[string][]byte{}
	if err := filepath.WalkDir(app, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") || d.Name() == "go.mod" || d.Name() == "go.sum" {
			viewFiles[path] = read(path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for path, raw := range baseline {
		viewFiles[path] = raw
	}
	var viewSession goload.Session
	viewSession.SetMaxUniverses(3)
	view := viewSession.PrepareView(viewFiles)
	defer view.Close()
	modes := []string{"baseline-full-overlay", "exports-root", "retained-root", "retained-view"}
	for _, mode := range modes {
		for iteration := 1; iteration <= 3; iteration++ {
			t.Logf("SAMPLE mode=%s iteration=%d", mode, iteration)
			c, err := architectureCompile(t, ctx, tc, app, pkg, filepath.Join(app, "importcfg"), filepath.Join(work, fmt.Sprintf("%s-%d", mode, iteration)), baseline, changed(iteration*1000), mode, &session, view, viewFiles, 0)
			if err != nil {
				t.Fatal(err)
			}
			install(c, iteration*1000)
		}
		// A malformed body never publishes. The repaired edit is a fourth sample.
		broken := changed(9999)
		broken[filepath.Join(dir, "orders.remote.go")] = []byte(strings.Replace(string(broken[filepath.Join(dir, "orders.remote.go")]), "return total*0", "return (", 1))
		if _, err := architectureCompile(t, ctx, tc, app, pkg, filepath.Join(app, "importcfg"), filepath.Join(work, mode+"-broken"), baseline, broken, mode, &session, view, viewFiles, 0); err == nil {
			t.Fatal("syntax failure accepted")
		} else {
			t.Logf("CONTROL mode=%s syntax-rejected=%v", mode, err)
		}
		fetch(3000)
		t.Logf("REPAIR mode=%s", mode)
		c, err := architectureCompile(t, ctx, tc, app, pkg, filepath.Join(app, "importcfg"), filepath.Join(work, mode+"-repair"), baseline, changed(4000), mode, &session, view, viewFiles, 0)
		if err != nil {
			t.Fatal(err)
		}
		install(c, 4000)
	}
	// Dependency control: changing an imported constant must invalidate exported
	// types and compiler archives. This is a separate dependency edit, not a warm
	// body sample. The private test fixture is the captured materialized view.
	write("dependency/value.go", []byte("package dependency\nconst Offset = 77\n"))
	viewFiles[filepath.Join(app, "dependency/value.go")] = read(filepath.Join(app, "dependency/value.go"))
	dependencyStart := time.Now()
	write("importcfg", run("list", "-export", "-deps", "-f", "{{if .Export}}packagefile {{.ImportPath}}={{.Export}}{{end}}", pkg))
	t.Logf("CONTROL stage=dependency-archive-refresh elapsed=%s", time.Since(dependencyStart))
	for _, mode := range modes {
		t.Logf("DEPENDENCY mode=%s", mode)
		c, err := architectureCompile(t, ctx, tc, app, pkg, filepath.Join(app, "importcfg"), filepath.Join(work, mode+"-dependency"), baseline, changed(5000), mode, &session, view, viewFiles, 77)
		if err != nil {
			t.Fatal(err)
		}
		install(c, 5077)
	}
	t.Logf("SESSION stats=%+v", session.Stats())
	t.Logf("VIEW stats=%+v", viewSession.Stats())
}

func architectureCompile(t *testing.T, ctx context.Context, tc Toolchain, dir, pkgPath, importcfg, output string, before, after map[string][]byte, modeName string, session *goload.Session, view *goload.View, viewFiles map[string][]byte, expectedDependency int) (Candidate, error) {
	t.Helper()
	var candidate Candidate
	beginning := time.Now()
	stage := beginning
	note := func(name string) { now := time.Now(); t.Logf("stage=%s elapsed=%s", name, now.Sub(stage)); stage = now }
	if err := tc.checkFlags(); err != nil {
		return candidate, err
	}
	if err := Classify(before, after, nil, nil); err != nil {
		return candidate, err
	}
	note("source-classify")
	mode := packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedModule
	if modeName == "baseline-full-overlay" {
		mode |= packages.NeedDeps
	}

	cfg := &packages.Config{Context: ctx, Dir: dir, Env: tc.command(ctx, dir).Env, Overlay: after, Mode: mode}
	var pkgs []*packages.Package
	var err error
	switch modeName {
	case "baseline-full-overlay":
		pkgs, err = packages.Load(cfg, pkgPath)
	case "exports-root":
		pkgs, err = gen.ArchitectureLoadExports(cfg, pkgPath)
	case "retained-root":
		pkgs, err = session.LoadWithFallback(cfg, gen.ArchitectureLoadExports, pkgPath)
	case "retained-view":
		capture := map[string][]byte{}
		for path, raw := range viewFiles {
			capture[path] = raw
		}
		for path, raw := range after {
			capture[path] = raw
		}
		if err = view.Update(capture); err == nil {
			pkgs, err = view.LoadWithFallback(cfg, gen.ArchitectureLoadExports, pkgPath)
		}
	default:
		return candidate, fmt.Errorf("unknown mode %q", modeName)
	}

	note("packages.Load")
	if err != nil {
		return candidate, err
	}
	if len(pkgs) != 1 {
		return candidate, fmt.Errorf("expected one package: %d", len(pkgs))
	}
	p := pkgs[0]
	dependencyFound := false
	for _, dependency := range p.Types.Imports() {
		if dependency.Path() == "example.com/nativefixture/dependency" {
			value, ok := dependency.Scope().Lookup("Offset").(*types.Const)
			if !ok || value.Val().ExactString() != fmt.Sprint(expectedDependency) {
				return candidate, fmt.Errorf("dependency analysis wanted literal %d, got %v", expectedDependency, value)
			}
			dependencyFound = true
		}
	}
	if !dependencyFound {
		return candidate, fmt.Errorf("dependency analysis missing")
	}
	t.Logf("dependencyAnalysisLiteral=%d", expectedDependency)
	if len(p.Errors) > 0 {
		return candidate, fmt.Errorf("package errors: %v", p.Errors)
	}
	var application []*packages.Package
	packages.Visit(pkgs, nil, func(q *packages.Package) { application = append(application, q) })
	if err = Classify(before, after, []*packages.Package{p}, application); err != nil {
		return candidate, err
	}
	note("typed-classify")
	lowered, err := LowerAuthored(p, after, tc.AuthoredPaths)
	if err != nil {
		return candidate, err
	}
	note("Lower")
	if err = os.MkdirAll(output, 0755); err != nil {
		return candidate, err
	}
	var names, files []string
	for name := range lowered.Candidate {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		file := filepath.Join(output, name)
		if err = os.WriteFile(file, lowered.Candidate[name], 0644); err != nil {
			return candidate, err
		}
		files = append(files, file)
	}
	candidate.Object = filepath.Join(output, "candidate.o")
	candidate.Storage = lowered.Storage
	note("candidate-file-write")
	candidate.Reclaim = numericRevision(p, after)
	note("numericRevision")
	args := append([]string{"tool", "compile", "-p", pkgPath, "-lang=go1.27", "-importcfg", importcfg, "-o", candidate.Object}, files...)
	raw, err := tc.command(ctx, dir, args...).CombinedOutput()
	if err != nil {
		return Candidate{}, fmt.Errorf("native compiler: %w\n%s", err, raw)
	}
	note("go-tool-compile")
	if err := tc.writeArtifact(candidate.Object, pkgPath, importcfg); err != nil {
		return Candidate{}, err
	}
	note("artifact-identity")
	t.Logf("stage=total elapsed=%s", time.Since(beginning))
	packageCount, syntaxCount := 0, 0
	packages.Visit(pkgs, func(p *packages.Package) bool { packageCount++; syntaxCount += len(p.Syntax); return true }, nil)
	t.Logf("mode=%s packages=%d syntaxFiles=%d rootSyntaxFiles=%d", modeName, packageCount, syntaxCount, len(p.Syntax))
	return candidate, nil
}
