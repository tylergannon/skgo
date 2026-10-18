// Package render turns the documentation source tree under documentation/docs
// into the rendered tree the module embeds and the project site serves. It
// resolves the two directive forms the authoring contract allows, include
// fences and reference placeholders, substitutes the toolchain pins, and
// checks everything the contract promises: frontmatter, naming, links,
// anchors, proof names, symbol ownership and the absence of literal pins.
package render

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Problem is one finding of the checker, located in a source file.
type Problem struct {
	File    string // relative to the module root
	Line    int    // 1-based; zero when the problem is the whole file
	Message string
}

func (p Problem) String() string {
	if p.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", p.File, p.Line, p.Message)
	}
	return fmt.Sprintf("%s: %s", p.File, p.Message)
}

// Page is one markdown file of the source tree, index files included.
type Page struct {
	// Rel is the path relative to documentation/docs, which is also the
	// path in the rendered tree: skgo/10-getting-started/10-introduction.md.
	Rel      string
	File     string // relative to the module root
	Division string
	Section  string // the directory name with its prefix; empty for a division index
	Slug     string // the file name without prefix and extension; "index" for an index
	Index    bool
	Title    string
	Summary  string
	Proof    []string
	Body     string // the markdown after the frontmatter
	bodyLine int    // the 1-based line the body starts on
}

// Href is the page's path on the site.
func (p *Page) Href() string {
	if p.Index {
		return "/docs/" + p.Division
	}
	return "/docs/" + p.Division + "/" + p.Slug
}

// Tree is the loaded source tree.
type Tree struct {
	Root  string // the module root, absolute
	Pages []*Page
	// pages by division then slug, index pages under "index"
	bySlug map[string]map[string]*Page
}

// Page finds a page by division and slug.
func (t *Tree) Page(division, slug string) (*Page, bool) {
	p, ok := t.bySlug[division][slug]
	return p, ok
}

// FindRoot walks up from dir to the skgo module root.
func FindRoot(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil && bytes.HasPrefix(data, []byte("module github.com/tylergannon/skgo\n")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no skgo module root above %s", dir)
		}
		dir = parent
	}
}

var (
	prefixed   = regexp.MustCompile(`^(\d\d)-([a-z0-9]+(?:-[a-z0-9]+)*)$`)
	divisions  = map[string]bool{"skgo": true, "cli": true, "ai": true}
	frontmatte = regexp.MustCompile(`(?s)\A---\n(.*?)\n---\n`)
)

// Load reads documentation/docs under root. Structural problems (naming,
// frontmatter) are returned alongside the tree; the tree holds every page
// that could be read.
func Load(root string) (*Tree, []Problem) {
	t := &Tree{Root: root, bySlug: map[string]map[string]*Page{}}
	var problems []Problem
	docs := filepath.Join(root, "documentation", "docs")
	_ = filepath.WalkDir(docs, func(abs string, d fs.DirEntry, err error) error {
		if err != nil {
			problems = append(problems, Problem{File: "documentation/docs", Message: err.Error()})
			return nil
		}
		rel, _ := filepath.Rel(docs, abs)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == "." {
				return nil
			}
			parts := strings.Split(rel, "/")
			switch len(parts) {
			case 1:
				if !divisions[parts[0]] {
					problems = append(problems, Problem{File: "documentation/docs/" + rel, Message: "unknown division; expected skgo, cli or ai"})
				}
			case 2:
				if !prefixed.MatchString(parts[1]) {
					problems = append(problems, Problem{File: "documentation/docs/" + rel, Message: "section directory must be NN-slug"})
				}
			default:
				problems = append(problems, Problem{File: "documentation/docs/" + rel, Message: "pages nest two levels deep at most"})
				return fs.SkipDir
			}
			return nil
		}
		file := "documentation/docs/" + rel
		if !strings.HasSuffix(rel, ".md") {
			problems = append(problems, Problem{File: file, Message: "only .md files belong in the source tree"})
			return nil
		}
		page, ps := loadPage(root, rel)
		problems = append(problems, ps...)
		if page != nil {
			t.Pages = append(t.Pages, page)
		}
		return nil
	})
	sort.Slice(t.Pages, func(i, j int) bool { return t.Pages[i].Rel < t.Pages[j].Rel })
	for _, p := range t.Pages {
		if t.bySlug[p.Division] == nil {
			t.bySlug[p.Division] = map[string]*Page{}
		}
		key := p.Slug
		if p.Index && p.Section != "" {
			key = p.Section + "/index"
		}
		if other, dup := t.bySlug[p.Division][key]; dup {
			problems = append(problems, Problem{File: p.File, Message: fmt.Sprintf("slug %q is already used by %s", p.Slug, other.File)})
			continue
		}
		t.bySlug[p.Division][key] = p
	}
	for division := range divisions {
		if _, ok := t.bySlug[division]["index"]; !ok && len(t.bySlug[division]) > 0 {
			problems = append(problems, Problem{File: "documentation/docs/" + division, Message: "division has no index.md"})
		}
	}
	for _, p := range t.Pages {
		if p.Section != "" && !p.Index {
			if _, ok := t.bySlug[p.Division][p.Section+"/index"]; !ok {
				problems = append(problems, Problem{File: p.File, Message: "section has no index.md"})
			}
		}
	}
	return t, problems
}

