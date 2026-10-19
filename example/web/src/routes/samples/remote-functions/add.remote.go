package remotefunctions

import (
	"context"
	"strconv"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/internal/skgo/params"
)

// addNote appends a note to the list.
//
// The page writes `addNote(text).updates(getNotes())`. A command runs the
// requested queries it names and no others, so the last line is what puts the
// new list on the response.
func addNote(ctx context.Context, event params.RequestEvent, text string) (Note, error) {
	if text == "" {
		return Note{}, skgo.Errorf(400, "A note needs some text")
	}
	var note Note
	write(ctx, func(list []Note) []Note {
		note = Note{ID: strconv.Itoa(len(list) + 1), Text: text}
		return append(list, note)
	})
	return note, skgo.RefreshRequestedNoArg(ctx, getNotes)
}

var _ = skgo.Command(addNote)
