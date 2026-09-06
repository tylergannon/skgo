// Package messages ports the junkyard guestbook's messages.remote.ts.
package messages

import (
	"context"
	"strconv"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/survey/store"
)

const sessionCookie = "session"

func author(ctx context.Context) string {
	name, _ := skgo.EventFrom(ctx).Cookie(sessionCookie)
	if name == "" {
		return "anonymous"
	}
	return name
}

// getMessages is the port of `query(() => store.list())`.
func getMessages(ctx context.Context, _ skgo.None) ([]store.Message, error) {
	return store.Default.List(), nil
}

// getMessage backs the /messages/[id] page, which the junkyard app served
// from a `+page.server.ts` load that raised a 404.
func getMessage(ctx context.Context, id string) (store.Message, error) {
	n, err := strconv.Atoi(id)
	if err != nil {
		return store.Message{}, skgo.Errorf(404, "no message #%s", id)
	}
	m, ok := store.Default.Get(n)
	if !ok {
		return store.Message{}, skgo.Errorf(404, "no message #%s", id)
	}
	return m, nil
}

// getMessageCount is the port of `query.live`.
func getMessageCount(ctx context.Context, _ skgo.None, yield func(int) error) error {
	updates, unsubscribe, count := store.Default.Watch()
	defer unsubscribe()

	if err := yield(count); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case n := <-updates:
			if err := yield(n); err != nil {
				return err
			}
		}
	}
}

// postMessage is the port of the `command`, including the single-flight
// refresh of getMessages that rode back with the command response.
func postMessage(ctx context.Context, text string) (int, error) {
	if text == "" {
		return 0, skgo.Errorf(400, "text is required")
	}
	return store.Default.Add(author(ctx), text, nil).ID, nil
}

// Banner is the payload of the prerendered remote function.
type Banner struct {
	Text string `json:"text"`
}

// getBanner is the port of `prerender(() => ({ text: ... }))`.
func getBanner(ctx context.Context, _ skgo.None) (Banner, error) {
	return Banner{Text: "guestbook — prerendered banner"}, nil
}

var (
	_ = skgo.Query(getMessages)
	_ = skgo.Query(getMessage)
	_ = skgo.LiveQuery(getMessageCount)
	_ = skgo.Command(postMessage)
	_ = skgo.Query(getBanner)
)
