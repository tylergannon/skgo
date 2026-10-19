package remotefunctions

import (
	"context"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/internal/skgo/params"
)

// Rename is the argument of renameNote.
type Rename struct {
	// ID names the note to change.
	ID string `json:"id"`
	// Text is its new text.
	Text string `json:"text"`
}

// renameNote changes one note's text and refreshes that note's getNote.
//
// The page that calls it passes no `.updates(...)`: the server decides what to
// refresh, and names the query function and its argument in Go, so getNote's
// own parameter type is what arg.ID has to satisfy.
func renameNote(ctx context.Context, event params.RequestEvent, arg Rename) (Note, error) {
	var note Note
	write(ctx, func(list []Note) []Note {
		for i := range list {
			if list[i].ID == arg.ID {
				list[i].Text = arg.Text
				note = list[i]
			}
		}
		return list
	})
	if note.ID == "" {
		return Note{}, skgo.Errorf(404, "No note with id %q", arg.ID)
	}
	return note, skgo.Refresh(ctx, getNote, arg.ID)
}

var _ = skgo.Command(renameNote)
