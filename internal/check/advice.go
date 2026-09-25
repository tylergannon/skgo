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
	dec := json.NewDecoder(bytes.NewReader(output))
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
			c.Message = "Go analysis returned incomplete JSON: " + err.Error() + ": " + strings.TrimSpace(string(output))
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
	if runErr != nil && len(ds) == 0 {
		c.Status = "failed"
		c.Message = fmt.Sprintf("Go analysis stopped: %v: %s", runErr, strings.TrimSpace(string(output)))
	}
	return c, ds
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
