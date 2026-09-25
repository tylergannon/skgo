package advice

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// eventOrigins follows only local, single-write aliases of the registered
// handler's context and EventFrom(ctx). An arbitrary Event or helper call has
// no known request context, so rules 1 and 2 leave it alone.
type eventOrigins struct {
	pass     *analysis.Pass
	context  types.Object
	bindings map[types.Object]ast.Expr
	writes   map[types.Object]int
}

func handlerOrigins(pass *analysis.Pass, h handler) *eventOrigins {
	o := &eventOrigins{pass: pass, context: h.ctx, bindings: map[types.Object]ast.Expr{}, writes: map[types.Object]int{}}
	ast.Inspect(h.fn.Body, func(n ast.Node) bool {
		if _, nested := n.(*ast.FuncLit); nested {
			return false
		}
		switch x := n.(type) {
		case *ast.AssignStmt:
			for i, lhs := range x.Lhs {
				id, ok := lhs.(*ast.Ident)
				if !ok {
					continue
				}
				obj := pass.TypesInfo.Defs[id]
				if obj == nil {
					obj = pass.TypesInfo.Uses[id]
				}
				if obj == nil {
					continue
				}
				o.writes[obj]++
				if x.Tok == token.DEFINE && pass.TypesInfo.Defs[id] != nil && len(x.Rhs) == len(x.Lhs) {
					o.bindings[obj] = x.Rhs[i]
				}
			}
		case *ast.ValueSpec:
			for i, id := range x.Names {
				obj := pass.TypesInfo.Defs[id]
				if obj == nil {
					continue
				}
				o.writes[obj]++
				if len(x.Values) == len(x.Names) {
					o.bindings[obj] = x.Values[i]
				}
			}
		case *ast.IncDecStmt:
			if id, ok := x.X.(*ast.Ident); ok {
				o.writes[pass.TypesInfo.Uses[id]]++
			}
		case *ast.RangeStmt:
			for _, expr := range []ast.Expr{x.Key, x.Value} {
				if id, ok := expr.(*ast.Ident); ok {
					obj := pass.TypesInfo.Defs[id]
					if obj == nil {
						obj = pass.TypesInfo.Uses[id]
					}
					o.writes[obj]++
				}
			}
		}
		return true
	})
	return o
}

func (o *eventOrigins) fromHandler(expr ast.Expr) bool {
	return o.event(expr, map[types.Object]bool{})
}

func (o *eventOrigins) event(expr ast.Expr, seen map[types.Object]bool) bool {
	switch x := expr.(type) {
	case *ast.ParenExpr:
		return o.event(x.X, seen)
	case *ast.Ident:
		return o.binding(x, seen, o.event)
	case *ast.CallExpr:
		return skgoCall(o.pass, x) == "EventFrom" && len(x.Args) == 1 && o.ctx(x.Args[0], seen)
	}
	return false
}

func (o *eventOrigins) ctx(expr ast.Expr, seen map[types.Object]bool) bool {
	switch x := expr.(type) {
	case *ast.ParenExpr:
		return o.ctx(x.X, seen)
	case *ast.Ident:
		obj := o.pass.TypesInfo.Uses[x]
		if obj == o.context && o.writes[obj] == 0 {
			return true
		}
		return o.binding(x, seen, o.ctx)
	case *ast.CallExpr:
		fn := calledFunc(o.pass, x.Fun)
		if fn != nil && fn.Pkg() != nil && fn.Pkg().Path() == "context" && len(x.Args) > 0 {
			switch fn.Name() {
			case "WithValue", "WithCancel", "WithDeadline", "WithTimeout", "WithoutCancel", "WithCancelCause", "WithDeadlineCause", "WithTimeoutCause":
				return o.ctx(x.Args[0], seen)
			}
		}
	}
	return false
}

func (o *eventOrigins) binding(id *ast.Ident, seen map[types.Object]bool, check func(ast.Expr, map[types.Object]bool) bool) bool {
	obj := o.pass.TypesInfo.Uses[id]
	if obj == nil || seen[obj] || o.writes[obj] != 1 {
		return false
	}
	value := o.bindings[obj]
	if value == nil {
		return false
	}
	seen[obj] = true
	return check(value, seen)
}
