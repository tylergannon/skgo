package render

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/tylergannon/skgo/internal/toolchain"
)

// Output is the rendered tree: file contents by path relative to
// documentation/rendered.
type Output map[string][]byte

// Render loads, checks and renders the documentation under root. Every
// problem the checker finds is returned; the output is complete even when
// problems exist, so a caller can show what a broken page would render as,
// but callers that publish must refuse on any problem.
func Render(root string) (Output, []Problem) {
	tree, problems := Load(root)
	out := Output{}
	pins := toolchain.Current()
	versions, _ := json.MarshalIndent(pins, "", "  ")
	out["versions.json"] = append(versions, '\n')

	// Reference placeholders, packages and ownership first: every page's
	// anchors must be known before any page's doc links render.
	pageRefs := map[*Page][]reference{}
	packages := map[string]*apiPackage{}
	owned := map[string]map[string]*Page{} // import path -> owner -> page
	for _, page := range tree.Pages {
		refs, ps := references(page)
		problems = append(problems, ps...)
		pageRefs[page] = refs
		for _, ref := range refs {
			pkg, ok := packages[ref.ImportPath]
			if !ok {
				var err error
				pkg, err = loadPackage(root, ref.ImportPath)
				if err != nil {
					problems = append(problems, Problem{File: page.File, Line: page.bodyLine + ref.Line, Message: err.Error()})
					continue
				}
				packages[ref.ImportPath] = pkg
				owned[ref.ImportPath] = map[string]*Page{}
			}
			names := ref.Symbols
			if names == nil {
				names = pkg.owners()
			}
			for _, name := range names {
				s, ok := pkg.symbols[name]
				if !ok {
					problems = append(problems, Problem{File: page.File, Line: page.bodyLine + ref.Line, Message: fmt.Sprintf("package %s has no exported %s", ref.ImportPath, name)})
					continue
				}
				if s.Owner != name {
					problems = append(problems, Problem{File: page.File, Line: page.bodyLine + ref.Line, Message: fmt.Sprintf("%s renders under %s; list %s instead", name, s.Owner, s.Owner)})
					continue
				}
				if other, dup := owned[ref.ImportPath][name]; dup {
					problems = append(problems, Problem{File: page.File, Line: page.bodyLine + ref.Line, Message: fmt.Sprintf("%s is already documented on %s", name, other.File)})
					continue
				}
				owned[ref.ImportPath][name] = page
			}
		}
	}
	for importPath, pkg := range packages {
		var missing []string
		for _, owner := range pkg.owners() {
			if owned[importPath][owner] == nil {
				missing = append(missing, owner)
			}
		}
		if len(missing) > 0 {
			problems = append(problems, Problem{File: "documentation/docs", Message: fmt.Sprintf("package %s: no page documents %s", importPath, strings.Join(missing, ", "))})
		}
	}
	// symbol -> anchor, per package
	anchors := map[string]map[string]symbolAnchor{}
	for importPath, pkg := range packages {
		anchors[importPath] = map[string]symbolAnchor{}
		for owner, page := range owned[importPath] {
			for name, id := range pkg.anchors(owner) {
				anchors[importPath][name] = symbolAnchor{Href: page.Href(), ID: id}
			}
		}
	}
	resolver := func(from *Page, pkg *apiPackage) linkResolver {
		return func(importPath, recv, name string) string {
			if importPath == "" {
				importPath = pkg.ImportPath
			}
			key := name
			if recv != "" {
				key = recv + "." + name
			}
			if a, ok := anchors[importPath][key]; ok {
				if a.Href == from.Href() {
					return "#" + a.ID
				}
				return a.Href + "#" + a.ID
			}
			if _, local := packages[importPath]; local {
				return ""
			}
			if name == "" {
				return "https://pkg.go.dev/" + importPath
			}
			return "https://pkg.go.dev/" + importPath + "#" + key
		}
	}

	tests := testIndex(root)
	rendered := map[*Page]string{}
	for _, page := range tree.Pages {
		problems = append(problems, checkPins(page, pins)...)
		problems = append(problems, checkProof(root, page, tests)...)
		body, ps := expandIncludes(root, page)
		problems = append(problems, ps...)
		// Reference placeholders: replaced by line index, bottom-up so
		// earlier indexes stay valid. Includes never add or remove lines
		// outside fences, but they do change the count, so re-find.
		refs, _ := references(&Page{Body: body, File: page.File, bodyLine: page.bodyLine})
		lines := strings.Split(body, "\n")
		for i := len(refs) - 1; i >= 0; i-- {
			ref := refs[i]
			pkg := packages[ref.ImportPath]
			if pkg == nil {
				continue
			}
			names := ref.Symbols
			if names == nil {
				names = pkg.owners()
			}
			var parts []string
			for _, name := range names {
				if owned[ref.ImportPath][name] != page {
					continue
				}
				entry, err := pkg.renderEntry(name, resolver(page, pkg))
				if err != nil {
					problems = append(problems, Problem{File: page.File, Line: page.bodyLine + ref.Line, Message: err.Error()})
					continue
				}
				parts = append(parts, strings.TrimRight(entry, "\n"))
			}
			replacement := strings.Split(strings.Join(parts, "\n\n"), "\n")
			lines = append(lines[:ref.Line], append(replacement, lines[ref.Line+ref.span:]...)...)
		}
		body = strings.Join(lines, "\n")
		body, ps = expandCommands(page, body)
		problems = append(problems, ps...)
		body = substitutePins(body, pins)
		rendered[page] = body
	}
	for _, page := range tree.Pages {
		problems = append(problems, checkLinks(tree, page, rendered)...)
		var b strings.Builder
		b.WriteString("---\n")
		fmt.Fprintf(&b, "title: %s\n", yamlString(substitutePins(page.Title, pins)))
		if page.Summary != "" {
			fmt.Fprintf(&b, "summary: %s\n", yamlString(substitutePins(page.Summary, pins)))
		}
		b.WriteString("---\n")
		b.WriteString(rendered[page])
		if !strings.HasSuffix(rendered[page], "\n") {
			b.WriteString("\n")
		}
		out[page.Rel] = []byte(b.String())
	}
	sort.Slice(problems, func(i, j int) bool {
		if problems[i].File != problems[j].File {
			return problems[i].File < problems[j].File
		}
		return problems[i].Line < problems[j].Line
	})
	return out, problems
}

