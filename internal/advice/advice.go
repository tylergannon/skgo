// Package advice checks skgo-specific mistakes in authored Go handlers.
// Its checks intentionally stop where the registered handler or route cannot
// be established from this package's syntax and type information.
package advice

import (
	"encoding/base32"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/analysis"
)

const skgoPath = "github.com/tylergannon/skgo"

// Analyzer reports skgo developer advice at authored source locations. Rule
// codes are stable identifiers for CLI, editor, and documentation consumers.
var Analyzer = &analysis.Analyzer{
	Name: "skgoadvice",
	Doc:  "check registered skgo handlers for known request and wire-contract mistakes",
	Run:  run,
}

const (
	QueryCookie    = "SKGO001"
	QueryPageInput = "SKGO002"
	RequestContext = "SKGO003"
	OperationError = "SKGO004"
	RefreshKind    = "SKGO005"
	FormField      = "SKGO006"
	// WireContract is emitted by the existing generator's authoritative checks.
	WireContract = "SKGO007"
	RouteParam   = "SKGO008"
)

type handler struct {
	fn    *ast.FuncDecl
	kind  string
	input types.Type
	ctx   types.Object
	yield types.Object
	route map[string]bool
}

func run(pass *analysis.Pass) (any, error) {
	decls := make(map[types.Object]*ast.FuncDecl)
	markers := make(map[types.Object]string)
	registrations := make(map[types.Object]string)
	loads := make(map[types.Object]map[string]bool)
	loadSeen := make(map[types.Object]bool)
	loadAmbiguous := make(map[types.Object]bool)
	ambiguous := make(map[types.Object]bool)
	for _, file := range pass.Files {
		filename := pass.Fset.Position(file.Pos()).Filename
		remoteFile := strings.HasSuffix(filename, ".remote.go")
		loadFile := filepath.Base(filename) == "page.server.go" || filepath.Base(filename) == "layout.server.go"
		for _, d := range file.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if ok && fn.Recv == nil {
				decls[pass.TypesInfo.Defs[fn.Name]] = fn
			}
			// The generator recognizes declarations in var initializers. A
			// marker called in ordinary code does not register its argument.
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR || (!remoteFile && !loadFile) {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, value := range vs.Values {
					call, ok := value.(*ast.CallExpr)
					if !ok || len(call.Args) != 1 {
						continue
					}
					kind := skgoCall(pass, call)
					if id, ok := call.Args[0].(*ast.Ident); ok {
						obj := pass.TypesInfo.Uses[id]
						if loadFile && kind == "Load" {
							if loadSeen[obj] {
								loadAmbiguous[obj] = true
							}
							loadSeen[obj] = true
							loads[obj] = routeParams(filename)
						}
						if !remoteFile || !remoteKind(kind) {
							continue
						}
						if existing := registrations[obj]; existing != "" && existing != kind {
							ambiguous[obj] = true
						}
						registrations[obj] = kind
					}
				}
			}
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				return true
			}
			name := skgoCall(pass, call)
			switch name {
			case "Query", "LiveQuery", "BatchQuery", "Command", "Form", "Load":
				if id, ok := call.Args[0].(*ast.Ident); ok {
					markers[pass.TypesInfo.Uses[id]] = name
				}
			}
			return true
		})
	}
	for obj := range ambiguous {
		delete(registrations, obj)
	}
	for obj := range loadAmbiguous {
		delete(loads, obj)
	}
	for obj, kind := range markers {
		fn := decls[obj]
		if fn == nil || fn.Body == nil {
			continue
		}
		h := handler{fn: fn, kind: kind}
		sig, _ := obj.Type().(*types.Signature)
		if sig != nil && sig.Params().Len() > 0 {
			h.ctx = sig.Params().At(0)
			if kind == "Form" && sig.Params().Len() > 1 {
				h.input = sig.Params().At(1).Type()
			}
			if kind == "LiveQuery" {
				h.yield = sig.Params().At(sig.Params().Len() - 1)
			}
		}
		if kind == "Load" {
			h.route = loads[obj]
		}
		checkHandler(pass, h, registrations)
	}
	return nil, nil
}

func report(pass *analysis.Pass, node ast.Node, code, message string, args ...any) {
	pass.Report(analysis.Diagnostic{Pos: node.Pos(), End: node.End(), Category: code, Message: fmt.Sprintf(message, args...)})
}

