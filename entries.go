package skgo

import (
	"encoding/json"
	"fmt"
	"io"
)

// Entries declares a route's build-time entry producer in page.server.go or
// server.go. The function takes no RequestEvent and returns this route's
// generated EntryParams values. Kit invokes it during route analysis, before
// prerendering; universal +page.ts entries take precedence over server entries.
func Entries[P any](fn func() ([]P, error)) Marker { _ = fn; return Marker{} }

// RouteEntries is a generated registration for a route's build-time entries.
// Application code declares the producer with Entries.
type RouteEntries struct {
	module string
	run    func() ([]byte, error)
}

// NewRouteEntries connects a generated entry producer to the build service.
func NewRouteEntries[P any](module string, fn func() ([]P, error)) *RouteEntries {
	return &RouteEntries{module: module, run: func() ([]byte, error) {
		values, err := fn()
		if err != nil {
			return nil, err
		}
		if values == nil {
			values = []P{}
		}
		return json.Marshal(struct {
			Entries []P `json:"entries"`
		}{values})
	}}
}

func runPrerenderEntries(raw []byte, out io.Writer, entries []*RouteEntries) (err error) {
	var input struct {
		Module string `json:"module"`
	}
	if err = json.Unmarshal(raw, &input); err != nil {
		return fmt.Errorf("skgo: entries request: %w", err)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("skgo: entries for %s panicked: %v", input.Module, recovered)
		}
	}()
	for _, entry := range entries {
		if entry.module != input.Module {
			continue
		}
		data, err := entry.run()
		if err != nil {
			return fmt.Errorf("skgo: entries for %s: %w", input.Module, err)
		}
		_, err = out.Write(data)
		return err
	}
	return fmt.Errorf("skgo: entries for %s are not registered", input.Module)
}
