package check

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var vitePlusANSI = regexp.MustCompile(`\x1b\[[0-9;]*m`)
var vitePlusFormatted = regexp.MustCompile(`All [1-9][0-9]* files? are correctly formatted`)
var vitePlusLinted = regexp.MustCompile(`Found (?:0 errors? and [0-9]+ warnings?|no warnings or lint errors) in [1-9][0-9]* files?`)

// checkVitePlus lets Vite+ apply the project's configured formatter, linter,
// type checker and ignore rules. A failed check is diagnosed with its own
// component commands, without mutating any files.
func checkVitePlus(ctx context.Context, root, web, bin string) (Check, []Diagnostic) {
	output, err := command(ctx, web, nil, bin, "check")
	plain := strings.TrimSpace(vitePlusANSI.ReplaceAllString(string(output), ""))
	c := Check{Name: "vite-plus", Status: "complete"}
	if err == nil {
		if !vitePlusFormatted.MatchString(plain) || !vitePlusLinted.MatchString(plain) {
			c.Status = "incomplete"
			c.Message = "Vite+ exited successfully without confirming formatting and lint coverage: " + plain
		}
		return c, nil
	}
	c.Status = "failed"
	c.Message = plain
	if c.Message == "" {
		c.Message = err.Error()
	}
	var ds []Diagnostic
	if formatted, fmtErr := command(ctx, web, nil, bin, "fmt", "--list-different"); fmtErr != nil {
		for _, name := range strings.Split(strings.TrimSpace(string(formatted)), "\n") {
			name = strings.TrimSpace(name)
			path := filepath.Join(web, name)
			if name == "" || filepath.IsAbs(name) || strings.HasPrefix(name, "..") {
				continue
			}
			if fi, err := os.Stat(path); err != nil || fi.IsDir() {
				continue
			}
			rel := relative(root, path)
			ds = append(ds, Diagnostic{Source: "vite-plus/fmt", Code: "format", Severity: "error", Message: "Frontend formatting differs from Vite+", Location: &Location{File: rel}, Fix: "vp fmt --write " + name})
		}
	}
	if linted, _ := command(ctx, web, nil, bin, "lint", "--format", "json"); len(linted) > 0 {
		ds = append(ds, parseVitePlusLint(root, web, linted)...)
	}
	return c, ds
}

func parseVitePlusLint(root, web string, output []byte) []Diagnostic {
	var report struct {
		Diagnostics []struct {
			Message  string `json:"message"`
			Code     string `json:"code"`
			Severity string `json:"severity"`
			URL      string `json:"url"`
			Filename string `json:"filename"`
			Labels   []struct {
				Span struct {
					Line   int `json:"line"`
					Column int `json:"column"`
				} `json:"span"`
			} `json:"labels"`
		} `json:"diagnostics"`
	}
	if json.Unmarshal(output, &report) != nil {
		return nil
	}
	var ds []Diagnostic
	for _, item := range report.Diagnostics {
		severity := item.Severity
		if severity != "error" && severity != "warning" {
			severity = "error"
		}
		d := Diagnostic{Source: "vite-plus/lint", Code: item.Code, Severity: severity, Message: item.Message, Documentation: item.URL}
		if item.Filename != "" && len(item.Labels) > 0 {
			path := item.Filename
			if !filepath.IsAbs(path) {
				path = filepath.Join(web, path)
			}
			d.Location = &Location{File: relative(root, path), Line: item.Labels[0].Span.Line, Column: item.Labels[0].Span.Column}
		}
		ds = append(ds, d)
	}
	return ds
}

func checkSvelteFormatting(ctx context.Context, root, web, bin string) (Check, []Diagnostic) {
	c := Check{Name: "svelte-format", Status: "complete"}
	output, err := command(ctx, web, nil, bin, "--plugin", "prettier-plugin-svelte", "--list-different", "src/**/*.svelte")
	if err == nil {
		if strings.TrimSpace(string(output)) != "" {
			c.Status = "incomplete"
			c.Message = "Prettier exited successfully with unrecognized output: " + strings.TrimSpace(string(output))
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
	var ds []Diagnostic
	for _, name := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		name = strings.TrimSpace(name)
		path := filepath.Join(web, name)
		if name == "" || filepath.IsAbs(name) || strings.HasPrefix(name, "..") {
			break
		}
		if fi, statErr := os.Stat(path); statErr != nil || fi.IsDir() || !strings.HasSuffix(name, ".svelte") {
			break
		}
		rel := relative(root, path)
		ds = append(ds, Diagnostic{Source: "svelte-format", Code: "format", Severity: "error", Message: "Svelte formatting differs from Prettier", Location: &Location{File: rel}, Fix: "prettier --plugin prettier-plugin-svelte --write " + name})
	}
	if len(ds) == 0 || len(ds) != len(strings.Split(strings.TrimSpace(string(output)), "\n")) {
		c.Status = "failed"
		c.Message = strings.TrimSpace(string(output))
		return c, nil
	}
	c.Status = "failed"
	c.Message = "unformatted Svelte files"
	return c, ds
}
