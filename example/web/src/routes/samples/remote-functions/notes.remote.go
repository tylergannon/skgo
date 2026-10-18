// Package remotefunctions is the sample behind the remote functions guide: a
// list of notes read with queries, written with commands and watched with a
// live query.
package remotefunctions

import (
	"context"
	"slices"
	"sync"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/businesslogic/visitor"
)

// Note is one note on the list.
type Note struct {
	// ID identifies the note.
	ID string `json:"id"`
	// Text is what the note says.
	Text string `json:"text"`
}

// list is one visitor's notes. changed is closed and replaced on every write,
// which wakes anything waiting for the list to change.
type list struct {
	notes   []Note
	changed chan struct{}
}

// lists holds every visitor's notes in memory, keyed by the visitor id the
// root layout hands each browser, so one visitor never sees another's notes.
var lists = struct {
	sync.Mutex
	byVisitor map[string]*list
}{byVisitor: map[string]*list{}}

// mine is the list of the visitor ctx belongs to. Call it with lists locked.
func mine(ctx context.Context) *list {
	who := visitor.Of(ctx)
	l, ok := lists.byVisitor[who]
	if !ok {
		l = &list{changed: make(chan struct{})}
		lists.byVisitor[who] = l
	}
	return l
}

// write applies change to this visitor's list under the lock and wakes every
// waiter.
func write(ctx context.Context, change func([]Note) []Note) {
	lists.Lock()
	defer lists.Unlock()
	l := mine(ctx)
	l.notes = change(l.notes)
	close(l.changed)
	l.changed = make(chan struct{})
}

// getNotes lists this visitor's notes.
func getNotes(ctx context.Context) ([]Note, error) {
	lists.Lock()
	defer lists.Unlock()
	return slices.Clone(mine(ctx).notes), nil
}

// getNote looks one note up by id.
func getNote(ctx context.Context, id string) (Note, error) {
	lists.Lock()
	defer lists.Unlock()
	for _, note := range mine(ctx).notes {
		if note.ID == id {
			return note, nil
		}
	}
	return Note{}, skgo.Errorf(404, "No note with id %q", id)
}

var (
	_ = skgo.Query(getNotes)
	_ = skgo.Query(getNote)
)
