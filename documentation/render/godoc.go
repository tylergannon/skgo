package render

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/build"
	"go/doc"
	"go/doc/comment"
	"go/parser"
	"go/printer"
	"go/token"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const modulePath = "github.com/tylergannon/skgo"

// apiPackage is one Go package the reference documents.
type apiPackage struct {
	ImportPath string
	doc        *doc.Package
	fset       *token.FileSet
	files      []*ast.File
	// symbols by name: top-level funcs, types and values plus the funcs and
	// values go/doc associates with a type (those map to the type's name as
	// owner).
	symbols map[string]*symbol
}

// symbol is an exported top-level name of a package.
type symbol struct {
	Name  string
	Kind  string // func, type, const, var
	Owner string // the top-level name whose entry renders this symbol (itself, or its type)
	fn    *doc.Func
	typ   *doc.Type
	val   *doc.Value
}

// loadPackage parses the package at importPath under root, honouring build
// constraints for the current platform, and returns its go/doc view.
func loadPackage(root, importPath string) (*apiPackage, error) {
	if importPath != modulePath && !strings.HasPrefix(importPath, modulePath+"/") {
		return nil, fmt.Errorf("package %s is outside the module", importPath)
	}
	dir := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(importPath, modulePath)))
	bp, err := build.Default.ImportDir(dir, 0)
	if err != nil {
		return nil, fmt.Errorf("package %s: %w", importPath, err)
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, name := range bp.GoFiles {
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	dp, err := doc.NewFromFiles(fset, files, importPath)
	if err != nil {
		return nil, fmt.Errorf("package %s: %w", importPath, err)
	}
	p := &apiPackage{ImportPath: importPath, doc: dp, fset: fset, files: files, symbols: map[string]*symbol{}}
	add := func(s *symbol) { p.symbols[s.Name] = s }
	for _, f := range dp.Funcs {
		add(&symbol{Name: f.Name, Kind: "func", Owner: f.Name, fn: f})
	}
	for _, t := range dp.Types {
		add(&symbol{Name: t.Name, Kind: "type", Owner: t.Name, typ: t})
		// go/doc files a function under the type it returns. For a marker
		// type with no fields that association is formal: Load, Query and
		// GET return Marker, and each is its own entry.
		if isEmptyStruct(t) {
			for _, f := range t.Funcs {
				add(&symbol{Name: f.Name, Kind: "func", Owner: f.Name, fn: f})
			}
			t.Funcs = nil
			continue
		}
		for _, f := range t.Funcs {
			add(&symbol{Name: f.Name, Kind: "func", Owner: t.Name, fn: f})
		}
		for _, v := range append(append([]*doc.Value{}, t.Consts...), t.Vars...) {
			for _, name := range v.Names {
				if ast.IsExported(name) {
					add(&symbol{Name: name, Kind: kindOf(v), Owner: t.Name, val: v})
				}
			}
		}
	}
	for _, v := range append(append([]*doc.Value{}, dp.Consts...), dp.Vars...) {
		for _, name := range v.Names {
			if ast.IsExported(name) {
				add(&symbol{Name: name, Kind: kindOf(v), Owner: v.Names[0], val: v})
			}
		}
	}
	return p, nil
}

func isEmptyStruct(t *doc.Type) bool {
	for _, spec := range t.Decl.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		st, ok := ts.Type.(*ast.StructType)
		if ok && (st.Fields == nil || len(st.Fields.List) == 0) {
			return true
		}
	}
	return false
}

func kindOf(v *doc.Value) string {
	if v.Decl.Tok == token.CONST {
		return "const"
	}
	return "var"
}

// owners lists the top-level names whose entries render something: every
// exported func, type and value block, sorted.
func (p *apiPackage) owners() []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range p.symbols {
		if !seen[s.Owner] {
			seen[s.Owner] = true
			out = append(out, s.Owner)
		}
	}
	sort.Strings(out)
	return out
}

// symbolAnchor is where a symbol renders: the page href and the heading id.
type symbolAnchor struct {
	Href string
	ID   string
}

// linkResolver maps a doc link to a URL, or "" to leave plain text.
type linkResolver func(importPath, recv, name string) string

// renderEntry renders the owner's entry: an h2, the declaration, the doc
// comment and, for a type, its constructors, methods and values under h3.
func (p *apiPackage) renderEntry(owner string, resolve linkResolver) (string, error) {
	s, ok := p.symbols[owner]
	if !ok {
		return "", fmt.Errorf("package %s has no exported %s", p.ImportPath, owner)
	}
	var b strings.Builder
	switch {
	case s.fn != nil && s.Owner == s.Name && s.fn.Recv == "":
		if err := p.writeFunc(&b, "## ", s.fn, resolve); err != nil {
			return "", err
		}
	case s.typ != nil:
		t := s.typ
		fmt.Fprintf(&b, "## %s\n\n", t.Name)
		if err := p.writeDecl(&b, t.Decl); err != nil {
			return "", err
		}
		b.WriteString(p.markdown(t.Doc, resolve))
		for _, v := range t.Consts {
			p.writeValue(&b, "### ", v, resolve)
		}
		for _, v := range t.Vars {
			p.writeValue(&b, "### ", v, resolve)
		}
		for _, f := range t.Funcs {
			if err := p.writeFunc(&b, "### ", f, resolve); err != nil {
				return "", err
			}
		}
		for _, m := range t.Methods {
			if err := p.writeFunc(&b, "### ", m, resolve); err != nil {
				return "", err
			}
		}
	case s.val != nil:
		p.writeValue(&b, "## ", s.val, resolve)
	default:
		return "", fmt.Errorf("%s.%s belongs to type %s; list the type instead", p.ImportPath, owner, s.Owner)
	}
	return b.String(), nil
}

