//go:build skgo_browser_experiment

package hotreload

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/tylergannon/devalue/v5"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tylergannon/skgo/generate"
	"golang.org/x/tools/go/packages"
)

// Diagnostic copy of Compile's stages, intentionally separate from production.
// Used to identify the boundary hidden by the browser fixture's compile timer.
func TestNativeCompileStageTimingExperiment(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
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
	write("web/src/routes/orders/orders.remote.go", []byte(orderSource))
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
	write("cmd/main.go", []byte(strings.ReplaceAll(browserHost, "PACKAGE", pkg)))
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

	for _, fullGraph := range []bool{true, false} {
		t.Logf("fullGraph=%v", fullGraph)
		for iteration := 1; iteration <= 3; iteration++ {
			changed := map[string][]byte{}
			for file, raw := range baseline {
				changed[file] = raw
			}
			changed[filepath.Join(dir, "orders.remote.go")] = []byte(strings.Replace(orderSource, "return total", fmt.Sprintf("return total*0+%d", iteration*1000), 1))
			candidate, err := timedNativeCompile(t, ctx, tc, app, pkg, filepath.Join(app, "importcfg"), filepath.Join(app, fmt.Sprintf("revision-%v-%d", fullGraph, iteration)), baseline, changed, fullGraph)
			if err != nil {
				t.Fatal(err)
			}
			installStart := time.Now()
			if err = enc.Encode(candidate); err != nil {
				t.Fatal(err)
			}
			var installed struct {
				PID   int
				Error string
			}
			if err = dec.Decode(&installed); err != nil || installed.Error != "" {
				t.Fatalf("install: %v %s", err, installed.Error)
			}
			if installed.PID != ready.PID {
				t.Fatal("changed host PID")
			}
			t.Logf("iteration=%d install=%s pid=%d", iteration, time.Since(installStart), installed.PID)
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
			tree, err := devalue.Parse(envelope.Data, nil)
			if err != nil {
				t.Fatal(err)
			}
			value, _ := tree.(*devalue.Object).Get("_")
			total, _ := value.(*devalue.Object).Get("total")
			if total != float64(iteration*1000) {
				t.Fatalf("remote response: %s", raw)
			}
			t.Logf("fullGraph=%v literalHTTP=%v", fullGraph, total)
		}
	}
}

func timedNativeCompile(t *testing.T, ctx context.Context, tc Toolchain, dir, pkgPath, importcfg, output string, before, after map[string][]byte, fullGraph bool) (Candidate, error) {
	t.Helper()
	var candidate Candidate
	beginning := time.Now()
	stage := beginning
	note := func(name string) { now := time.Now(); t.Logf("stage=%s elapsed=%s", name, now.Sub(stage)); stage = now }
	if err := Classify(before, after, nil, nil); err != nil {
		return candidate, err
	}
	note("source-classify")
	mode := packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedModule
	if fullGraph {
		mode |= packages.NeedDeps
	}
	pkgs, err := packages.Load(&packages.Config{
		Context: ctx, Dir: dir, Env: tc.command(ctx, dir).Env, Overlay: after,
		Mode: mode,
		Logf: func(format string, args ...any) { t.Logf("go/packages: "+format, args...) },
	}, pkgPath)
	note("packages.Load")
	if err != nil {
		return candidate, err
	}
	if len(pkgs) != 1 {
		return candidate, fmt.Errorf("expected one package: %d", len(pkgs))
	}
	p := pkgs[0]
	if err = Classify(before, after, []*packages.Package{p}, []*packages.Package{p}); err != nil {
		return candidate, err
	}
	note("typed-classify")
	lowered, err := Lower(p, after)
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
	t.Logf("stage=total elapsed=%s", time.Since(beginning))
	packageCount, syntaxCount := 0, 0
	packages.Visit(pkgs, func(p *packages.Package) bool { packageCount++; syntaxCount += len(p.Syntax); return true }, nil)
	t.Logf("fullGraph=%v packages=%d syntaxFiles=%d rootSyntaxFiles=%d", fullGraph, packageCount, syntaxCount, len(p.Syntax))
	return candidate, nil
}
