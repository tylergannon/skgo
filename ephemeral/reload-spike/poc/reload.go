package main

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/pkujhd/goloader"
)

var ErrRestart = errors.New("normal rebuild/restart required")
var ErrLoaderState = errors.New("loader state uncertain; stop experimental host")

// This is intentionally a narrow experimental lane, not an arbitrary Go
// package reloader. Only these synchronous operations may enter loaded code.
var calls = map[string]bool{
	"identity": true, "pause": true, "fmt.Sprintf": true,
	"strconv.Atoi": true, "counter.Add": true,
	"r.URL.Query": true, "r.URL.Query().Get": true,
}

func printed(n interface{}) string {
	var b bytes.Buffer
	_ = printer.Fprint(&b, token.NewFileSet(), n)
	return b.String()
}

// compatible permits function-body edits only. Its deliberately closed syntax
// and call set prevents new types, retained callbacks, module globals, init,
// goroutines and writes through host pointers. Expand only with separate proof.
func compatible(source []byte) error {
	f, err := parser.ParseFile(token.NewFileSet(), "handler.go", source, parser.ParseComments)
	if err != nil {
		return err
	}
	for _, group := range f.Comments {
		for _, c := range group.List {
			if strings.Contains(c.Text, "go:") && c.Text != "//go:noinline" {
				return fmt.Errorf("%w: compiler directive", ErrRestart)
			}
		}
	}
	f.Comments = nil
	var bodies []*ast.BlockStmt
	for _, d := range f.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok {
			bodies = append(bodies, fn.Body)
			fn.Body, fn.Doc = nil, nil
		}
	}
	contract, _ := parser.ParseFile(token.NewFileSet(), "contract.go", `package hotcode
import ("fmt"; "net/http"; "strconv"; "sync/atomic")
func identity[T ~string](s T) T
func Value(r *http.Request, counter *atomic.Int64, pause func()) string
`, 0)
	if printed(f) != printed(contract) {
		return fmt.Errorf("%w: imports, signatures or declarations changed", ErrRestart)
	}
	for _, body := range bodies {
		var problem error
		// Function references must be direct calls, so none can escape in an
		// interface, host field, callback, return value or imported function argument.
		direct := map[ast.Node]bool{}
		ast.Inspect(body, func(n ast.Node) bool {
			if c, ok := n.(*ast.CallExpr); ok {
				direct[c.Fun] = true
			}
			return true
		})
		ast.Inspect(body, func(n ast.Node) bool {
			if problem != nil {
				return false
			}
			bad := func(why string) { problem = fmt.Errorf("%w: %s", ErrRestart, why) }
			switch n := n.(type) {
			case *ast.GoStmt, *ast.DeferStmt, *ast.FuncLit, *ast.TypeSpec,
				*ast.SendStmt, *ast.ChanType, *ast.SelectStmt, *ast.CompositeLit,
				*ast.StructType, *ast.InterfaceType, *ast.ArrayType, *ast.MapType:
				bad("unsupported lifetime or type construction")
			case *ast.CallExpr:
				if !calls[printed(n.Fun)] {
					bad("unapproved call " + printed(n.Fun))
				}
			case *ast.Ident:
				if (n.Name == "Value" || n.Name == "identity" || n.Name == "pause") && !direct[n] {
					bad("function value may escape")
				}
			case *ast.SelectorExpr:
				name := printed(n)
				if calls[name] {
					if !direct[n] {
						bad("method/function value may escape")
					}
				} else if name != "r.URL" && name != "r.URL.Path" {
					bad("unapproved field or method " + name)
				}
			case *ast.UnaryExpr:
				if n.Op == token.AND || n.Op == token.ARROW {
					bad("address or channel operation")
				}
			case *ast.AssignStmt:
				for _, target := range n.Lhs {
					if _, ok := target.(*ast.Ident); !ok {
						bad("write through host pointer")
					}
				}
			case *ast.IncDecStmt:
				if _, ok := n.X.(*ast.Ident); !ok {
					bad("write through host pointer")
				}
			}
			return problem == nil
		})
		if problem != nil {
			return problem
		}
	}
	return nil
}

type handler func(*http.Request, *atomic.Int64, func()) string

type revision struct {
	fn     handler
	entry  *uintptr
	module *goloader.CodeModule
}

type timing struct {
	FenceMS           float64 `json:"fence_ms"`
	CompileMS         float64 `json:"compile_ms"`
	LoadMS            float64 `json:"load_ms"`
	DrainSwapUnloadMS float64 `json:"drain_swap_unload_ms"`
}

type reloader struct {
	// update serializes compilation/loading and bounds the number of modules to
	// one active plus one candidate. calls holds an executing revision alive.
	update      sync.Mutex
	calls       sync.RWMutex
	current     *revision
	symbols     map[string]uintptr
	dir         string
	goBinary    string
	fence       *fence
	counter     atomic.Int64
	pause       func()
	closed      bool
	loaded      int
	unloaded    int
	live        int
	peak        int
	beforeDrain func()
}

var symbolOnce sync.Once
var hostSymbols map[string]uintptr
var symbolError error

