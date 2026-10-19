// Package tailwind supplies optional development-time Tailwind setup through
// the same add-on operations used by application templates.
package tailwind

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/scanner"
	"unicode"

	"github.com/tylergannon/skgo/templateapi"
	"golang.org/x/mod/semver"
)

const version = "4.3.3"

type plugin struct {
	version string
	install func(context.Context, string) error
}

func Plugin(skgoVersion string) templateapi.Plugin {
	return plugin{version: skgoVersion, install: install}
}
func (p plugin) Describe() templateapi.Descriptor {
	return templateapi.Descriptor{ID: "skgo-tailwind", Version: p.version, SkgoVersion: p.version, Addons: []templateapi.Addon{{Name: "tailwindcss", Summary: "Add Tailwind CSS with Go-safe source scanning", Options: []templateapi.Option{
		{Name: "layout", Help: "Root layout relative to the frontend root", Default: "src/routes/+layout.svelte"},
		{Name: "stylesheet", Help: "Stylesheet relative to the frontend root; defaults to the layout's imported CSS or layout.css"},
	}}}}
}

func (p plugin) Apply(ctx context.Context, req templateapi.Request) (result templateapi.Result, err error) {
	if req.Addon != "tailwindcss" {
		return result, fmt.Errorf("unknown Tailwind add-on %q", req.Addon)
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	web := req.Project.Web
	if relative, err := filepath.Rel(req.Project.Root, web); err != nil || !filepath.IsLocal(relative) {
		return result, fmt.Errorf("frontend root must be inside the application root")
	}
	layout := req.Options["layout"]
	if layout == "" {
		layout = "src/routes/+layout.svelte"
	}
	if !filepath.IsLocal(layout) || filepath.Ext(layout) != ".svelte" {
		return result, fmt.Errorf("layout must name a Svelte file inside the frontend root")
	}
	layoutPath := filepath.Join(web, layout)
	layoutSource, err := readOptional(layoutPath)
	if err != nil {
		return result, err
	}
	stylesheet := req.Options["stylesheet"]
	if stylesheet == "" {
		imports := stylesheetImports(string(layoutSource))
		if len(imports) > 1 {
			return result, fmt.Errorf("%s imports multiple stylesheets; select the Tailwind stylesheet with --set stylesheet=PATH", layout)
		}
		if len(imports) == 1 {
			if !strings.HasPrefix(imports[0], ".") {
				return result, fmt.Errorf("%s has a stylesheet alias; select its frontend-relative path with --set stylesheet=PATH", layout)
			}
			stylesheet = filepath.Join(filepath.Dir(layout), filepath.FromSlash(imports[0]))
		} else {
			stylesheet = filepath.Join(filepath.Dir(layout), "layout.css")
		}
	}
	if !filepath.IsLocal(stylesheet) || filepath.Ext(stylesheet) != ".css" {
		return result, fmt.Errorf("stylesheet must name a CSS file inside the frontend root")
	}
	cssPath := filepath.Join(web, stylesheet)
	css, err := readOptional(cssPath)
	if err != nil {
		return result, err
	}
	if legacyTailwind.MatchString(withoutCSSComments(string(css))) {
		return result, fmt.Errorf("%s uses Tailwind 3 directives; migrate it to Tailwind 4 before applying this add-on", stylesheet)
	}
	vitePath := filepath.Join(web, "vite.config.ts")
	vite, err := os.ReadFile(vitePath)
	if os.IsNotExist(err) {
		vitePath = filepath.Join(web, "vite.config.js")
		vite, err = os.ReadFile(vitePath)
	}
	if err != nil {
		return result, err
	}
	updatedVite, err := viteConfig(string(vite))
	if err != nil {
		return result, fmt.Errorf("%s: %w", filepath.Base(vitePath), err)
	}
	relative, err := filepath.Rel(filepath.Dir(cssPath), web)
	if err != nil {
		return result, err
	}
	exclusion := filepath.ToSlash(filepath.Join(relative, "**", "*.go"))
	if !strings.HasPrefix(exclusion, ".") {
		exclusion = "./" + exclusion
	}
	updatedCSS := string(css)
	if !tailwindImport.MatchString(withoutCSSComments(updatedCSS)) {
		offset := 0
		if charset := cssCharset.FindStringIndex(updatedCSS); charset != nil {
			offset = charset[1]
		}
		updatedCSS = updatedCSS[:offset] + "\n@import \"tailwindcss\";\n" + updatedCSS[offset:]
	}
	found := false
	for _, match := range sourceNot.FindAllStringSubmatch(withoutCSSComments(updatedCSS), -1) {
		if filepath.Clean(match[1]) == filepath.Clean(exclusion) {
			found = true
		}
	}
	if !found {
		updatedCSS += "\n@source not \"" + exclusion + "\";\n"
	}
	stylesheetImport, err := filepath.Rel(filepath.Dir(layoutPath), cssPath)
	if err != nil {
		return result, err
	}
	stylesheetImport = filepath.ToSlash(stylesheetImport)
	if !strings.HasPrefix(stylesheetImport, ".") {
		stylesheetImport = "./" + stylesheetImport
	}
	updatedLayout, err := layoutImport(string(layoutSource), stylesheetImport)
	if err != nil {
		return result, fmt.Errorf("%s: %w", layout, err)
	}
	packagePath := filepath.Join(web, "package.json")
	packageSource, err := os.ReadFile(packagePath)
	if err != nil {
		return result, err
	}
	var pkg map[string]json.RawMessage
	if err = json.Unmarshal(packageSource, &pkg); err != nil {
		return result, err
	}
	var deps, devDeps map[string]string
	if err = json.Unmarshal(pkg["dependencies"], &deps); err != nil && len(pkg["dependencies"]) != 0 {
		return result, err
	}
	if err = json.Unmarshal(pkg["devDependencies"], &devDeps); err != nil && len(pkg["devDependencies"]) != 0 {
		return result, err
	}
	if devDeps == nil {
		devDeps = map[string]string{}
	}
	dependencyChange := false
	for _, name := range []string{"tailwindcss", "@tailwindcss/vite"} {
		prior := deps[name]
		if prior == "" {
			prior = devDeps[name]
		}
		if prior != "" {
			if !regexp.MustCompile(`^[~^]?4\.[0-9]+\.[0-9]+$`).MatchString(prior) {
				return result, fmt.Errorf("%s dependency %q conflicts with Tailwind 4; review the existing setup before applying", name, prior)
			}
			if deps[name] != "" && devDeps[name] != "" {
				return result, fmt.Errorf("%s is declared in both dependencies and devDependencies; choose one before applying", name)
			}
			if semver.Compare("v"+strings.TrimLeft(prior, "^~"), "v"+version) < 0 {
				if deps[name] != "" {
					deps[name] = version
				} else {
					devDeps[name] = version
				}
				dependencyChange = true
			}
			continue
		}
		devDeps[name] = version
		dependencyChange = true
	}
	updatedPackage := packageSource
	if dependencyChange {
		var document map[string]any
		decoder := json.NewDecoder(bytes.NewReader(packageSource))
		decoder.UseNumber()
		if err := decoder.Decode(&document); err != nil {
			return result, err
		}
		if len(deps) != 0 {
			document["dependencies"] = deps
		}
		document["devDependencies"] = devDeps
		var encoded bytes.Buffer
		encoder := json.NewEncoder(&encoded)
		encoder.SetEscapeHTML(false)
		for _, line := range strings.Split(string(packageSource), "\n") {
			trimmed := strings.TrimLeft(line, " \t")
			if len(trimmed) < len(line) && strings.HasPrefix(trimmed, "\"") {
				encoder.SetIndent("", line[:len(line)-len(trimmed)])
				break
			}
		}
		if err := encoder.Encode(document); err != nil {
			return result, err
		}
		updatedPackage = encoded.Bytes()
		if !bytes.HasSuffix(packageSource, []byte("\n")) {
			updatedPackage = bytes.TrimSuffix(updatedPackage, []byte("\n"))
		}
	}
	planned := map[string][]byte{vitePath: []byte(updatedVite), cssPath: []byte(updatedCSS), layoutPath: []byte(updatedLayout), packagePath: updatedPackage}
	paths := make([]string, 0, len(planned))
	for path := range planned {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	// Preflight all destinations before the first write or package installation.
	for _, path := range paths {
		for part := path; part != req.Project.Root; part = filepath.Dir(part) {
			info, e := os.Lstat(part)
			if e == nil && info.Mode()&os.ModeSymlink != 0 {
				return result, fmt.Errorf("%s contains a symlink", path)
			}
			if e != nil && !os.IsNotExist(e) {
				return result, e
			}
		}
	}
	for _, path := range paths {
		original, e := readOptional(path)
		if e != nil {
			return result, e
		}
		if bytes.Equal(original, planned[path]) {
			continue
		}
		if err = ctx.Err(); err != nil {
			return result, err
		}
		if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return result, err
		}
		// Report partial writes through the ordinary add-on result contract.
		relative, _ := filepath.Rel(req.Project.Root, path)
		result.Changed = append(result.Changed, filepath.ToSlash(relative))
		if err = os.WriteFile(path, planned[path], 0644); err != nil {
			return result, err
		}
	}
	if dependencyChange || !installed(web) {
		lock := filepath.Join(web, "pnpm-lock.yaml")
		before, _ := readOptional(lock)
		err = p.install(ctx, web)
		after, _ := readOptional(lock)
		if !bytes.Equal(before, after) {
			relative, _ := filepath.Rel(req.Project.Root, lock)
			result.Changed = append(result.Changed, filepath.ToSlash(relative))
		}
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

func installed(web string) bool {
	for _, name := range []string{"tailwindcss", "@tailwindcss/vite"} {
		data, err := os.ReadFile(filepath.Join(web, "node_modules", filepath.FromSlash(name), "package.json"))
		var pkg struct{ Version string }
		if err != nil || json.Unmarshal(data, &pkg) != nil || semver.Major("v"+pkg.Version) != "v4" || semver.Compare("v"+pkg.Version, "v"+version) < 0 {
			return false
		}
	}
	return true
}

func readOptional(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return data, err
}
func install(ctx context.Context, web string) error {
	vp := filepath.Join(web, "node_modules", ".bin", "vp")
	if _, err := os.Stat(vp); os.IsNotExist(err) {
		var e error
		vp, e = exec.LookPath("vp")
		if e != nil {
			return fmt.Errorf("Tailwind setup requires VitePlus: %w", e)
		}
	}
	command := exec.CommandContext(ctx, vp, "install")
	command.Dir = web
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("vp install: %w\n%s", err, output)
	}
	return nil
}

func withoutCSSComments(source string) string {
	var out strings.Builder
	var quote byte
	for i := 0; i < len(source); i++ {
		c := source[i]
		if quote != 0 {
			out.WriteByte(c)
			if c == '\\' && i+1 < len(source) {
				i++
				out.WriteByte(source[i])
			} else if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			out.WriteByte(c)
			continue
		}
		if c == '/' && i+1 < len(source) && source[i+1] == '*' {
			end := strings.Index(source[i+2:], "*/")
			if end < 0 {
				return out.String()
			}
			i += end + 3
			out.WriteByte(' ')
			continue
		}
		out.WriteByte(c)
	}
	return out.String()
}

var cssCharset = regexp.MustCompile(`^\s*@charset\s+[^;]+;`)
var legacyTailwind = regexp.MustCompile(`@tailwind\s`)
var tailwindImport = regexp.MustCompile(`@import\s+["']tailwindcss(?:/[^"']+)?["']`)
var sourceNot = regexp.MustCompile(`@source\s+not\s+["']([^"']+)["']\s*;`)
var scriptTag = regexp.MustCompile(`<script\b[^>]*>`)

func layoutImport(source, path string) (string, error) {
	for _, imported := range stylesheetImports(source) {
		if imported == path {
			return source, nil
		}
	}
	statement := "\n  import " + fmt.Sprintf("%q", path) + ";\n"
	for _, span := range scriptTag.FindAllStringIndex(source, -1) {
		tag := source[span[0]:span[1]]
		if strings.Contains(tag, "module") {
			continue
		}
		return source[:span[1]] + statement + source[span[1]:], nil
	}
	if source == "" {
		return "<script lang=\"ts\">" + statement + "  let { children } = $props();\n</script>\n\n{@render children()}\n", nil
	}
	return "<script>" + statement + "</script>\n" + source, nil
}

func stylesheetImports(source string) []string {
	var result []string
	for _, span := range scriptTag.FindAllStringIndex(source, -1) {
		end := strings.Index(source[span[1]:], "</script>")
		if end < 0 {
			continue
		}
		ts := tokens(source[span[1] : span[1]+end])
		for i, t := range ts {
			if t.text == "import" && i+1 < len(ts) {
				path := ts[i+1].text
				if len(path) > 2 && (path[0] == '\'' || path[0] == '"') && strings.HasSuffix(path[1:len(path)-1], ".css") {
					result = append(result, path[1:len(path)-1])
				}
			}
		}
	}
	return result
}

type token struct {
	text       string
	start, end int
}

func tokens(source string) []token {
	var s scanner.Scanner
	s.Init(strings.NewReader(source))
	s.Error = func(*scanner.Scanner, string) {}
	s.IsIdentRune = func(r rune, i int) bool {
		return r == '$' || r == '_' || unicode.IsLetter(r) || i > 0 && unicode.IsDigit(r)
	}
	var out []token
	for kind := s.Scan(); kind != scanner.EOF; kind = s.Scan() {
		out = append(out, token{s.TokenText(), s.Position.Offset, s.Pos().Offset})
	}
	return out
}
func viteConfig(source string) (string, error) {
	ts := tokens(source)
	alias := ""
	for i, t := range ts {
		if t.text == "import" && i+3 < len(ts) && ts[i+2].text == "from" && strings.Trim(ts[i+3].text, "\"'") == "@tailwindcss/vite" {
			alias = ts[i+1].text
		}
	}
	start := -1
	for i := 0; i+2 < len(ts); i++ {
		if ts[i].text != "export" || ts[i+1].text != "default" {
			continue
		}
		next := i + 2
		if ts[next].text == "defineConfig" && next+2 < len(ts) && ts[next+1].text == "(" {
			next += 2
		}
		if ts[next].text != "{" {
			return "", fmt.Errorf("expected an object or defineConfig object export; add @tailwindcss/vite to the dynamic configuration first")
		}
		start = next
		break
	}
	if start < 0 {
		return "", fmt.Errorf("no supported default configuration export")
	}
	depth, brackets, round := 0, 0, 0
	plugins := -1
	lastSpread := -1
	for i := start; i < len(ts); i++ {
		t := ts[i].text
		property := depth == 1 && brackets == 0 && round == 0 && i > 0 && (ts[i-1].text == "{" || ts[i-1].text == ",")
		if property && (t == "[" || strings.Trim(t, "\"'") == "plugins" && (i+1 >= len(ts) || ts[i+1].text != ":")) {
			return "", fmt.Errorf("computed or shorthand plugin configuration needs an explicit plugins array before applying")
		}
		if depth == 1 && brackets == 0 && round == 0 && i+2 < len(ts) && t == "." && ts[i+1].text == "." && ts[i+2].text == "." {
			lastSpread = i
		}
		if depth == 1 && brackets == 0 && round == 0 && strings.Trim(t, "\"'") == "plugins" && i+2 < len(ts) && ts[i+1].text == ":" {
			if plugins >= 0 {
				return "", fmt.Errorf("duplicate plugins configuration; choose one array before applying")
			}
			array := i + 2
			if ts[array].text == "lazyPlugins" {
				// VitePlus' ordinary scaffold wraps the plugin array in its
				// metadata-safe lazy factory. Keep that upstream wrapper intact.
				array++
				if array < len(ts) && ts[array].text == "(" {
					array++
				}
				if array < len(ts) && ts[array].text == "async" {
					array++
				}
				for _, expected := range []string{"(", ")", "=", ">"} {
					if array >= len(ts) || ts[array].text != expected {
						return "", fmt.Errorf("lazyPlugins must return a literal array; configure @tailwindcss/vite in its factory first")
					}
					array++
				}
			}
			if array >= len(ts) || ts[array].text != "[" {
				return "", fmt.Errorf("plugins must be an array or lazyPlugins array factory; configure @tailwindcss/vite in the dynamic plugin selection first")
			}
			plugins = array
		}
		switch t {
		case "{":
			depth++
		case "}":
			depth--
		case "[":
			brackets++
		case "]":
			brackets--
		case "(":
			round++
		case ")":
			round--
		}
		if depth == 0 {
			break
		}
	}
	if depth != 0 || brackets != 0 || round != 0 {
		return "", fmt.Errorf("unbalanced configuration object")
	}
	if lastSpread >= 0 && lastSpread >= plugins {
		return "", fmt.Errorf("a configuration spread may replace plugins; place an explicit plugins array after the spread before applying")
	}
	if alias != "" && plugins >= 0 {
		depth := 0
		for i := plugins; i < len(ts); i++ {
			if ts[i].text == "[" {
				depth++
			}
			if ts[i].text == "]" {
				depth--
				if depth == 0 {
					break
				}
			}
			if ts[i].text == alias && i+1 < len(ts) && ts[i+1].text == "(" {
				return source, nil
			}
		}
	}
	prefix := ""
	if alias == "" {
		alias = "skgoTailwindcss"
		for _, t := range ts {
			if t.text == alias {
				return "", fmt.Errorf("identifier %s already exists; configure @tailwindcss/vite explicitly", alias)
			}
		}
		prefix = "import " + alias + " from \"@tailwindcss/vite\";\n"
	}
	offset := ts[start].end
	addition := "\n  plugins: [" + alias + "()],"
	if plugins >= 0 {
		offset = ts[plugins].end
		addition = alias + "(), "
	}
	return prefix + source[:offset] + addition + source[offset:], nil
}