func loadPage(root, rel string) (*Page, []Problem) {
	file := "documentation/docs/" + rel
	parts := strings.Split(rel, "/")
	page := &Page{Rel: rel, File: file, Division: parts[0]}
	var problems []Problem
	base := path.Base(rel)
	switch len(parts) {
	case 2:
		if base != "index.md" {
			return nil, []Problem{{File: file, Message: "a division holds sections and index.md only"}}
		}
		page.Index, page.Slug = true, "index"
	case 3:
		page.Section = parts[1]
		if base == "index.md" {
			page.Index, page.Slug = true, "index"
		} else {
			m := prefixed.FindStringSubmatch(strings.TrimSuffix(base, ".md"))
			if m == nil {
				return nil, []Problem{{File: file, Message: "page file must be NN-slug.md"}}
			}
			page.Slug = m[2]
		}
	default:
		return nil, []Problem{{File: file, Message: "pages live at docs/<division>/<NN-section>/<NN-slug>.md"}}
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
	if err != nil {
		return nil, []Problem{{File: file, Message: err.Error()}}
	}
	m := frontmatte.FindSubmatch(data)
	if m == nil {
		return nil, []Problem{{File: file, Line: 1, Message: "missing frontmatter"}}
	}
	page.Body = string(data[len(m[0]):])
	page.bodyLine = bytes.Count(m[0], []byte("\n")) + 1
	var fields map[string]any
	if err := yaml.Unmarshal(m[1], &fields); err != nil {
		return nil, []Problem{{File: file, Line: 1, Message: "frontmatter: " + err.Error()}}
	}
	for key, value := range fields {
		switch key {
		case "title":
			s, _ := value.(string)
			page.Title = strings.TrimSpace(s)
		case "summary":
			s, _ := value.(string)
			page.Summary = strings.TrimSpace(s)
		case "proof":
			list, ok := value.([]any)
			if !ok {
				problems = append(problems, Problem{File: file, Line: 1, Message: "proof must be a list"})
				continue
			}
			for _, item := range list {
				s, _ := item.(string)
				page.Proof = append(page.Proof, s)
			}
		default:
			problems = append(problems, Problem{File: file, Line: 1, Message: fmt.Sprintf("frontmatter key %q is not allowed; use title, summary, proof", key)})
		}
	}
	if page.Title == "" {
		problems = append(problems, Problem{File: file, Line: 1, Message: "frontmatter needs a title"})
	}
	if page.Index {
		if page.Summary != "" || page.Proof != nil {
			problems = append(problems, Problem{File: file, Line: 1, Message: "an index.md has a title only"})
		}
	} else {
		words := len(strings.Fields(page.Summary))
		if words == 0 {
			problems = append(problems, Problem{File: file, Line: 1, Message: "frontmatter needs a summary"})
		} else if words >= 20 {
			problems = append(problems, Problem{File: file, Line: 1, Message: fmt.Sprintf("summary has %d words; keep it under 20", words)})
		}
	}
	return page, problems
}
