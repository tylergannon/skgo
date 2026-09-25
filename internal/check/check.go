// Package check composes the project's installed checkers without editing source.
package check

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/tylergannon/skgo/internal/advice"
	"github.com/tylergannon/skgo/internal/gen"
)

type Location struct {
	File      string `json:"file"`
	Line      int    `json:"line,omitempty"`
	Column    int    `json:"column,omitempty"`
	EndLine   int    `json:"endLine,omitempty"`
	EndColumn int    `json:"endColumn,omitempty"`
}

type Diagnostic struct {
	Source        string     `json:"source"`
	Code          string     `json:"code"`
	Severity      string     `json:"severity"`
	Message       string     `json:"message"`
	Location      *Location  `json:"location,omitempty"`
	Related       []Location `json:"related,omitempty"`
	Fix           string     `json:"fix,omitempty"`
	Documentation string     `json:"documentation,omitempty"`
}

type Check struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // complete, failed, incomplete
	Message string `json:"message,omitempty"`
}

type Report struct {
	OK          bool         `json:"ok"`
	Checks      []Check      `json:"checks"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

type Options struct{ Root, Web, Out string }

// Run always attempts each independent checker. A diagnostic exit is distinct
// from a checker that failed or stopped before publishing a complete result.
func Run(ctx context.Context, o Options) Report {
	r := Report{OK: true, Checks: []Check{}, Diagnostics: []Diagnostic{}}
	root, err := filepath.Abs(o.Root)
	if err != nil {
		return Report{Checks: []Check{{Name: "project", Status: "failed", Message: err.Error()}}}
	}
	web := o.Web
	if web == "" {
		web = "web"
	}
	if !filepath.IsAbs(web) {
		web = filepath.Join(root, web)
	}
	out := o.Out
	if out == "" {
		if fi, err := os.Stat(filepath.Join(root, "internal", "skgo")); err == nil && fi.IsDir() {
			out = filepath.Join("internal", "skgo")
		} else {
			out = "generated"
		}
	}
	if !filepath.IsAbs(out) {
		out = filepath.Join(root, out)
	}
	add := func(c Check, ds ...Diagnostic) {
		r.Checks = append(r.Checks, c)
		r.Diagnostics = append(r.Diagnostics, ds...)
		if c.Status != "complete" {
			r.OK = false
		}
		for _, d := range ds {
			if d.Severity == "error" {
				r.OK = false
			}
		}
	}

	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		add(Check{Name: "project", Status: "failed", Message: "go.mod is missing: " + err.Error()})
		return r
	}
	if _, err := os.Stat(filepath.Join(web, "package.json")); err != nil {
		add(Check{Name: "project", Status: "failed", Message: "web/package.json is missing: " + err.Error()})
		return r
	}
	routes, err := loadRoutes(out)
	if err != nil {
		add(Check{Name: "go-overlay", Status: "failed", Message: "cannot read generated route inventory: " + err.Error()})
	}
	overlayPath, cleanup, overlayErr := makeOverlay(root, routes)
	if overlayErr != nil {
		add(Check{Name: "go-overlay", Status: "failed", Message: "cannot prepare authored route overlay: " + overlayErr.Error()})
	} else {
		defer cleanup()
	}

	c, ds := checkGoImports(ctx, root, out)
	add(c, ds...)
	// The formatter's own file selection and ignore rules are authoritative.
	if bin := localBin(web, "prettier"); bin != "" {
		c, ds := checkPrettier(ctx, root, web, bin)
		add(c, ds...)
	} else {
		add(Check{Name: "prettier", Status: "failed", Message: "project Prettier is missing; install/configure prettier and prettier-plugin-svelte"})
	}
	if bin := localBin(web, "eslint"); bin != "" {
		c, ds := checkESLint(ctx, root, web, bin)
		add(c, ds...)
	} else {
		add(Check{Name: "eslint", Status: "failed", Message: "project ESLint is missing; install/configure eslint and eslint-plugin-svelte"})
	}
	if bin := localBin(web, "svelte-check"); bin != "" {
		// Kit 3's generated $app tsconfig is supplied by its sync command. This
		// command is deliberately read-only and never invokes sync itself.
		if needsKitConfig(web) {
			if _, err := os.Stat(filepath.Join(web, "node_modules", "$app", "tsconfig.json")); err != nil {
				add(Check{Name: "svelte-check", Status: "failed", Message: "Kit type configuration is missing; run svelte-kit sync, then check again"})
			} else {
				c, ds := checkSvelte(ctx, root, web, bin)
				add(c, ds...)
			}
		} else {
			c, ds := checkSvelte(ctx, root, web, bin)
			add(c, ds...)
		}
	} else {
		add(Check{Name: "svelte-check", Status: "failed", Message: "project svelte-check is missing"})
	}
	goEnv := readonlyGoEnv()
	if overlayPath != "" {
		goEnv = appendGoFlag(goEnv, "-overlay="+overlayPath)
	}
	goCompileFailed := false
	for _, spec := range []struct {
		name string
		args []string
	}{
		{"go-build", []string{"build", "./..."}},
		{"go-vet", []string{"vet", "./..."}},
	} {
		output, err := command(ctx, root, goEnv, "go", spec.args...)
		c, ds := checkGoOutput(spec.name, root, routes, output, err)
		if spec.name == "go-build" && (len(ds) > 0 || c.Status != "complete") {
			goCompileFailed = true
		}
		if spec.name == "go-vet" && goCompileFailed {
			c.Status = "incomplete"
			c.Message = "Go compilation failed before all vet analyzers could complete"
		}
		add(c, ds...)
	}
	if bin, err := exec.LookPath("staticcheck"); err == nil {
		output, runErr := command(ctx, root, goEnv, bin, "-f", "json", "./...")
		c, ds := checkStaticcheck(root, routes, output, runErr)
		add(c, ds...)
	} else {
		add(Check{Name: "staticcheck", Status: "failed", Message: "Staticcheck is missing from PATH"})
	}
	if exe, err := os.Executable(); err == nil {
		output, runErr := command(ctx, root, goEnv, "go", "vet", "-json", "-vettool="+exe, "./...")
		c, ds := parseAdvice(root, routes, output, runErr)
		if goCompileFailed && c.Status == "complete" {
			c.Status = "incomplete"
			c.Message = "Go compilation failed before all skgo advice analyzers could complete"
		}
		add(c, ds...)
	} else {
		add(Check{Name: "skgo-advice", Status: "failed", Message: "cannot locate this skgo executable: " + err.Error()})
	}
	if err := gen.Check(gen.Config{Web: web, Out: out}); err != nil {
		message := authoredMessage(root, routes, err.Error())
		add(Check{Name: "skgo", Status: "failed", Message: message}, skgoDiagnostic(root, message))
	} else {
		add(Check{Name: "skgo", Status: "complete"})
	}
	return r
}

func command(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
	c := exec.CommandContext(ctx, name, args...)
	c.Dir = dir
	if env != nil {
		c.Env = env
	}
	return c.CombinedOutput()
}

func readonlyGoEnv() []string {
	env := os.Environ()
	flags := os.Getenv("GOFLAGS")
	if !strings.Contains(flags, "-mod=") {
		flags += " -mod=readonly"
	}
	return append(env, "GOFLAGS="+strings.TrimSpace(flags))
}

func appendGoFlag(env []string, flag string) []string {
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], "GOFLAGS=") {
			env[i] += " " + flag
			return env
		}
	}
	return append(env, "GOFLAGS="+flag)
}

func localBin(web, name string) string {
	p := filepath.Join(web, "node_modules", ".bin", name)
	if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
		return p
	}
	return ""
}

func needsKitConfig(web string) bool {
	b, err := os.ReadFile(filepath.Join(web, "tsconfig.json"))
	return err == nil && bytes.Contains(b, []byte("$app/tsconfig"))
}

func checkGoImports(ctx context.Context, root, out string) (Check, []Diagnostic) {
	c := Check{Name: "goimports", Status: "complete"}
	var ds []Diagnostic
	bin, err := exec.LookPath("goimports")
	if err != nil {
		c.Status = "failed"
		c.Message = "goimports is missing"
		return c, nil
	}
	var files []string
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p == filepath.Join(out, "links") {
				return filepath.SkipDir
			}
			if p != root && (d.Name() == ".git" || d.Name() == "node_modules" || d.Name() == "vendor" || d.Name() == "testdata" || d.Name() == "ephemeral" || d.Name() == "generated" || d.Name() == "build") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_gen.go") {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		c.Status = "failed"
		c.Message = err.Error()
		return c, ds
	}
	if len(files) == 0 {
		return c, nil
	}
	args := append([]string{"-l"}, files...)
	output, err := command(ctx, root, nil, bin, args...)
	if err != nil {
		c.Status = "failed"
		c.Message = strings.TrimSpace(string(output))
		if c.Message == "" {
			c.Message = err.Error()
		}
		return c, nil
	}
	var different []string
	for _, p := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if p == "" {
			continue
		}
		before, e1 := os.ReadFile(p)
		after, e2 := command(ctx, root, nil, bin, p)
		if e1 != nil || e2 != nil {
			c.Status = "failed"
			c.Message = "goimports could not inspect " + p
			return c, ds
		}
		rel := relative(root, p)
		different = append(different, rel)
		ds = append(ds, Diagnostic{Source: "goimports", Code: "format", Severity: "error", Message: "Go formatting or imports differ from goimports", Location: &Location{File: rel, Line: firstDifferentLine(before, after)}, Fix: "goimports -w " + rel})
	}
	if len(different) > 0 {
		c.Message = "unformatted: " + strings.Join(different, ", ")
	}
	return c, ds
}

func firstDifferentLine(a, b []byte) int {
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	line := 1
	for i := 0; i < limit; i++ {
		if a[i] != b[i] {
			return line
		}
		if a[i] == '\n' {
			line++
		}
	}
	return line
}

func checkPrettier(ctx context.Context, root, web, bin string) (Check, []Diagnostic) {
	output, err := command(ctx, web, nil, bin, "--list-different", ".")
	c := Check{Name: "prettier", Status: "complete"}
	if err == nil {
		if strings.TrimSpace(string(output)) != "" {
			c.Status = "incomplete"
			c.Message = "Prettier listed files or other output but exited successfully: " + strings.TrimSpace(string(output))
		}
		return c, nil
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		c.Status = "failed"
		c.Message = strings.TrimSpace(string(output))
		if c.Message == "" {
			c.Message = err.Error()
		}
		return c, nil
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		c.Status = "failed"
		c.Message = "Prettier exited with status 1 without naming any files"
		return c, nil
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") || strings.Contains(line, "Error") || strings.Contains(line, "error") {
			c.Status = "failed"
			c.Message = strings.TrimSpace(string(output))
			return c, nil
		}
	}
	c.Message = "unformatted: " + strings.Join(lines, ", ")
	var ds []Diagnostic
	for _, line := range lines {
		if line == "" {
			continue
		}
		ds = append(ds, Diagnostic{Source: "prettier", Code: "format", Severity: "error", Message: "Frontend formatting differs from Prettier", Location: &Location{File: relative(root, filepath.Join(web, line))}, Fix: "prettier --write " + line})
	}
	return c, ds
}

var positionRE = regexp.MustCompile(`^(.+?):(\d+)(?::(\d+))?: (.+)$`)
var goPositionRE = regexp.MustCompile(`([^\s,()]+\.go):(\d+):(\d+)`)
var goFileRE = regexp.MustCompile(`([^\s,()]+\.go):`)

func skgoCode(message string) string {
	switch {
	case strings.Contains(message, "generated route link"):
		return "stale-route-link"
	case strings.Contains(message, "adapter"):
		return "adapter-compatibility"
	case strings.Contains(message, "declared twice"), strings.Contains(message, "two server loads"), strings.Contains(message, "answers") && strings.Contains(message, "twice"):
		return "duplicate-declaration"
	case strings.Contains(message, "belongs in"):
		return "declaration-placement"
	case strings.Contains(message, "File travels"), strings.Contains(message, "skgo.File"),
		strings.Contains(message, "Deferred"), strings.Contains(message, "cannot cross"),
		strings.Contains(message, "polytype"), strings.Contains(message, "wire"),
		strings.Contains(message, "cannot travel over JSON"), strings.Contains(message, "duplicate serialized name"):
		return "SKGO007"
	case strings.Contains(message, "loading remote packages"), strings.Contains(message, "cannot use"):
		return "go-type"
	default:
		return "contract"
	}
}

func diagnosticFromError(source, code, root, msg string) Diagnostic {
	d := Diagnostic{Source: source, Code: code, Severity: "error", Message: msg}
	for _, m := range goPositionRE.FindAllStringSubmatch(msg, -1) {
		line, _ := strconv.Atoi(m[2])
		col, _ := strconv.Atoi(m[3])
		loc := Location{File: relative(root, m[1]), Line: line, Column: col}
		if d.Location == nil {
			d.Location = &loc
		} else {
			d.Related = append(d.Related, loc)
		}
	}
	if d.Location == nil {
		if m := goFileRE.FindStringSubmatch(msg); m != nil {
			d.Location = &Location{File: relative(root, m[1])}
		}
	}
	return d
}

func skgoDiagnostic(root, message string) Diagnostic {
	d := diagnosticFromError("skgo", skgoCode(message), root, message)
	if _, ok := advice.Lookup(d.Code); ok {
		d.Documentation = "skgo advice " + d.Code
	}
	if d.Code == "SKGO007" && len(d.Related) > 0 {
		// The nested field position follows the function marker in the
		// generator error. Put the offending field first for an editor.
		field := d.Related[0]
		d.Related[0] = *d.Location
		d.Location = &field
	}
	return d
}

func relative(root, path string) string {
	path = strings.TrimPrefix(path, "vet: ")
	if filepath.IsAbs(path) {
		// Go may report the physical path while --root names a symlinked
		// workspace (notably /tmp on macOS). Keep authored locations inside
		// the project in that case.
		if resolvedRoot, err := filepath.EvalSymlinks(root); err == nil {
			if resolvedPath, err := filepath.EvalSymlinks(path); err == nil {
				if rel, err := filepath.Rel(resolvedRoot, resolvedPath); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
					return filepath.ToSlash(rel)
				}
			}
		}
		if rel, err := filepath.Rel(root, path); err == nil {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(filepath.Clean(path))
}

func checkGoOutput(name, root string, routes routeInventory, output []byte, err error) (Check, []Diagnostic) {
	c := Check{Name: name, Status: "complete"}
	if name == "go-vet" && strings.Contains(string(output), "vet: ") {
		c.Status = "incomplete"
		c.Message = "Go compilation failed before vet analyzers completed: " + strings.TrimSpace(string(output))
		return c, nil
	}
	var ds []Diagnostic
	var unknown []string
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if err != nil && strings.HasPrefix(line, "# ") {
			// The Go command prefixes compile diagnostics with a package name.
			continue
		}
		m := positionRE.FindStringSubmatch(line)
		if m == nil {
			unknown = append(unknown, line)
			continue
		}
		ln, _ := strconv.Atoi(m[2])
		col, _ := strconv.Atoi(m[3])
		ds = append(ds, Diagnostic{Source: name, Code: "go", Severity: "error", Message: m[4], Location: &Location{File: authoredPath(root, routes, m[1]), Line: ln, Column: col}})
	}
	if len(unknown) > 0 {
		c.Status = "incomplete"
		c.Message = "Go tool emitted unrecognized output: " + strings.Join(unknown, "\n")
	}
	if err != nil && len(ds) == 0 && c.Status == "complete" {
		c.Status = "failed"
		c.Message = strings.TrimSpace(string(output))
		if c.Message == "" {
			c.Message = err.Error()
		}
	}
	return c, ds
}

func checkStaticcheck(root string, routes routeInventory, output []byte, err error) (Check, []Diagnostic) {
	c := Check{Name: "staticcheck", Status: "complete"}
	var ds []Diagnostic
	type pos struct {
		File   string `json:"file"`
		Line   int    `json:"line"`
		Column int    `json:"column"`
	}
	type item struct {
		Code     string `json:"code"`
		Severity string `json:"severity"`
		Message  string `json:"message"`
		Location struct {
			File   string `json:"file"`
			Line   int    `json:"line"`
			Column int    `json:"column"`
		} `json:"location"`
		End pos `json:"end"`
	}
	for _, line := range bytes.Split(bytes.TrimSpace(output), []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var v item
		if e := json.Unmarshal(line, &v); e != nil || v.Code == "" {
			c.Status = "failed"
			c.Message = "Staticcheck emitted non-diagnostic output: " + string(output)
			return c, ds
		}
		if v.Code == "compile" {
			c.Status = "incomplete"
			c.Message = "Go compilation failed before Staticcheck analyzers completed: " + v.Message
			continue
		}
		severity := "error"
		if v.Severity == "warning" {
			severity = "warning"
		}
		ds = append(ds, Diagnostic{Source: "staticcheck", Code: v.Code, Severity: severity, Message: v.Message, Location: &Location{File: authoredPath(root, routes, v.Location.File), Line: v.Location.Line, Column: v.Location.Column, EndLine: v.End.Line, EndColumn: v.End.Column}})
	}
	if err != nil && len(ds) == 0 && c.Status == "complete" {
		c.Status = "failed"
		c.Message = strings.TrimSpace(string(output))
		if c.Message == "" {
			c.Message = err.Error()
		}
	}
	return c, ds
}

func RenderHuman(r Report) string {
	var b strings.Builder
	for _, d := range r.Diagnostics {
		loc := ""
		if d.Location != nil {
			loc = d.Location.File
			if d.Location.Line > 0 {
				loc += fmt.Sprintf(":%d", d.Location.Line)
				if d.Location.Column > 0 {
					loc += fmt.Sprintf(":%d", d.Location.Column)
				}
			}
			loc += ": "
		}
		fmt.Fprintf(&b, "%s%s %s/%s: %s\n", loc, d.Severity, d.Source, d.Code, d.Message)
		if d.Documentation != "" {
			fmt.Fprintf(&b, "  %s\n", d.Documentation)
		}
	}
	for _, c := range r.Checks {
		fmt.Fprintf(&b, "%s: %s", c.Name, c.Status)
		if c.Message != "" {
			fmt.Fprintf(&b, " (%s)", c.Message)
		}
		b.WriteByte('\n')
	}
	return b.String()
}