func newReloader(dir string) (*reloader, error) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" || runtime.Version() != "go1.27.2" {
		return nil, fmt.Errorf("this POC was exercised only on Go 1.27.2 darwin/arm64")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	r := &reloader{dir: dir, symbols: map[string]uintptr{}, pause: func() {}}
	symbolOnce.Do(func() {
		hostSymbols = map[string]uintptr{}
		symbolError = goloader.RegSymbol(hostSymbols)
		if symbolError == nil {
			goloader.RegTypes(hostSymbols, &http.Request{}, &r.counter, fmt.Sprintf, strconv.Atoi)
		}
	})
	if symbolError != nil {
		return nil, symbolError
	}
	for name, address := range hostSymbols {
		r.symbols[name] = address
	}
	goBinary, err := exec.LookPath("go")
	if err != nil {
		return nil, err
	}
	r.goBinary = goBinary
	cmd := exec.Command(goBinary, "list", "-export", "-deps", "-f", "{{if .Export}}packagefile {{.ImportPath}}={{.Export}}{{end}}", "net/http", "fmt", "sync/atomic", "strconv")
	config, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("resolve frozen exports: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "importcfg"), config, 0644); err != nil {
		return nil, err
	}
	metadata, err := exec.Command(goBinary, "list", "-export", "-deps", "-json", "net/http", "fmt", "sync/atomic", "strconv").Output()
	if err != nil {
		return nil, err
	}
	r.fence, err = newFence(goBinary, string(metadata))
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (r *reloader) Reload(source []byte) (timing, error) {
	r.update.Lock()
	defer r.update.Unlock()
	var times timing
	if r.closed {
		return times, errors.New("reloader closed")
	}
	start := time.Now()
	if err := compatible(source); err != nil {
		return times, err
	}
	if err := r.fence.check(); err != nil {
		return times, err
	}
	times.FenceMS = time.Since(start).Seconds() * 1000
	start = time.Now()
	path := filepath.Join(r.dir, "handler.go")
	if err := os.WriteFile(path, source, 0644); err != nil {
		return times, err
	}
	obj := filepath.Join(r.dir, "handler.o")
	cmd := exec.Command(r.goBinary, "tool", "compile", "-p", "hotcode", "-lang=go1.27", "-importcfg", filepath.Join(r.dir, "importcfg"), "-o", obj, path)
	if out, err := cmd.CombinedOutput(); err != nil {
		return times, fmt.Errorf("compile: %w: %s", err, out)
	}
	times.CompileMS = time.Since(start).Seconds() * 1000
	start = time.Now()
	linker, err := goloader.ReadObjs([]string{obj}, []string{"hotcode"})
	if err != nil {
		return times, err
	}
	// Missing references are rejected before Load: upstream Load's error path
	// does not promise to release partially allocated executable mappings.
	if missing := goloader.UnresolvedSymbols(linker, r.symbols); len(missing) != 0 {
		return times, fmt.Errorf("%w: unresolved host symbols %v", ErrRestart, missing)
	}
	fenceStart := time.Now()
	if err := r.fence.check(); err != nil {
		return times, err
	}
	times.FenceMS += time.Since(fenceStart).Seconds() * 1000
	start = time.Now()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	module, err := goloader.Load(linker, r.symbols)
	if err != nil {
		return times, fmt.Errorf("%w: %v", ErrLoaderState, err)
	}
	r.loaded++
	r.live++
	if r.live > r.peak {
		r.peak = r.live
	}
	address := module.Syms["hotcode.Value"]
	if address == 0 {
		module.Unload()
		r.unloaded++
		r.live--
		return times, errors.New("missing Value")
	}
	entry := new(uintptr)
	*entry = address
	fn := *(*handler)(unsafe.Pointer(&entry))
	candidate := &revision{fn: fn, entry: entry, module: module}
	times.LoadMS = time.Since(start).Seconds() * 1000
	start = time.Now()
	// Readers hold this through the return-value copy. A pending writer blocks
	// new admissions; existing calls finish before the old code is unmapped.
	if r.beforeDrain != nil {
		r.beforeDrain()
	}
	r.calls.Lock()
	old := r.current
	r.current = candidate
	if old != nil {
		old.fn = nil
		old.module.Unload()
		r.unloaded++
		r.live--
	}
	r.calls.Unlock()
	times.DrainSwapUnloadMS = time.Since(start).Seconds() * 1000
	return times, nil
}

func (r *reloader) response(req *http.Request) string {
	r.calls.RLock()
	defer r.calls.RUnlock()
	if r.current == nil {
		return "no working revision"
	}
	// A literal may reside in module data. Clone before releasing its lifetime.
	return strings.Clone(r.current.fn(req, &r.counter, r.pause))
}

func (r *reloader) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("X-Host-PID", strconv.Itoa(os.Getpid()))
	fmt.Fprint(w, r.response(req))
}

func (r *reloader) Close() {
	r.update.Lock()
	defer r.update.Unlock()
	if r.closed {
		return
	}
	r.closed = true
	r.calls.Lock()
	defer r.calls.Unlock()
	if r.current != nil {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		r.current.fn = nil
		r.current.module.Unload()
		r.unloaded++
		r.live--
		r.current = nil
	}
}