// anchors lists the heading ids an owner's entry produces, keyed by symbol
// name (methods as Type.Method), so doc links can target them.
func (p *apiPackage) anchors(owner string) map[string]string {
	out := map[string]string{}
	s := p.symbols[owner]
	if s == nil {
		return out
	}
	switch {
	case s.typ != nil:
		t := s.typ
		top := slugify(t.Name)
		out[t.Name] = top
		for _, f := range t.Funcs {
			out[f.Name] = top + "-" + slugify(f.Name)
		}
		for _, m := range t.Methods {
			out[t.Name+"."+m.Name] = top + "-" + slugify(m.Name)
		}
		for _, v := range append(append([]*doc.Value{}, t.Consts...), t.Vars...) {
			id := top + "-" + slugify(valueHeading(v))
			for _, name := range v.Names {
				out[name] = id
			}
		}
	case s.val != nil:
		id := slugify(valueHeading(s.val))
		for _, name := range s.val.Names {
			out[name] = id
		}
	case s.fn != nil:
		out[s.fn.Name] = slugify(s.fn.Name)
	}
	return out
}

func valueHeading(v *doc.Value) string {
	names := v.Names
	if len(names) > 3 {
		return names[0] + ", …"
	}
	return strings.Join(names, ", ")
}

func (p *apiPackage) writeFunc(b *strings.Builder, prefix string, f *doc.Func, resolve linkResolver) error {
	fmt.Fprintf(b, "%s%s\n\n", prefix, f.Name)
	decl := *f.Decl
	decl.Body = nil
	if err := p.writeDecl(b, &decl); err != nil {
		return err
	}
	b.WriteString(p.markdown(f.Doc, resolve))
	return nil
}

func (p *apiPackage) writeValue(b *strings.Builder, prefix string, v *doc.Value, resolve linkResolver) {
	fmt.Fprintf(b, "%s%s\n\n", prefix, valueHeading(v))
	_ = p.writeDecl(b, v.Decl)
	b.WriteString(p.markdown(v.Doc, resolve))
}

// writeDecl prints a declaration, with the comments inside it, in a go fence.
func (p *apiPackage) writeDecl(b *strings.Builder, node ast.Node) error {
	var comments []*ast.CommentGroup
	for _, f := range p.files {
		if f.FileStart <= node.Pos() && node.Pos() <= f.FileEnd {
			comments = f.Comments
			break
		}
	}
	var buf bytes.Buffer
	cfg := printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 8}
	if err := cfg.Fprint(&buf, p.fset, &printer.CommentedNode{Node: node, Comments: comments}); err != nil {
		return err
	}
	fmt.Fprintf(b, "```go\n%s\n```\n\n", strings.TrimRight(buf.String(), "\n"))
	return nil
}

// markdown renders a doc comment with go/doc/comment, headings at h4 so they
// sit under the symbol's heading, and doc links resolved to pages.
func (p *apiPackage) markdown(text string, resolve linkResolver) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	parsed := p.doc.Parser().Parse(text)
	pr := p.doc.Printer()
	pr.HeadingLevel = 4
	pr.DocLinkURL = func(link *comment.DocLink) string {
		return resolve(link.ImportPath, link.Recv, link.Name)
	}
	return strings.TrimRight(tidyMarkdown(string(pr.Markdown(parsed))), "\n") + "\n\n"
}

var codeSpan = regexp.MustCompile("`[^`\n]+`")

// tidyMarkdown adjusts go/doc/comment's markdown for the site. The printer
// escapes backquotes, but skgo's doc comments use them as inline code, so
// they are restored and the escapes inside them undone; and its indented
// code blocks become go fences, since a doc comment's code is Go.
func tidyMarkdown(md string) string {
	md = strings.ReplaceAll(md, "\\`", "`")
	md = codeSpan.ReplaceAllStringFunc(md, func(span string) string {
		return strings.NewReplacer("\\_", "_", "\\*", "*", "\\[", "[", "\\]", "]", "\\<", "<", "\\>", ">").Replace(span)
	})
	lines := strings.Split(md, "\n")
	var out []string
	for i := 0; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], "\t") {
			out = append(out, lines[i])
			continue
		}
		var block []string
		for i < len(lines) && (strings.HasPrefix(lines[i], "\t") || strings.TrimSpace(lines[i]) == "") {
			block = append(block, strings.TrimPrefix(lines[i], "\t"))
			i++
		}
		i--
		for len(block) > 0 && strings.TrimSpace(block[len(block)-1]) == "" {
			block = block[:len(block)-1]
		}
		out = append(out, "```go")
		out = append(out, block...)
		out = append(out, "```")
		if i+1 < len(lines) && strings.TrimSpace(lines[i+1]) != "" {
			out = append(out, "")
		}
	}
	return strings.Join(out, "\n")
}
