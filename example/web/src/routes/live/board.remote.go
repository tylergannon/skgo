// Package live serves the /live route: a board whose value is pushed by Go and
// arrives in the document already rendered.
package live

import (
	"context"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/businesslogic"
)

// sessionCookie is the name of the cookie the auth functions in src/lib set.
const sessionCookie = "skgo_session"

// Board is what the live board shows.
type Board struct {
	// Count is how many todos this visitor may see.
	Count int `json:"count"`
	// Newest is the text of the newest one, or "" when there are none.
	Newest string `json:"newest"`
	// Push is which value of this stream this is: 1 for the one the render was
	// answered with, 2 for the first update after it, and so on.
	//
	// It is the difference between "the board shows a number" and "the board
	// was pushed a new one". A page that quietly refetched instead of
	// listening would show push 1 for ever.
	Push int `json:"push"`
}

// watchBoard pushes the board now and after every change, until the client
// disconnects.
//
// The identity is read once, when the stream opens, because that is what kit's
// event is: a snapshot of the request that opened it. See watchCount in
// src/routes/todos/todos.remote.go.
func watchBoard(ctx context.Context, yield func(Board) error) error {
	id, _ := skgo.EventFrom(ctx).Cookie(sessionCookie)
	signedIn := businesslogic.Default.Session(id).User != ""

	updates, unsubscribe, now := businesslogic.Default.Watch(signedIn)
	defer unsubscribe()

	push := 1
	if err := yield(Board{Count: now.Count, Newest: now.Newest, Push: push}); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case next := <-updates:
			push++
			if err := yield(Board{Count: next.Count, Newest: next.Newest, Push: push}); err != nil {
				return err
			}
		}
	}
}

var _ = skgo.LiveQuery(watchBoard)