func yamlString(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}

var placeholder = regexp.MustCompile(`%(kit|svelte|go|node|vp|pnpm)%`)

func substitutePins(s string, pins toolchain.Pins) string {
	return placeholder.ReplaceAllStringFunc(s, func(m string) string {
		switch m {
		case "%kit%":
			return pins.Kit
		case "%svelte%":
			return pins.Svelte
		case "%go%":
			return pins.Go
		case "%node%":
			return pins.NodeMinimum
		case "%vp%":
			return pins.VitePlus
		case "%pnpm%":
			return pins.PNPM
		}
		return m
	})
}

// checkPins fails a source page that writes a pinned version as a literal.
func checkPins(page *Page, pins toolchain.Pins) []Problem {
	var problems []Problem
	values := map[string]string{pins.Kit: "%kit%", pins.Svelte: "%svelte%", pins.Go: "%go%", pins.NodeMinimum: "%node%", pins.VitePlus: "%vp%", pins.PNPM: "%pnpm%"}
	for i, line := range strings.Split(page.Body, "\n") {
		for value, name := range values {
			re := regexp.MustCompile(`(^|[^0-9.\w])` + regexp.QuoteMeta(value) + `([^0-9.\w]|$)`)
			if re.MatchString(line) {
				problems = append(problems, Problem{File: page.File, Line: page.bodyLine + i, Message: fmt.Sprintf("literal version %s; write %s", value, name)})
			}
		}
	}
	return problems
}

var testFunc = regexp.MustCompile(`(?m)^func (Test\w+)\(`)

// testIndex lists every Go test function name in the module and its example.
func testIndex(root string) map[string]bool {
	names := map[string]bool{}
	skip := map[string]bool{".git": true, "node_modules": true, "ephemeral": true, ".claude": true, ".svelte-kit": true, "rendered": true, "testdata": true}
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skip[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		for _, m := range testFunc.FindAllSubmatch(data, -1) {
			names[string(m[1])] = true
		}
		return nil
	})
	return names
}

