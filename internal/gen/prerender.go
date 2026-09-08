package gen

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

type prerenderValue string

const (
	prerenderFalse   prerenderValue = "false"
	prerenderTrue    prerenderValue = "true"
	prerenderAuto    prerenderValue = "auto"
	prerenderUnknown prerenderValue = "unknown"
)

// checkPrerenderedLoads mirrors kit's PageNodes.prerender reduction. A page
// inherits the last option set by its selected layouts and then its leaf; at
// each node the universal module wins over the server module. Both true and
// 'auto' can enter kit's prerender crawl, so neither can have a Go load in the
// selected branch until #81 makes that load callable during the build.
func (a *app) checkPrerenderedLoads() error {
	routes := filepath.Join(a.cfg.Web, filepath.FromSlash(routesDir))
	pages, err := pageDirs(routes)
	if err != nil {
		return err
	}

	loads := make(map[string]*loadFn, len(a.loads))
	for _, load := range a.loads {
		loads[filepath.Clean(load.stub)] = load
	}

	for _, page := range pages {
		branch, err := selectedLayouts(routes, page, loads)
		if err != nil {
			return err
		}

		value := prerenderFalse
		var branchLoads []*loadFn
		for _, dir := range branch {
			if option, ok, err := nodePrerender(dir, "+layout"); err != nil {
				return err
			} else if ok {
				value = option
			}
			if load := loads[filepath.Join(dir, "+layout.server.ts")]; load != nil {
				branchLoads = append(branchLoads, load)
			}
		}
		if option, ok, err := nodePrerender(page, "+page"); err != nil {
			return err
		} else if ok {
			value = option
		}
		if load := loads[filepath.Join(page, "+page.server.ts")]; load != nil {
			branchLoads = append(branchLoads, load)
		}

		if value == prerenderFalse || value == prerenderUnknown || len(branchLoads) == 0 {
			continue
		}
		route, err := filepath.Rel(routes, page)
		if err != nil {
			return err
		}
		route = filepath.ToSlash(route)
		if route == "." {
			route = "/"
		} else {
			route = "/" + route
		}
		return fmt.Errorf("%s", prerenderLoadMessage(route, branchLoads[0].source))
	}
	return nil
}

func prerenderLoadMessage(route, source string) string {
	return fmt.Sprintf("skgo: route %s is prerendered, and its branch has a Go server load at %s; skgo cannot answer a load while kit prerenders (#81). Remove the prerender or move the load", route, source)
}

// pageDirs finds kit page nodes, including pages whose component selects a
// named layout. Module-only pages are page nodes too.
func pageDirs(routes string) ([]string, error) {
	seen := map[string]bool{}
	err := filepath.WalkDir(routes, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if isPageFile(d.Name()) {
			seen[filepath.Dir(path)] = true
		}
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	pages := make([]string, 0, len(seen))
	for page := range seen {
		pages = append(pages, page)
	}
	sort.Strings(pages)
	return pages, nil
}

func isPageFile(name string) bool {
	if name == "+page.ts" || name == "+page.js" || name == "+page.server.ts" || name == "+page.server.js" {
		return true
	}
	return strings.HasPrefix(name, "+page") && strings.HasSuffix(name, ".svelte") && (name == "+page.svelte" || strings.HasPrefix(name, "+page@"))
}

// selectedLayouts follows kit's named-layout parent links before applying the
// option reduction. With no @ selector this is simply every ancestor layout.
func selectedLayouts(routes, page string, loads map[string]*loadFn) ([]string, error) {
	var ancestors []string
	for dir := page; ; dir = filepath.Dir(dir) {
		ancestors = append(ancestors, dir)
		if dir == routes {
			break
		}
		if !withinTree(routes, dir) || dir == filepath.Dir(dir) {
			return nil, fmt.Errorf("skgo: page %s is outside the route tree %s", page, routes)
		}
	}

	parent, err := layoutSelector(page, "+page")
	if err != nil {
		return nil, err
	}
	var selected []string
	for _, dir := range ancestors {
		segment := filepath.Base(dir)
		if dir == routes {
			segment = ""
		}
		if parent != nil && segment != *parent {
			continue
		}
		if hasLayout(dir, loads) {
			selected = append(selected, dir)
			parent, err = layoutSelector(dir, "+layout")
			if err != nil {
				return nil, err
			}
		} else {
			parent = nil
		}
	}
	for i, j := 0, len(selected)-1; i < j; i, j = i+1, j-1 {
		selected[i], selected[j] = selected[j], selected[i]
	}
	return selected, nil
}

func hasLayout(dir string, loads map[string]*loadFn) bool {
	if loads[filepath.Join(dir, "+layout.server.ts")] != nil {
		return true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == "+layout.ts" || name == "+layout.js" || name == "+layout.server.ts" || name == "+layout.server.js" ||
			name == "+layout.svelte" || strings.HasPrefix(name, "+layout@") && strings.HasSuffix(name, ".svelte") {
			return true
		}
	}
	return false
}

func layoutSelector(dir, prefix string) (*string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".svelte") || !strings.HasPrefix(name, prefix+"@") {
			continue
		}
		selector := strings.TrimSuffix(strings.TrimPrefix(name, prefix+"@"), ".svelte")
		return &selector, nil
	}
	return nil, nil
}