func skgoCall(pass *analysis.Pass, call *ast.CallExpr) string {
	fn := calledFunc(pass, call.Fun)
	if fn != nil && fn.Pkg() != nil && fn.Pkg().Path() == skgoPath {
		return fn.Name()
	}
	return ""
}

func calledFunc(pass *analysis.Pass, expr ast.Expr) *types.Func {
	switch x := expr.(type) {
	case *ast.IndexExpr:
		return calledFunc(pass, x.X)
	case *ast.IndexListExpr:
		return calledFunc(pass, x.X)
	case *ast.Ident:
		fn, _ := pass.TypesInfo.Uses[x].(*types.Func)
		return fn
	case *ast.SelectorExpr:
		fn, _ := pass.TypesInfo.Uses[x.Sel].(*types.Func)
		return fn
	}
	return nil
}

func remoteKind(kind string) bool {
	switch kind {
	case "Query", "BatchQuery", "LiveQuery", "Command", "Form":
		return true
	}
	return false
}

func checkHandler(pass *analysis.Pass, h handler, registrations map[types.Object]string) {
	origins := handlerOrigins(pass, h)
	ast.Inspect(h.fn.Body, func(n ast.Node) bool {
		// A nested closure has its own execution context; this handler marker
		// proves nothing about where a passed closure will run.
		if _, nested := n.(*ast.FuncLit); nested {
			return false
		}
		switch x := n.(type) {
		case *ast.CallExpr:
			checkCall(pass, h, x, registrations, origins)
		case *ast.CompositeLit:
			if h.kind == "Form" && h.input != nil {
				checkIssueLiteral(pass, h.input, x)
			}
		case *ast.ExprStmt:
			call, ok := x.X.(*ast.CallExpr)
			if ok {
				checkDiscarded(pass, h, call)
			}
		case *ast.AssignStmt:
			if len(x.Rhs) == 1 {
				if call, ok := x.Rhs[0].(*ast.CallExpr); ok && allBlank(x.Lhs) {
					checkDiscarded(pass, h, call)
				}
			}
		}
		return true
	})
}

func checkCall(pass *analysis.Pass, h handler, call *ast.CallExpr, registrations map[types.Object]string, origins *eventOrigins) {
	name := skgoCall(pass, call)
	if name == "EventFrom" && len(call.Args) == 1 && freshContext(pass, call.Args[0]) {
		report(pass, call, RequestContext, "EventFrom on a fresh context loses this handler's request event. Pass the supplied handler context or a context derived from it.")
	}
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		if eventMethod(pass, sel) {
			switch sel.Sel.Name {
			case "SetCookie", "DeleteCookie":
				if isQuery(h.kind) && origins.fromHandler(sel.X) {
					report(pass, call, QueryCookie, "This %s cannot change cookies; Kit refuses the write and cached queries may skip it. Move the cookie change into a command or form.", h.kind)
				}
			case "Param", "URL", "SearchParam", "RouteID":
				if isQuery(h.kind) && origins.fromHandler(sel.X) {
					report(pass, call, QueryPageInput, "This query cannot read page %s; Kit refuses page-dependent reads because the query cache key omits that page value and may reuse a stale result. Pass the page value as a typed query argument.", sel.Sel.Name)
				} else if h.kind == "Load" && sel.Sel.Name == "Param" && h.route != nil && origins.fromHandler(sel.X) && len(call.Args) == 1 {
					if value, ok := stringLiteral(pass, call.Args[0]); ok && !h.route[value] {
						report(pass, call.Args[0], RouteParam, "Route parameter %q is not declared by this page route; Event.Param returns an empty string for that name. Use a parameter declared by this route.", value)
					}
				}
			}
		}
		if sel.Sel.Name == "Add" && invalidMethod(pass, sel) && h.kind == "Form" && h.input != nil && len(call.Args) > 0 {
			checkField(pass, h.input, call.Args[0])
		}
	}
	if name == "Invalidf" && h.kind == "Form" && h.input != nil && len(call.Args) > 0 {
		checkField(pass, h.input, call.Args[0])
	}
	if name == "Refresh" || name == "RefreshNoArg" || name == "RefreshRequested" || name == "RefreshRequestedNoArg" || name == "ReconnectRequested" || name == "ReconnectRequestedNoArg" {
		if len(call.Args) < 2 {
			return
		}
		id, ok := call.Args[1].(*ast.Ident)
		if !ok {
			return
		}
		kind := registrations[pass.TypesInfo.Uses[id]]
		if kind == "" {
			return
		}
		want := "Query"
		if strings.HasPrefix(name, "Reconnect") {
			want = "LiveQuery"
		}
		if kind != want {
			report(pass, id, RefreshKind, "%s targets a registered %s; skgo rejects this operation at runtime. Use a registered %s target.", name, kind, want)
		}
	}
}

