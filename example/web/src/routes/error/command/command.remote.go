// Package command holds a command that a page calls while it renders, which is
// a thing kit forbids.
//
// A command is a mutation. Kit refuses one during server-side rendering
// (`runtime/app/server/remote/command.js`: `if (state.is_in_render) throw`)
// because a document is produced on every navigation and a page that mutated
// while it rendered would mutate again on every reload. The refusal happens
// inside kit's own wrapper, before the call could reach Go, so this function's
// body is never run by the render that names it.
package command

import (
	"context"
	"sync"

	"github.com/tylergannon/skgo"
)

// Tally is how many times the command has run.
type Tally struct {
	// Runs is the count.
	Runs int `json:"runs"`
}

var runs struct {
	sync.Mutex
	n int
}

// bumpTally is the command. Calling it from markup is the mistake the page
// makes; calling it from a button is what it is for.
func bumpTally(_ context.Context) (Tally, error) {
	runs.Lock()
	defer runs.Unlock()
	runs.n++
	return Tally{Runs: runs.n}, nil
}

// getTally reads the count without mutating anything, which is what a query is
// allowed to do during a render.
func getTally(_ context.Context) (Tally, error) {
	runs.Lock()
	defer runs.Unlock()
	return Tally{Runs: runs.n}, nil
}

var (
	_ = skgo.Command(bumpTally)
	_ = skgo.Query(getTally)
)
