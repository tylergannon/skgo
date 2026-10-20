package check

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/tylergannon/skgo/internal/advice"
)

// parseAdvice reads Go vet's stream of per-package JSON objects. Go vet can
// exit zero while reporting diagnostics, so the structured findings decide
// whether the project is clean.
func parseAdvice(root string, routes routeInventory, output []byte, runErr error) (Check, []Diagnostic) {
	c := Check{Name: "skgo-advice", Status: "complete"}
	// Go vet prints compile errors as plain text beside its JSON, one `{}` per
	// package it could not analyze. Keep the text: it is the original diagnostic.
	jsonOutput, text := splitVetOutput(output)
	dec := json.NewDecoder(bytes.NewReader(jsonOutput))
	var ds []Diagnostic
	documents := 0
	for {
		var packages map[string]map[string][]struct {
			Posn     string `json:"posn"`
			End      string `json:"end"`
			Message  string `json:"message"`
			Category string `json:"category"`
		}
		err := dec.Decode(&packages)
		if err == io.EOF {
			break
		}
		if err != nil {
			c.Status = "incomplete"
			c.Message = "Go analysis returned incomplete JSON: " + err.Error() + ": " + boundedOutput(string(jsonOutput))
			return c, ds
		}
		documents++
		for _, analyzers := range packages {
			for analyzer, items := range analyzers {
				for _, item := range items {
					code := item.Category
					if code == "" {
						code = analyzer
					}
					d := Diagnostic{Source: "skgo-advice", Code: code, Severity: "error", Message: item.Message}
					if _, ok := advice.Lookup(code); ok {
						d.Documentation = "skgo advice " + code
					}
					if loc := parseVetPosition(root, routes, item.Posn); loc != nil {
						d.Location = loc
					}
					if end := parseVetPosition(root, routes, item.End); end != nil && d.Location != nil && end.File == d.Location.File {
						d.Location.EndLine = end.Line
						d.Location.EndColumn = end.Column
					}
					ds = append(ds, d)
				}
			}
		}
	}
	if documents == 0 {
		c.Status = "incomplete"
		c.Message = "Go analysis returned no package result"
	}
	if len(text) > 0 && c.Status == "complete" {
		c.Status = "incomplete"
		c.Message = "Go analysis reported errors before all analyzers completed: " + boundedOutput(strings.Join(text, "\n"))
	}
	if runErr != nil && len(ds) == 0 {
		c.Status = "failed"
		c.Message = fmt.Sprintf("Go analysis stopped: %v: %s", runErr, boundedOutput(strings.Join(text, "\n")+"\n"+string(jsonOutput)))
	}
	return c, ds
}

// splitVetOutput separates vet's JSON documents from the plain text lines the
// Go command writes around them: `# package` headers and compile errors.
func splitVetOutput(output []byte) (jsonOutput []byte, text []string) {
	var b bytes.Buffer
	for _, line := range strings.Split(string(output), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
		case strings.HasPrefix(trimmed, "# "):
			// The package header names a package the next line already locates.
		case strings.ContainsAny(trimmed[:1], "{}[]\""):
			b.WriteString(line)
			b.WriteByte('\n')
		default:
			text = append(text, trimmed)
		}
	}
	return b.Bytes(), text
}

// boundedOutput keeps a checker's own words but not a page of them.
func boundedOutput(s string) string {
	const limit = 20
	var kept []string
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "{}" {
			continue
		}
		kept = append(kept, line)
	}
	if len(kept) > limit {
		kept = append(kept[:limit], fmt.Sprintf("... %d more lines", len(kept)-limit))
	}
	return strings.Join(kept, "\n")
}

func parseVetPosition(root string, routes routeInventory, s string) *Location {
	if s == "" {
		return nil
	}
	m := positionRE.FindStringSubmatch(s + ": location")
	if m == nil {
		return nil
	}
	return &Location{File: authoredPath(root, routes, m[1]), Line: mustInt(m[2]), Column: mustInt(m[3])}
}

func mustInt(s string) int { n, _ := strconv.Atoi(s); return n }