func isQuery(kind string) bool {
	return kind == "Query" || kind == "LiveQuery" || kind == "BatchQuery"
}

func eventMethod(pass *analysis.Pass, sel *ast.SelectorExpr) bool {
	fn, _ := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if fn == nil || fn.Pkg() == nil || fn.Pkg().Path() != skgoPath {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() == nil {
		return false
	}
	p, ok := types.Unalias(sig.Recv().Type()).(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := p.Elem().(*types.Named)
	return ok && named.Obj().Name() == "Event"
}

func freshContext(pass *analysis.Pass, expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	fn := calledFunc(pass, call.Fun)
	if fn == nil || fn.Pkg() == nil || fn.Pkg().Path() != "context" {
		return false
	}
	switch fn.Name() {
	case "Background", "TODO":
		return true
	case "WithValue", "WithCancel", "WithDeadline", "WithTimeout", "WithoutCancel":
		return len(call.Args) > 0 && freshContext(pass, call.Args[0])
	}
	return false
}

func checkDiscarded(pass *analysis.Pass, h handler, call *ast.CallExpr) {
	if skgoOperation(pass, call) {
		report(pass, call, OperationError, "Handle the skgo operation error; discarding it hides a failed cookie write or refresh.")
		return
	}
	if h.yield != nil {
		if id, ok := call.Fun.(*ast.Ident); ok && pass.TypesInfo.Uses[id] == h.yield {
			report(pass, call, OperationError, "Handle the live-query yield error; it signals a disconnected subscriber.")
		}
	}
}

func skgoOperation(pass *analysis.Pass, call *ast.CallExpr) bool {
	switch skgoCall(pass, call) {
	case "Refresh", "RefreshNoArg", "RefreshRequested", "RefreshRequestedNoArg", "ReconnectRequested", "ReconnectRequestedNoArg", "IgnoreRequested", "IgnoreRequestedNoArg":
		return true
	}
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok && eventMethod(pass, sel) {
		return sel.Sel.Name == "SetCookie" || sel.Sel.Name == "DeleteCookie"
	}
	return false
}

func allBlank(exprs []ast.Expr) bool {
	if len(exprs) == 0 {
		return false
	}
	for _, expr := range exprs {
		id, ok := expr.(*ast.Ident)
		if !ok || id.Name != "_" {
			return false
		}
	}
	return true
}

func stringLiteral(pass *analysis.Pass, expr ast.Expr) (string, bool) {
	v := pass.TypesInfo.Types[expr].Value
	if v == nil || v.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(v), true
}

// Only a page load's own directory establishes a single applicable route.
// Layout loads and shared helpers intentionally have no route assertion.
func routeParams(filename string) map[string]bool {
	if filepath.Base(filename) != "page.server.go" {
		return nil
	}
	dir := filepath.Dir(filename)
	if filepath.Base(filepath.Dir(dir)) == "links" {
		encoded := strings.ToUpper(filepath.Base(dir))
		decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(encoded)
		if err != nil {
			return nil
		}
		dir = filepath.ToSlash(string(decoded))
	}
	parts := strings.Split(filepath.ToSlash(dir), "/")
	idx := -1
	for i := range parts {
		if parts[i] == "routes" && i > 0 && parts[i-1] == "src" {
			idx = i
		}
	}
	if idx < 0 {
		return nil
	}
	out := make(map[string]bool)
	for _, part := range parts[idx+1:] {
		name := ""
		if strings.HasPrefix(part, "[[") && strings.HasSuffix(part, "]]") {
			name = part[2 : len(part)-2]
		} else if strings.HasPrefix(part, "[...") && strings.HasSuffix(part, "]") {
			name = part[4 : len(part)-1]
		} else if strings.HasPrefix(part, "[") && strings.HasSuffix(part, "]") {
			name = part[1 : len(part)-1]
		} else if strings.ContainsAny(part, "[]") {
			// Mixed route segments need Kit's full route parser. Do not
			// assert that a name is absent when this route may declare it.
			return nil
		}
		if i := strings.IndexByte(name, '='); i >= 0 {
			name = name[:i]
		}
		if name != "" {
			out[name] = true
		}
	}
	return out
}