func checkProof(root string, page *Page, tests map[string]bool) []Problem {
	var problems []Problem
	for _, name := range page.Proof {
		switch {
		case strings.HasSuffix(name, ".feature"):
			candidates := []string{name, "example/e2e/features/" + name}
			found := false
			for _, c := range candidates {
				if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(c))); err == nil {
					found = true
					break
				}
			}
			if !found {
				problems = append(problems, Problem{File: page.File, Line: 1, Message: fmt.Sprintf("proof %s: no such feature file", name)})
			}
		case strings.HasPrefix(name, "Test"):
			if !tests[name] {
				problems = append(problems, Problem{File: page.File, Line: 1, Message: fmt.Sprintf("proof %s: no such Go test", name)})
			}
		default:
			problems = append(problems, Problem{File: page.File, Line: 1, Message: fmt.Sprintf("proof %q: name a feature file or a Go test function", name)})
		}
	}
	return problems
}

var (
	mdLink     = regexp.MustCompile(`\[[^\]\n]*\]\(([^)\s"']+)(?:\s+"[^"]*")?\)`)
	inlineCode = regexp.MustCompile("`[^`\n]*`")
)

// checkLinks verifies every relative and /docs/ link of a rendered page and
// its anchor.
func checkLinks(tree *Tree, page *Page, rendered map[*Page]string) []Problem {
	var problems []Problem
	ids := map[*Page]map[string]bool{}
	anchorsOf := func(p *Page) map[string]bool {
		if ids[p] == nil {
			ids[p] = headingIDs(rendered[p])
		}
		return ids[p]
	}
	inFence := false
	for i, line := range strings.Split(rendered[page], "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		line = inlineCode.ReplaceAllString(line, "")
		for _, m := range mdLink.FindAllStringSubmatch(line, -1) {
			target := m[1]
			problem := func(format string, args ...any) {
				problems = append(problems, Problem{File: page.File, Line: page.bodyLine + i, Message: fmt.Sprintf(format, args...)})
			}
			switch {
			case strings.HasPrefix(target, "http://"), strings.HasPrefix(target, "https://"), strings.HasPrefix(target, "mailto:"):
				continue
			case strings.HasPrefix(target, "#"):
				if !anchorsOf(page)[target[1:]] {
					problem("link %s: no such heading on this page", target)
				}
			case strings.HasPrefix(target, "/docs/"):
				rest := strings.TrimPrefix(target, "/docs/")
				slugPart, anchor, _ := strings.Cut(rest, "#")
				division, slug, hasSlug := strings.Cut(slugPart, "/")
				if !hasSlug {
					slug = "index"
				}
				other, ok := tree.Page(division, slug)
				if !ok {
					problem("link %s: no such page", target)
					continue
				}
				if anchor != "" && !anchorsOf(other)[anchor] {
					problem("link %s: no such heading on %s", target, other.File)
				}
			case strings.HasPrefix(target, "/"):
				continue // another route on the site
			default:
				slug, anchor, _ := strings.Cut(target, "#")
				if strings.ContainsAny(slug, "/.") {
					problem("link %s: link to a page by its slug, to another division by /docs/<division>/<slug>", target)
					continue
				}
				other, ok := tree.Page(page.Division, slug)
				if !ok {
					problem("link %s: no page with that slug in the %s division", target, page.Division)
					continue
				}
				if anchor != "" && !anchorsOf(other)[anchor] {
					problem("link %s: no such heading on %s", target, other.File)
				}
			}
		}
	}
	return problems
}

// Write replaces dir with the output: files are written and stale files
// removed, so a renamed page leaves nothing behind.
func Write(dir string, out Output) error {
	existing := map[string]bool{}
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			existing[filepath.ToSlash(rel)] = true
		}
		return nil
	})
	for rel, data := range out {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			return err
		}
		delete(existing, rel)
	}
	for rel := range existing {
		if err := os.Remove(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			return err
		}
	}
	// drop directories left empty
	var dirs []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() && p != dir {
			dirs = append(dirs, p)
		}
		return nil
	})
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err == nil && len(entries) == 0 {
			_ = os.Remove(d)
		}
	}
	return nil
}

// Drift compares the output to the files under dir and reports every path
// that differs, is missing or is stale.
func Drift(dir string, out Output) []string {
	var drift []string
	seen := map[string]bool{}
	for rel, data := range out {
		seen[rel] = true
		disk, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			drift = append(drift, rel+": not rendered")
			continue
		}
		if string(disk) != string(data) {
			drift = append(drift, rel+": differs from a fresh render")
		}
	}
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		if !seen[filepath.ToSlash(rel)] {
			drift = append(drift, filepath.ToSlash(rel)+": stale; no source renders it")
		}
		return nil
	})
	sort.Strings(drift)
	return drift
}
