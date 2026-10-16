// Package devgen adapts an application-owned development generator to skgo dev.
// Import it only from a tooling main package, never an application entrypoint.
// Stdout is reserved for the protocol; write diagnostics to stderr.
package devgen

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

// Request selects one serialized generation pass. Paths are cumulative canonical
// absolute observed changes, including removed paths. Full requests require the
// generator to reconcile all its producers and owned outputs. Epoch is separate
// from Revision: non-Go inputs need not change SKGO's captured compiler inputs.
type Request struct {
	ID       uint64   `json:"id"`
	Epoch    uint64   `json:"epoch"`
	Revision uint64   `json:"revision"`
	Full     bool     `json:"full"`
	Paths    []string `json:"paths"`
}

// Response is the wire result. Generated means application-consumed owned output
// changed, was removed, or was repaired, including changes made before an error.
type Response struct {
	ID        uint64 `json:"id"`
	Generated bool   `json:"generated"`
	Error     string `json:"error,omitempty"`
}

// Serve runs callback serially until stdin closes or the protocol fails. Return
// changedSoFar on partial publication errors. Equal-byte writes, private cache
// updates, and analysis alone must return false. Callback state survives ordinary
// application replacement; helper source changes create a fresh process.
func Serve(ctx context.Context, callback func(context.Context, Request) (bool, error)) error {
	return ServeIO(ctx, os.Stdin, os.Stdout, callback)
}

// ServeIO is Serve with explicit streams, useful for tooling transport tests.
func ServeIO(ctx context.Context, in io.Reader, out io.Writer, callback func(context.Context, Request) (bool, error)) error {
	dec, enc := json.NewDecoder(in), json.NewEncoder(out)
	var previous uint64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var req Request
		if err := dec.Decode(&req); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("devgen request: %w", err)
		}
		if req.ID == 0 || req.ID <= previous {
			return fmt.Errorf("devgen: non-monotonic request id %d", req.ID)
		}
		previous = req.ID
		changed, err := callback(ctx, req)
		res := Response{ID: req.ID, Generated: changed}
		if err != nil {
			res.Error = err.Error()
		}
		if err := enc.Encode(res); err != nil {
			return fmt.Errorf("devgen response: %w", err)
		}
	}
}
