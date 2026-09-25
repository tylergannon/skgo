package check

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

func checkESLint(ctx context.Context, root, web, bin string) (Check, []Diagnostic) {
	output, err := command(ctx, web, nil, bin, "--format", "json", ".")
	c := Check{Name: "eslint", Status: "complete"}
	type point struct{ Line, Column int }
	type message struct {
		RuleID      string `json:"ruleId"`
		Severity    int    `json:"severity"`
		Message     string `json:"message"`
		Line        int    `json:"line"`
		Column      int    `json:"column"`
		EndLine     int    `json:"endLine"`
		EndColumn   int    `json:"endColumn"`
		Suggestions []struct {
			Desc string `json:"desc"`
		} `json:"suggestions"`
		Fix *struct{} `json:"fix"`
	}
	var entries []struct {
		FilePath        string    `json:"filePath"`
		Messages        []message `json:"messages"`
		FatalErrorCount int       `json:"fatalErrorCount"`
	}
	if e := json.Unmarshal(output, &entries); e != nil {
		c.Status = "failed"
		c.Message = "ESLint did not return complete JSON: " + strings.TrimSpace(string(output))
		if c.Message == "ESLint did not return complete JSON: " {
			c.Message = e.Error()
		}
		return c, nil
	}
	var ds []Diagnostic
	svelteCovered := false
	for _, entry := range entries {
		if strings.HasSuffix(entry.FilePath, ".svelte") {
			svelteCovered = true
		}
		for _, m := range entry.Messages {
			sev := "warning"
			if m.Severity == 2 {
				sev = "error"
			}
			code := m.RuleID
			if code == "" {
				code = "parse"
			}
			d := Diagnostic{Source: "eslint", Code: code, Severity: sev, Message: m.Message}
			if m.Line > 0 {
				d.Location = &Location{File: relative(root, entry.FilePath), Line: m.Line, Column: m.Column, EndLine: m.EndLine, EndColumn: m.EndColumn}
			}
			if m.Fix != nil {
				d.Fix = "eslint --fix"
			}
			if len(m.Suggestions) > 0 {
				d.Fix = m.Suggestions[0].Desc
			}
			if strings.HasPrefix(m.RuleID, "svelte/") {
				d.Documentation = "https://sveltejs.github.io/eslint-plugin-svelte/rules/" + strings.TrimPrefix(m.RuleID, "svelte/") + "/"
			} else if m.RuleID != "" {
				d.Documentation = "https://eslint.org/docs/latest/rules/" + m.RuleID
			}
			ds = append(ds, d)
		}
	}
	if hasSvelteFiles(web) && !svelteCovered {
		c.Status = "incomplete"
		c.Message = "ESLint returned no Svelte file results; check the project ESLint configuration"
	}
	var exit *exec.ExitError
	if err != nil && (!errors.As(err, &exit) || exit.ExitCode() > 1 || len(ds) == 0) {
		c.Status = "failed"
		c.Message = fmt.Sprintf("ESLint exited without a complete diagnostic result: %v", err)
	}
	return c, ds
}

func hasSvelteFiles(web string) bool {
	found := false
	_ = filepath.WalkDir(filepath.Join(web, "src"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "node_modules" || d.Name() == ".svelte-kit" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(d.Name(), ".svelte") {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found
}

var machinePrefix = regexp.MustCompile(`^\d+ (.*)$`)
var completion = regexp.MustCompile(`^COMPLETED (\d+) FILES (\d+) ERRORS (\d+) WARNINGS (\d+) FILES_WITH_PROBLEMS$`)

func checkSvelte(ctx context.Context, root, web, bin string) (Check, []Diagnostic) {
	output, err := command(ctx, web, nil, bin, "--output", "machine-verbose", "--tsconfig", "./tsconfig.json")
	exitCode := 0
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			exitCode = exit.ExitCode()
		} else {
			exitCode = -1
		}
	}
	return parseSvelte(root, web, output, exitCode)
}

func parseSvelte(root, web string, output []byte, exitCode int) (Check, []Diagnostic) {
	c := Check{Name: "svelte-check", Status: "complete"}
	var ds []Diagnostic
	started := false
	completed := false
	unknown := []string{}
	filesChecked := 0
	expectedErrors := 0
	expectedWarnings := 0
	for _, raw := range bytes.Split(bytes.TrimSpace(output), []byte("\n")) {
		line := strings.TrimSpace(string(raw))
		if m := machinePrefix.FindStringSubmatch(line); m != nil {
			line = m[1]
		}
		if strings.HasPrefix(line, "START ") {
			started = true
			continue
		}
		if m := completion.FindStringSubmatch(line); m != nil {
			completed = true
			filesChecked, _ = strconv.Atoi(m[1])
			expectedErrors, _ = strconv.Atoi(m[2])
			expectedWarnings, _ = strconv.Atoi(m[3])
			continue
		}
		if !strings.HasPrefix(line, "{") {
			if line != "" {
				unknown = append(unknown, line)
			}
			continue
		}
		var v struct {
			Type            string                        `json:"type"`
			Filename        string                        `json:"filename"`
			Start           struct{ Line, Character int } `json:"start"`
			End             struct{ Line, Character int } `json:"end"`
			Message         string                        `json:"message"`
			Code            json.RawMessage               `json:"code"`
			CodeDescription struct {
				Href string `json:"href"`
			} `json:"codeDescription"`
			Source string `json:"source"`
		}
		if e := json.Unmarshal([]byte(line), &v); e != nil {
			c.Status = "incomplete"
			c.Message = "invalid machine diagnostic: " + e.Error()
			return c, ds
		}
		if v.Type != "ERROR" && v.Type != "WARNING" {
			continue
		}
		sev := "warning"
		if v.Type == "ERROR" {
			sev = "error"
		}
		code := strings.Trim(string(v.Code), `"`)
		path := v.Filename
		if !filepath.IsAbs(path) {
			path = filepath.Join(web, path)
		}
		loc := &Location{File: relative(root, path), Line: v.Start.Line + 1, Column: v.Start.Character + 1, EndLine: v.End.Line + 1, EndColumn: v.End.Character + 1}
		ds = append(ds, Diagnostic{Source: "svelte-check/" + v.Source, Code: code, Severity: sev, Message: v.Message, Location: loc, Documentation: v.CodeDescription.Href})
	}
	errorsCount, warningsCount := 0, 0
	for _, d := range ds {
		if d.Severity == "error" {
			errorsCount++
		} else {
			warningsCount++
		}
	}
	if !started || !completed || filesChecked == 0 || errorsCount != expectedErrors || warningsCount != expectedWarnings || len(unknown) > 0 {
		c.Status = "incomplete"
		c.Message = fmt.Sprintf("machine output lacks a matched START/COMPLETED summary or contains unrecognized output (read %d errors, %d warnings): %s", errorsCount, warningsCount, strings.TrimSpace(string(output)))
	} else if (exitCode == 0 && errorsCount > 0) || (exitCode != 0 && (exitCode != 1 || errorsCount == 0)) {
		c.Status = "failed"
		c.Message = fmt.Sprintf("checker exit code %d conflicts with completed diagnostics", exitCode)
	}
	return c, ds
}
