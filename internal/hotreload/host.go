package hotreload

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tylergannon/skgo/internal/gen"
	"golang.org/x/tools/go/packages"
)

// HostOptions binds a captured application to a private native HTTP process.
// Inputs remains caller-owned and must live until Host.Close. The application
// constructor is used intact; its FetchConfig only gains native request context.
type HostOptions struct {
	Inputs                                                            *gen.CompilerInputs
	WorkDir, GoTool, Origin, DistDir, HandlerPackage, HandlerFunction string
	Output                                                            io.Writer
}
type hostPackage struct {
	path, dir, importcfg string
	sources              map[string][]byte
	retained             *retainedApplication
}

// Host is a replaceable private application process. Publication and revision
// admission belong to the caller, who calls Install inside ProjectBuild.Commit.
type Host struct {
	URL               string
	PID               int
	tc                Toolchain
	inputs            *gen.CompilerInputs
	files             map[string][]byte
	installed         map[string][]byte
	prepareMu         sync.Mutex
	stateMu           sync.Mutex
	authored          map[string]string
	packages          map[string]*hostPackage
	application       []string
	resources         map[string]bool
	cmd               *exec.Cmd
	commands, replies *os.File
	done              chan struct{}
	mu                sync.Mutex
	closeOnce         sync.Once
}
type HostUpdate struct {
	packagePath string
	candidate   Candidate
	files       map[string][]byte
}
type hostMessage struct {
	Op, Package, Dist string
	Candidate         Candidate
}
type hostReply struct {
	URL     string
	PID     int
	Error   string
	Rebuild bool
}

