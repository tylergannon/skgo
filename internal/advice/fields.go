package advice

import (
	"go/ast"
	"go/types"
	"reflect"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
)

func invalidMethod(pass *analysis.Pass, sel *ast.SelectorExpr) bool {
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
	n, ok := p.Elem().(*types.Named)
	return ok && n.Obj().Name() == "Invalid"
}

func checkIssueLiteral(pass *analysis.Pass, input types.Type, lit *ast.CompositeLit) {
	typ := types.Unalias(pass.TypesInfo.TypeOf(lit))
	n, ok := typ.(*types.Named)
	if !ok || n.Obj().Pkg() == nil || n.Obj().Pkg().Path() != skgoPath || n.Obj().Name() != "Issue" {
		return
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		id, ok := kv.Key.(*ast.Ident)
		if ok && id.Name == "Field" {
			checkField(pass, input, kv.Value)
		}
	}
}

func checkField(pass *analysis.Pass, input types.Type, expr ast.Expr) {
	path, ok := stringLiteral(pass, expr)
	if !ok || path == "" {
		return // Empty path names the form as a whole; dynamic paths remain unresolved.
	}
	valid, suggestion, known := validFieldPath(input, path)
	if !known || valid {
		return
	}
	message := "Form issue field %q is not in the declared input; use a field path from the form input"
	if suggestion != "" {
		message += " (did you mean %q?)"
		report(pass, expr, FormField, message, path, suggestion)
		return
	}
	report(pass, expr, FormField, message+".", path)
}

func validFieldPath(input types.Type, path string) (valid bool, suggestion string, known bool) {
	t := input
	remaining := path
	prefix := ""
	for remaining != "" {
		for {
			t = types.Unalias(t)
			p, ok := t.(*types.Pointer)
			if !ok {
				break
			}
			t = p.Elem()
		}
		if n, ok := t.(*types.Named); ok {
			t = n.Underlying()
		}
		if strings.HasPrefix(remaining, "[") {
			close := strings.IndexByte(remaining, ']')
			if close < 2 {
				return false, "", true
			}
			if _, err := strconv.Atoi(remaining[1:close]); err != nil {
				return false, "", true
			}
			switch x := t.(type) {
			case *types.Slice:
				t = x.Elem()
			case *types.Array:
				t = x.Elem()
			default:
				return false, "", true
			}
			remaining = remaining[close+1:]
			if strings.HasPrefix(remaining, ".") {
				remaining = remaining[1:]
			}
			continue
		}
		st, ok := t.(*types.Struct)
		if !ok {
			_, dynamicMap := t.(*types.Map)
			_, dynamicInterface := t.(*types.Interface)
			return false, "", !dynamicMap && !dynamicInterface
		}
		segment := remaining
		if i := strings.IndexAny(remaining, ".["); i >= 0 {
			segment = remaining[:i]
		}
		if segment == "" {
			return false, "", true
		}
		var next types.Type
		options := make([]string, 0, st.NumFields())
		for i := 0; i < st.NumFields(); i++ {
			field := st.Field(i)
			if !field.Exported() {
				continue
			}
			name := field.Name()
			if tag := reflect.StructTag(st.Tag(i)).Get("json"); tag != "" {
				name = strings.Split(tag, ",")[0]
			}
			if name == "-" || name == "" {
				continue
			}
			options = append(options, name)
			if name == segment {
				next = field.Type()
			}
		}
		if next == nil {
			candidate := nearest(segment, options)
			if candidate != "" {
				return false, prefix + candidate + remaining[len(segment):], true
			}
			return false, "", true
		}
		t = next
		prefix += segment
		remaining = remaining[len(segment):]
		if strings.HasPrefix(remaining, ".") {
			prefix += "."
			remaining = remaining[1:]
			if remaining == "" {
				return false, "", true
			}
		} else if strings.HasPrefix(remaining, "[") {
			end := strings.IndexByte(remaining, ']')
			if end > 0 {
				prefix += remaining[:end+1]
			}
		}
	}
	return true, "", true
}

func nearest(want string, choices []string) string {
	best := ""
	bestDistance := 3
	for _, choice := range choices {
		d := editDistance(want, choice)
		if d < bestDistance {
			best, bestDistance = choice, d
		} else if d == bestDistance {
			best = "" // An ambiguous repair is not a useful suggestion.
		}
	}
	return best
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := range a {
		curr := make([]int, len(b)+1)
		curr[0] = i + 1
		for j := range b {
			cost := 1
			if a[i] == b[j] {
				cost = 0
			}
			curr[j+1] = min(curr[j]+1, prev[j+1]+1, prev[j]+cost)
		}
		prev = curr
	}
	return prev[len(b)]
}