// nodePrerender follows kit's within-node precedence: universal first, then
// server. A missing export is inherited from the preceding branch node.
func nodePrerender(dir, prefix string) (prerenderValue, bool, error) {
	for _, suffix := range []string{".ts", ".js", ".server.ts", ".server.js"} {
		value, ok, err := sourcePrerender(filepath.Join(dir, prefix+suffix))
		if err != nil {
			return "", false, err
		}
		if ok {
			return value, true, nil
		}
	}
	return "", false, nil
}

func sourcePrerender(path string) (prerenderValue, bool, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	tokens := jsTokens(string(raw))
	for i := 0; i+3 < len(tokens); i++ {
		if tokens[i] != "export" {
			continue
		}
		if tokens[i+1] == "*" {
			return prerenderUnknown, true, nil
		}
		if tokens[i+1] == "{" {
			for j := i + 2; j < len(tokens) && tokens[j] != "}"; j++ {
				if tokens[j] == "prerender" {
					return prerenderUnknown, true, nil
				}
			}
			continue
		}
		if tokens[i+1] != "const" || tokens[i+2] != "prerender" {
			continue
		}
		for i += 3; i < len(tokens) && tokens[i] != ";"; i++ {
			if tokens[i] != "=" || i+1 == len(tokens) {
				continue
			}
			switch tokens[i+1] {
			case "true":
				return prerenderTrue, true, nil
			case "false":
				return prerenderFalse, true, nil
			case "'auto'", "\"auto\"", "`auto`":
				return prerenderAuto, true, nil
			default:
				return prerenderUnknown, true, nil
			}
		}
		return prerenderUnknown, true, nil
	}
	return "", false, nil
}

// jsTokens is the small lexical surface needed for a literal page option. It
// skips comments and preserves quoted literals so examples in comments and
// strings cannot be mistaken for declarations.
func jsTokens(source string) []string {
	var tokens []string
	for i := 0; i < len(source); {
		if unicode.IsSpace(rune(source[i])) {
			i++
			continue
		}
		if i+1 < len(source) && source[i:i+2] == "//" {
			if end := strings.IndexByte(source[i+2:], '\n'); end >= 0 {
				i += end + 3
			} else {
				break
			}
			continue
		}
		if i+1 < len(source) && source[i:i+2] == "/*" {
			if end := strings.Index(source[i+2:], "*/"); end >= 0 {
				i += end + 4
			} else {
				break
			}
			continue
		}
		if source[i] == '\'' || source[i] == '"' || source[i] == '`' {
			start, quote := i, source[i]
			i++
			for i < len(source) {
				if source[i] == '\\' {
					if i+1 < len(source) {
						i += 2
					} else {
						i++
					}
					continue
				}
				i++
				if source[i-1] == quote {
					break
				}
			}
			tokens = append(tokens, source[start:i])
			continue
		}
		if isJSIdent(source[i]) {
			start := i
			for i < len(source) && isJSIdent(source[i]) {
				i++
			}
			tokens = append(tokens, source[start:i])
			continue
		}
		tokens = append(tokens, source[i:i+1])
		i++
	}
	return tokens
}

func isJSIdent(c byte) bool {
	return c == '_' || c == '$' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}
