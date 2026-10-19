package gostandards

import (
	"context"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/internal/skgo/params"
)

// Note is what the note form submits. Each field's JSON name is the field name
// the page uses in `sendNote.fields`.
type Note struct {
	// Title is the `title` text input.
	Title string `json:"title"`
	// Attachment is the `attachment` file input. Its bytes are already read
	// by the time sendNote runs.
	Attachment skgo.File `json:"attachment"`
}

// NoteReceipt is what sendNote answers.
type NoteReceipt struct {
	Title string `json:"title"`
	File  string `json:"file"`
	Type  string `json:"type"`
	Bytes int    `json:"bytes"`
}

func sendNote(ctx context.Context, _ params.RequestEvent, note Note) (NoteReceipt, error) {
	if note.Title == "" {
		return NoteReceipt{}, skgo.Invalidf("title", "A note needs a title")
	}
	return NoteReceipt{
		Title: note.Title,
		File:  note.Attachment.Name,
		Type:  note.Attachment.Type,
		Bytes: len(note.Attachment.Data),
	}, nil
}

var _ = skgo.Form(sendNote)