func BuildHost(ctx context.Context, o HostOptions) (_ *Host, err error) {
	if o.Inputs == nil {
		return nil, fmt.Errorf("native host requires captured compiler inputs")
	}
	if o.GoTool == "" {
		o.GoTool = "go"
	}
	if o.Output == nil {
		o.Output = io.Discard
	}
	if err = os.MkdirAll(o.WorkDir, 0755); err != nil {
		return nil, err
	}
	tc, err := PrepareToolchain(ctx, o.GoTool, filepath.Join(o.WorkDir, "toolchain"))
	if err != nil {
		return nil, err
	}
	h := &Host{tc: tc, inputs: o.Inputs, files: o.Inputs.Files(), authored: map[string]string{}, packages: map[string]*hostPackage{}, resources: map[string]bool{}, done: make(chan struct{})}
	h.installed = h.files
	for path := range h.files {
		h.authored[capturedKey(o.Inputs, path)] = path
	}
	// Package loading uses only the immutable selected tree. Production source is
	// never rewritten and generated native support exists only in HostView.
	cfg := &packages.Config{Context: ctx, Dir: o.Inputs.Dir, Env: tc.command(ctx, o.Inputs.Dir).Env, Mode: packages.NeedEmbedPatterns | packages.NeedEmbedFiles | packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports | packages.NeedDeps | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedModule}
	roots, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, err
	}
	var constructors []*packages.Package
	for _, p := range roots {
		if p.Name != "main" && (o.HandlerPackage == "" || p.PkgPath == o.HandlerPackage) {
			name := o.HandlerFunction
			if name == "" {
				name = "NewHandler"
			}
			if constructor(p, name) {
				constructors = append(constructors, p)
			}
		}
	}
	if len(constructors) != 1 {
		return nil, fmt.Errorf("native host requires exactly one application NewHandler(fs.FS,string) (http.Handler,error); found %d (set HandlerPackage/HandlerFunction for an explicit constructor)", len(constructors))
	}
	app := constructors[0]
	h.application = []string{app.PkgPath}
	var executable []*packages.Package
	packages.Visit([]*packages.Package{app}, nil, func(p *packages.Package) { executable = append(executable, p) })

	for _, p := range executable {
		for _, name := range p.EmbedFiles {
			h.resources[name] = true
		}
		for _, pattern := range p.EmbedPatterns {
			pattern = strings.Replace(pattern, string(filepath.Separator)+"all:", string(filepath.Separator), 1)
			prefix := pattern
			if wildcard := strings.IndexAny(pattern, "*?["); wildcard >= 0 {
				prefix = filepath.Dir(pattern[:wildcard])
				if strings.HasSuffix(pattern[:wildcard], string(filepath.Separator)) {
					prefix = filepath.Clean(pattern[:wildcard])
				}
			} else {
				info, err := os.Stat(prefix)
				if err != nil || !info.IsDir() {
					continue
				}
			}
			h.resources[prefix+string(filepath.Separator)] = true
		}
	}
	for _, p := range executable {
		if len(p.Errors) > 0 {
			return nil, fmt.Errorf("load captured application %s: %v", p.PkgPath, p.Errors)
		}
	}
	if o.HandlerFunction == "" {
		o.HandlerFunction = "NewHandler"
	}
	overlays := map[string][]byte{}

	raw, err := tc.command(ctx, o.Inputs.Dir, "list", "-export", "-deps", "-f", "{{if .Export}}packagefile {{.ImportPath}}={{.Export}}{{end}}", app.PkgPath).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("native application archives: %w\n%s", err, raw)
	}
	importcfg := filepath.Join(o.WorkDir, "importcfg")
	if err = os.WriteFile(importcfg, raw, 0644); err != nil {
		return nil, err
	}
	identity, err := dependencyIdentity(importcfg)
	if err != nil {
		return nil, err
	}
	var paths []string
	linkRoot := filepath.Join(o.Inputs.Out, "links") + string(filepath.Separator)
	for _, p := range executable {
		if len(p.CompiledGoFiles) == 0 || !strings.HasPrefix(p.CompiledGoFiles[0], linkRoot) {
			continue
		}
		sources := map[string][]byte{}
		for _, name := range p.CompiledGoFiles {
			b, ok := h.files[name]
			if !ok {
				return nil, fmt.Errorf("captured route source missing: %s", name)
			}
			sources[name] = b
		}
		tc.AuthoredPaths = map[string]string{}
		for name := range sources {
			tc.AuthoredPaths[name] = o.Inputs.AuthoredPath(name)
		}
		lower, e := LowerAuthored(p, sources, tc.AuthoredPaths)
		if e != nil {
			var full *CompleteBuild
			if errors.As(e, &full) {
				continue
			}
			return nil, e
		}
		if len(lower.Functions) == 0 {
			continue
		}
		dir := filepath.Dir(p.CompiledGoFiles[0])

		for name, body := range lower.Host {
			body = bytes.ReplaceAll(body, []byte("SKGO_HOST_DEPENDENCIES"), []byte(identity))
			overlays[filepath.Join(dir, name)] = body
		}
		h.packages[p.PkgPath] = &hostPackage{path: p.PkgPath, dir: dir, importcfg: importcfg, sources: sources, retained: &retainedApplication{files: h.files}}
		paths = append(paths, p.PkgPath)
	}
	sort.Strings(paths)
	// Existing application middleware and hooks remain the constructor's business.
	// Only the native lease crosses FetchConfig's intentional fresh context seam.
	appDir := filepath.Dir(app.CompiledGoFiles[0])
	carry, err := carrySource(app.Name, paths)
	if err != nil {
		return nil, err
	}
	overlays[filepath.Join(appDir, "skgo_development_context.go")] = carry
	for _, name := range app.CompiledGoFiles {
		body := h.files[name]
		patched, e := injectCarry(name, body)
		if e != nil {
			return nil, e
		}
		if !bytes.Equal(body, patched) {
			overlays[name] = patched
		}
	}
	main, err := hostSource(app.PkgPath, o.HandlerFunction, paths)
	if err != nil {
		return nil, err
	}
	mainDir := filepath.Join(o.Inputs.Dir, "skgo_development_host")
	mainPath := filepath.Join(mainDir, "main.go")
	overlays[mainPath] = main
	// Go's overlay discovers virtual files inside existing directories.
	if err = os.MkdirAll(mainDir, 0755); err != nil {
		return nil, err
	}
	hostTC, err := tc.HostView(ctx, o.Inputs.Dir, filepath.Join(o.WorkDir, "view"), overlays)
	if err != nil {
		return nil, err
	}
	if raw, e := hostTC.command(ctx, o.Inputs.Dir, "mod", "tidy").CombinedOutput(); e != nil {
		return nil, fmt.Errorf("native host dependencies: %w\n%s", e, raw)
	}
	binary := filepath.Join(o.WorkDir, "host")
	if raw, e := hostTC.command(ctx, o.Inputs.Dir, "build", "-ldflags=-s=false", "-o", binary, "./skgo_development_host").CombinedOutput(); e != nil {
		return nil, fmt.Errorf("build native host: %w\n%s", e, raw)
	}
	commandRead, commandWrite, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	replyRead, replyWrite, err := os.Pipe()
	if err != nil {
		commandRead.Close()
		commandWrite.Close()
		return nil, err
	}
	h.commands = commandWrite
	h.replies = replyRead
	h.cmd = exec.CommandContext(ctx, binary)
	h.cmd.Dir = o.Inputs.Dir
	h.cmd.Env = append(os.Environ(), "SKGO_DEV_ORIGIN="+o.Origin, "SKGO_DEV_DIST="+o.DistDir)
	h.cmd.ExtraFiles = []*os.File{commandRead, replyWrite}
	h.cmd.Stdout = o.Output
	h.cmd.Stderr = o.Output
	if err = h.cmd.Start(); err != nil {
		commandRead.Close()
		commandWrite.Close()
		replyRead.Close()
		replyWrite.Close()
		return nil, err
	}
	commandRead.Close()
	replyWrite.Close()
	go func() { _ = h.cmd.Wait(); close(h.done) }()
	defer func() {
		if err != nil {
			_ = h.Close()
		}
	}()
	var ready hostReply
	if err = h.receive(ctx, &ready); err != nil {
		return nil, fmt.Errorf("native host readiness: %w", err)
	}
	if ready.Error != "" {
		return nil, fmt.Errorf("native host initialization: %s", ready.Error)
	}
	h.URL = ready.URL
	h.PID = ready.PID
	return h, nil
}
func constructor(p *packages.Package, name string) bool {
	if p.Types == nil {
		return false
	}
	obj, ok := p.Types.Scope().Lookup(name).(*types.Func)
	if !ok {
		return false
	}
	s := obj.Type().(*types.Signature)
	if s.Params().Len() != 2 || s.Results().Len() != 2 {
		return false
	}
	return types.TypeString(s.Params().At(0).Type(), nil) == "io/fs.FS" && types.TypeString(s.Params().At(1).Type(), nil) == "string" && types.TypeString(s.Results().At(0).Type(), nil) == "net/http.Handler" && types.TypeString(s.Results().At(1).Type(), nil) == "error"
}
func (h *Host) Done() <-chan struct{} { return h.done }
func (h *Host) Close() error {
	h.closeOnce.Do(func() {
		if h.cmd != nil && h.cmd.Process != nil {
			_ = h.cmd.Process.Kill()
			<-h.done
		}
		if h.commands != nil {
			h.commands.Close()
		}
		if h.replies != nil {
			h.replies.Close()
		}
		for _, p := range h.packages {
			if p.retained.view != nil {
				p.retained.view.Close()
			}
		}
	})
	return nil
}
func (h *Host) receive(ctx context.Context, r *hostReply) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(30 * time.Second)
	}
	_ = h.replies.SetReadDeadline(deadline)
	return json.NewDecoder(h.replies).Decode(r)
}
func (h *Host) exchange(ctx context.Context, m hostMessage) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := json.NewEncoder(h.commands).Encode(m); err != nil {
		return err
	}
	var r hostReply
	if err := h.receive(ctx, &r); err != nil {
		return err
	}
	if r.Rebuild {
		return &CompleteBuild{r.Error}
	}
	if r.Error != "" {
		return errors.New(r.Error)
	}
	return nil
}
func (h *Host) Reload(ctx context.Context, distDir string) error {
	return h.exchange(ctx, hostMessage{Op: "reload", Dist: distDir})
}
func (h *Host) Install(ctx context.Context, u *HostUpdate) error {
	if u == nil {
		return nil
	}
	if err := h.exchange(ctx, hostMessage{Op: "install", Package: u.packagePath, Candidate: u.candidate}); err != nil {
		return err
	}
	h.stateMu.Lock()
	h.installed = u.files
	h.stateMu.Unlock()
	return nil
}

