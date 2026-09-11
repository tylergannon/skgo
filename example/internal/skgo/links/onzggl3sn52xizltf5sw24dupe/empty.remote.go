// Package empty serves /empty, the page that proves a Go slice left at its
// zero value still crosses the wire as an array.
//
// Every function here returns a value a real handler would return on an early
// path: the work did not produce any rows, so the slice was never made. Go
// means "no rows" by that; encoding/json used to mean `null`, and the generated
// TypeScript says `Array<T>` — so the page threw on `.length`. The page beside
// this file reads `.length` on every one of them deliberately.
package empty

import (
	"context"

	"github.com/tylergannon/skgo"
)

// Diagnostic is one thing the parser had to say.
type Diagnostic struct {
	// Message is the complaint, in the parser's own words.
	Message string `json:"message"`
}

// Report is what a parse produced. Both slices are `Array<...>` in the
// generated TypeScript and neither is optional, so neither may be null on the
// wire.
type Report struct {
	// Title says what happened.
	Title string `json:"title"`
	// Diagnostics is what the parser complained about.
	Diagnostics []Diagnostic `json:"diagnostics"`
	// Models is what it resolved.
	Models []string `json:"models"`
}

// getReport answers with both slices left at their zero value, which is what a
// handler that returned early would send.
func getReport(ctx context.Context) (Report, error) {
	return Report{Title: "Parse failed"}, nil
}

// getModels returns a nil slice as the whole result, which is the shape
// `query((): Array<string> => ...)` declares.
func getModels(ctx context.Context) ([]string, error) {
	return nil, nil
}

// getKnownReport is the control. It is on the same page, through the same
// encoder, and its rows are named in the acceptance scenario — so a page that
// showed an empty state because it rendered nothing at all would fail here.
func getKnownReport(ctx context.Context) (Report, error) {
	return Report{
		Title:       "Parsed",
		Diagnostics: []Diagnostic{{Message: "unused import fmt"}, {Message: "missing return"}},
		Models:      []string{"opus", "haiku"},
	}, nil
}

// reparse is the same zero value coming back from a command rather than a
// query, which is a different encoder call on a different response envelope.
func reparse(ctx context.Context) (Report, error) {
	return Report{Title: "Reparsed"}, nil
}

var (
	_ = skgo.Query(getReport)
	_ = skgo.Query(getModels)
	_ = skgo.Query(getKnownReport)
	_ = skgo.Command(reparse)
)
