// Package gate serves /todos/gate: what a Go command does with the refreshes
// the browser asks it for.
//
// A page asks by writing `.updates(...)`, and kit's client posts the keys of
// the query instances it wants back. That list is client input — it is in the
// network tab, and anyone can post a longer one. So a command runs the queries
// its handler named and only those: `writeNotes` names `getNote` and accepts
// one instance of it, and says nothing at all about `getBanner`.
//
// The page below asks for three things on every write. One is honoured, one is
// past the limit and comes back refused, and one was never named and is not
// run — which is why the banner on screen goes stale while the value behind it
// has already changed. The reload button proves the difference.
package gate

import (
	"context"
	"sync"

	"github.com/tylergannon/skgo"
)

// Note is one panel's text. Two instances of getNote are on the page, told
// apart only by their argument, which is the key kit's client caches under.
type Note struct {
	// Name is which note this is: "left" or "right".
	Name string `json:"name"`
	// Text is what was last written to it.
	Text string `json:"text"`
}

// Banner is the page's third panel, and a different query entirely — not
// another instance of getNote. A handler's limit is per query function, so the
// only thing that keeps the banner from being refreshed is that nothing named
// it.
type Banner struct {
	// Text is what was last written to the banner.
	Text string `json:"text"`
}

// unwritten is what every panel reads before anything has been written to it.
const unwritten = "nothing written yet"

var store = struct {
	sync.Mutex
	notes  map[string]string
	banner string
}{
	notes:  map[string]string{"left": unwritten, "right": unwritten},
	banner: unwritten,
}

// getNote reads one note.
func getNote(_ context.Context, name string) (Note, error) {
	store.Lock()
	defer store.Unlock()
	text, ok := store.notes[name]
	if !ok {
		return Note{}, skgo.Errorf(404, "No note called %q", name)
	}
	return Note{Name: name, Text: text}, nil
}

// getBanner reads the banner.
func getBanner(_ context.Context) (Banner, error) {
	store.Lock()
	defer store.Unlock()
	return Banner{Text: store.banner}, nil
}

// Write is the argument of the writeNotes command: everything the page changes
// in one go.
type Write struct {
	// Left is the new text of the left note.
	Left string `json:"left"`
	// Right is the new text of the right note.
	Right string `json:"right"`
	// Banner is the new banner text.
	Banner string `json:"banner"`
}

// Ack is what the command returns, so the page can show that it landed even
// when nothing else on the page moved.
type Ack struct {
	// Wrote is how many values the command changed. It is always three, which
	// is the point: everything was written, and what the page shows afterwards
	// is a question about refreshes rather than about writes.
	Wrote int `json:"wrote"`
}

// writeNotes changes both notes and the banner.
//
// The page asks for three instances back — both notes and the banner. One line
// of this handler decides what that request is worth: getNote is named, with a
// limit of one, and getBanner is not named at all.
//
// So the left note comes back refreshed; the right note, being the second
// instance of getNote the page asked for, comes back refused with the reason
// on it; and the banner is not run, which the page shows by going stale.
func writeNotes(ctx context.Context, arg Write) (Ack, error) {
	store.Lock()
	store.notes["left"] = arg.Left
	store.notes["right"] = arg.Right
	store.banner = arg.Banner
	store.Unlock()

	return Ack{Wrote: 3}, skgo.RefreshRequested(ctx, getNote, 1)
}

var (
	_ = skgo.Query(getNote)
	_ = skgo.Query(getBanner)
	_ = skgo.Command(writeNotes)
)