// Prepare compares captured application membership/resources and compiles at
// most one independent package. A multi-package edit replaces the host so no
// request can observe only half of an application revision.
func (h *Host) Prepare(ctx context.Context, next *gen.CompilerInputs, outputDir string) (*HostUpdate, error) {
	h.prepareMu.Lock()
	defer h.prepareMu.Unlock()
	h.stateMu.Lock()
	installed := h.installed
	h.stateMu.Unlock()
	files := map[string][]byte{}
	for path, b := range next.Files() {
		base, ok := h.authored[capturedKey(next, path)]
		if !ok {
			if !nativeInput(h.inputs.PrivatePath(next.AuthoredPath(path)), h.resources) {
				continue
			}
			return nil, &CompleteBuild{"application file membership changed"}
		}
		if filepath.Base(path) == "go.mod" || strings.HasSuffix(path, ".go") {
			b = bytes.ReplaceAll(b, []byte(next.Dir), []byte(h.inputs.Dir))
			for oldName := range h.files {
				if filepath.Base(oldName) == "go.mod" {
					canonical := h.inputs.AuthoredPath(oldName)
					newName := next.PrivatePath(canonical)
					b = bytes.ReplaceAll(b, []byte(filepath.Dir(newName)), []byte(filepath.Dir(oldName)))
				}
			}
		}
		files[base] = b
	}
	for name, b := range h.files {
		if _, ok := files[name]; !ok {
			if nativeInput(name, h.resources) {
				return nil, &CompleteBuild{"application file membership changed"}
			}
			files[name] = b
		}
	}
	changed := map[string]bool{}
	for name, b := range files {
		if !nativeInput(name, h.resources) || bytes.Equal(b, installed[name]) {
			continue
		}
		found := ""
		for path, p := range h.packages {
			if _, ok := p.sources[name]; ok {
				found = path
				break
			}
		}
		if found == "" {
			compiled := h.inputs.CompiledPath(h.inputs.AuthoredPath(name))
			linked := false
			if compiled != name {
				for _, p := range h.packages {
					if _, ok := p.sources[compiled]; ok {
						linked = true
						break
					}
				}
			}
			if linked {
				continue
			}
			return nil, &CompleteBuild{"application dependency, configuration or resource changed: " + h.inputs.AuthoredPath(name)}
		}
		changed[found] = true
	}
	if len(changed) == 0 {
		return nil, nil
	}
	if len(changed) != 1 {
		return nil, &CompleteBuild{"multiple native packages changed"}
	}
	for path := range changed {
		p := h.packages[path]
		after := map[string][]byte{}
		for name := range p.sources {
			after[name] = files[name]
		}
		if !p.retained.initialized {
			baseline := &packages.Config{Context: ctx, Dir: h.inputs.Dir, Env: h.tc.command(ctx, h.inputs.Dir).Env, Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports | packages.NeedDeps | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedModule}
			if _, err := p.retained.Load(baseline, append([]string{path}, h.application...)...); err != nil {
				return nil, err
			}
		}
		p.retained.files = files
		tc := h.tc
		tc.AuthoredPaths = map[string]string{}
		for name := range after {
			tc.AuthoredPaths[name] = h.inputs.AuthoredPath(name)
		}
		c, err := tc.compileWithLoader(ctx, h.inputs.Dir, path, p.importcfg, outputDir, p.sources, after, h.application, p.retained.Load)
		if err != nil {
			return nil, err
		}
		return &HostUpdate{packagePath: path, candidate: c, files: files}, nil
	}
	panic("unreachable")
}
func carrySource(pkg string, paths []string) ([]byte, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "package %s\nimport(\"context\"\n", pkg)
	for i, p := range paths {
		fmt.Fprintf(&b, "hot%d %q\n", i, p)
	}
	b.WriteString(")\nfunc skgoDevelopmentCarryContext(dst,src context.Context)context.Context{\n")
	for i := range paths {
		fmt.Fprintf(&b, "dst=hot%d.SkgoHotContext(dst,src)\n", i)
	}
	b.WriteString("return dst}\nfunc skgoDevelopmentComposeCarryContext(original func(context.Context,context.Context)context.Context)func(context.Context,context.Context)context.Context{return func(dst,src context.Context)context.Context{if original!=nil{dst=original(dst,src)};return skgoDevelopmentCarryContext(dst,src)}}\n")
	return format.Source([]byte(b.String()))
}
func injectCarry(name string, body []byte) ([]byte, error) {
	fs := token.NewFileSet()
	f, err := parser.ParseFile(fs, name, body, 0)
	if err != nil {
		return nil, err
	}
	changed := false
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		sel, ok := lit.Type.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "FetchConfig" {
			return true
		}
		for _, e := range lit.Elts {
			if kv, ok := e.(*ast.KeyValueExpr); ok {
				if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "CarryContext" {
					kv.Value = &ast.CallExpr{Fun: ast.NewIdent("skgoDevelopmentComposeCarryContext"), Args: []ast.Expr{kv.Value}}
					changed = true
					return false
				}
			}
		}
		lit.Elts = append(lit.Elts, &ast.KeyValueExpr{Key: ast.NewIdent("CarryContext"), Value: ast.NewIdent("skgoDevelopmentCarryContext")})
		changed = true
		return true
	})
	if !changed {
		return body, nil
	}
	var out bytes.Buffer
	err = format.Node(&out, fs, f)
	return out.Bytes(), err
}
func hostSource(app, function string, paths []string) ([]byte, error) {
	var b strings.Builder
	b.WriteString("package main\nimport(\"encoding/json\";\"fmt\";\"net\";\"net/http\";\"os\";\"strings\";\"sync\";\"io\"\n")
	fmt.Fprintf(&b, "app %q\n", app)
	if len(paths) > 0 {
		b.WriteString("loader \"github.com/pkujhd/goloader\"\n")
	}
	for i, p := range paths {
		fmt.Fprintf(&b, "hot%d %q\n", i, p)
	}
	b.WriteString(")\ntype candidate struct{Object string;Storage []string;Reclaim bool}\ntype command struct{Op,Package,Dist string;Candidate candidate}\ntype reply struct{URL string;PID int;Error string;Rebuild bool}\nfunc main(){enc:=json.NewEncoder(os.NewFile(4,\"replies\"));fail:=func(e error){enc.Encode(reply{Error:e.Error()})}\n")
	if len(paths) > 0 {
		b.WriteString("symbols:=map[string]uintptr{};if e:=loader.RegSymbol(symbols);e!=nil{fail(e);return}\n")
	}
	for i := range paths {
		fmt.Fprintf(&b, "if e:=hot%d.SkgoHotInitSymbols(32,symbols);e!=nil{fail(e);return}\n", i)
	}
	fmt.Fprintf(&b, "origin:=os.Getenv(\"SKGO_DEV_ORIGIN\");var mu sync.RWMutex;var handler http.Handler;reload:=func(dist string)error{h,e:=app.%s(os.DirFS(dist),origin);if e!=nil{return e};", function)
	for i := range paths {
		fmt.Fprintf(&b, "h=hot%d.SkgoHotHandler(h);", i)
	}
	b.WriteString("mu.Lock();handler=h;mu.Unlock();return nil};if e:=reload(os.Getenv(\"SKGO_DEV_DIST\"));e!=nil{fail(e);return};listener,e:=net.Listen(\"tcp\",\"127.0.0.1:0\");if e!=nil{fail(e);return};server:=&http.Server{Handler:http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){mu.RLock();h:=handler;mu.RUnlock();h.ServeHTTP(w,r)})};defer server.Close();go server.Serve(listener);enc.Encode(reply{URL:\"http://\"+listener.Addr().String(),PID:os.Getpid()});dec:=json.NewDecoder(os.NewFile(3,\"commands\"));for{var c command;if e:=dec.Decode(&c);e!=nil{if e!=io.EOF{fmt.Fprintln(os.Stderr,e)};return};var e error;switch c.Op{case \"reload\":e=reload(c.Dist);case \"install\":switch c.Package{")
	for i, p := range paths {
		fmt.Fprintf(&b, "case %s:e=hot%d.SkgoHotInstall(c.Candidate.Object,c.Candidate.Storage,c.Candidate.Reclaim);", strconv.Quote(p), i)
	}
	b.WriteString("default:e=fmt.Errorf(\"unknown native package %s\",c.Package)};default:e=fmt.Errorf(\"unknown native host command %s\",c.Op)};r:=reply{PID:os.Getpid()};if e!=nil{r.Error=e.Error();r.Rebuild=strings.HasPrefix(r.Error,\"host replacement required:\")};enc.Encode(r)}}\n")
	return format.Source([]byte(b.String()))
}

func capturedKey(i *gen.CompilerInputs, path string) string {
	key := i.AuthoredPath(path)
	rel, err := filepath.Rel(i.Dir, path)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		key += "|" + rel
	}
	return key
}
func nativeInput(path string, resources map[string]bool) bool {
	for scope := range resources {
		if strings.HasSuffix(scope, string(filepath.Separator)) && strings.HasPrefix(path, scope) {
			return true
		}
	}
	return resources[path] || strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") || filepath.Base(path) == "go.mod" || filepath.Base(path) == "go.sum"
}
