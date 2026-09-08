package gen

import (
	"fmt"
	"strings"
)

// planNames chooses the name of every generated handler.
//
// They are named rather than written as anonymous closures because a handler is
// the frame a developer lands in when their remote function panics or their
// debugger stops: `generated.remote_getTodo` says which function and which half
// of it, and an anonymous func literal says `func1`.
//
// The base is the export's own name, which is unique in an app almost always;
// where two modules export the same name the second gets a numeric suffix.
// Generation walks the functions in module-then-name order, so the choice is
// the same on every run.
func (a *app) planNames() {
	taken := map[string]bool{}
	claim := func(base string) string {
		name := base
		for n := 2; taken[name]; n++ {
			name = fmt.Sprintf("%s_%d", base, n)
		}
		taken[name] = true
		return name
	}
	for _, fn := range a.remotes {
		fn.handler = claim("remote_" + fn.name)
		if fn.in != nil && fn.kind != kindForm {
			fn.requestedArg = claim("requestedArg_" + fn.name)
		}
	}
	for _, load := range a.loads {
		load.handler = claim("load_" + loadSlug(load.module))
	}
}

// loadSlug turns a `+*.server.ts` path into a readable identifier fragment:
// the route it answers for, and which of the two files it is.
//
//	src/routes/+layout.server.ts                  -> layout
//	src/routes/account/+layout.server.ts          -> account_layout
//	src/routes/(marketing)/pricing/+page.server.ts -> marketing_pricing_page
func loadSlug(module string) string {
	trimmed := strings.TrimPrefix(module, "src/routes/")
	trimmed = strings.TrimPrefix(trimmed, "src/")
	parts := strings.Split(trimmed, "/")
	var out []string
	for i, part := range parts {
		if i == len(parts)-1 {
			switch part {
			case "+page.server.ts":
				out = append(out, "page")
			case "+layout.server.ts":
				out = append(out, "layout")
			default:
				out = append(out, sanitizeIdent(part))
			}
			continue
		}
		if s := sanitizeIdent(part); s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, "_")
}

// sanitizeIdent keeps the letters and digits of a path segment. A route
// directory may be `(marketing)`, `[id]` or `[...rest]`, none of which Go can
// spell.
func sanitizeIdent(part string) string {
	var b strings.Builder
	for _, r := range part {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		}
	}
	return b.String()
}
