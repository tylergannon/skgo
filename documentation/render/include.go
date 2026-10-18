package render

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	fenceOpen   = regexp.MustCompile("^(\\s*)```([A-Za-z0-9_+-]*)\\s*$")
	fileLabel   = regexp.MustCompile(`^/// file:\s*(.+?)\s*$`)
	includeLine = regexp.MustCompile(`^/// include:\s*(\S+?)(?:#([A-Za-z0-9_-]+))?\s*$`)
	regionStart = regexp.MustCompile(`^\s*(?://|<!--|#)\s*docs:\s*([A-Za-z0-9_-]+)\s*(?:-->)?\s*$`)
	regionEnd   = regexp.MustCompile(`^\s*(?://|<!--|#)\s*docs:\s*end\s+([A-Za-z0-9_-]+)\s*(?:-->)?\s*$`)
)

// expandIncludes resolves every fence that carries an include line and
// enforces that labelled Go fences carry one. Line numbers in problems are
// relative to the page file.
func expandIncludes(root string, page *Page) (string, []Problem) {
	var problems []Problem
	lines := strings.Split(page.Body, "\n")
	var out []string
	for i := 0; i < len(lines); i++ {
		open := fenceOpen.FindStringSubmatch(lines[i])
		if open == nil {
			out = append(out, lines[i])
			continue
		}
		indent, lang := open[1], open[2]
		start := i
		var body []string
		closed := false
		for i++; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "```" {
				closed = true
				break
			}
			body = append(body, lines[i])
		}
		if !closed {
			problems = append(problems, Problem{File: page.File, Line: page.bodyLine + start, Message: "unterminated code fence"})
			out = append(out, lines[start:]...)
			break
		}
		label, includePath, region := "", "", ""
		var rest []string
		for _, line := range body {
			trimmed := strings.TrimPrefix(line, indent)
			if m := fileLabel.FindStringSubmatch(trimmed); m != nil && label == "" {
				label = m[1]
				continue
			}
			if m := includeLine.FindStringSubmatch(trimmed); m != nil {
				if includePath != "" {
					problems = append(problems, Problem{File: page.File, Line: page.bodyLine + start, Message: "a fence has one include"})
				}
				includePath, region = m[1], m[2]
				continue
			}
			rest = append(rest, trimmed)
		}
		if includePath == "" {
			if lang == "go" && label != "" {
				problems = append(problems, Problem{File: page.File, Line: page.bodyLine + start, Message: "a go fence with a /// file: label must include real code: add /// include: example/...#region"})
			}
			out = append(out, lines[start:i+1]...)
			continue
		}
		if strings.TrimSpace(strings.Join(rest, "")) != "" {
			problems = append(problems, Problem{File: page.File, Line: page.bodyLine + start, Message: "a fence with an include holds nothing else but its /// file: label"})
		}
		code, err := includeRegion(root, includePath, region)
		if err != nil {
			problems = append(problems, Problem{File: page.File, Line: page.bodyLine + start, Message: err.Error()})
			out = append(out, lines[start:i+1]...)
			continue
		}
		out = append(out, indent+"```"+lang)
		if label != "" {
			out = append(out, indent+"/// file: "+label)
		}
		for _, line := range strings.Split(code, "\n") {
			if line == "" {
				out = append(out, "")
			} else {
				out = append(out, indent+line)
			}
		}
		out = append(out, indent+"```")
	}
	return strings.Join(out, "\n"), problems
}

// includeRegion reads a file relative to the module root and returns the
// named region, or the whole file without marker lines when region is empty,
// with common indentation removed.
func includeRegion(root, rel, region string) (string, error) {
	if strings.HasPrefix(rel, "/") || strings.Contains(rel, "..") {
		return "", fmt.Errorf("include %s: paths are relative to the module root", rel)
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return "", fmt.Errorf("include %s: %w", rel, err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	var picked []string
	if region == "" {
		for _, line := range lines {
			if regionStart.MatchString(line) || regionEnd.MatchString(line) {
				continue
			}
			picked = append(picked, line)
		}
	} else {
		inside, found := false, false
		for _, line := range lines {
			if m := regionStart.FindStringSubmatch(line); m != nil && m[1] == region {
				if found {
					return "", fmt.Errorf("include %s#%s: the region is marked twice", rel, region)
				}
				inside, found = true, true
				continue
			}
			if m := regionEnd.FindStringSubmatch(line); m != nil && m[1] == region {
				inside = false
				continue
			}
			if inside && !regionStart.MatchString(line) && !regionEnd.MatchString(line) {
				picked = append(picked, line)
			}
		}
		if !found {
			return "", fmt.Errorf("include %s#%s: no line `// docs: %s` in the file", rel, region, region)
		}
		if inside {
			return "", fmt.Errorf("include %s#%s: no line `// docs: end %s` in the file", rel, region, region)
		}
	}
	// trim blank edges
	for len(picked) > 0 && strings.TrimSpace(picked[0]) == "" {
		picked = picked[1:]
	}
	for len(picked) > 0 && strings.TrimSpace(picked[len(picked)-1]) == "" {
		picked = picked[:len(picked)-1]
	}
	if len(picked) == 0 {
		return "", fmt.Errorf("include %s#%s: the region is empty", rel, region)
	}
	return dedent(picked), nil
}

func dedent(lines []string) string {
	common := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		n := len(line) - len(strings.TrimLeft(line, " \t"))
		if common < 0 || n < common {
			common = n
		}
	}
	if common <= 0 {
		return strings.Join(lines, "\n")
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		if len(line) >= common {
			out[i] = line[common:]
		} else {
			out[i] = strings.TrimLeft(line, " \t")
		}
	}
	return strings.Join(out, "\n")
}
