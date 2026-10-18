// Package redirect holds a query that redirects the visitor instead of
// answering.
//
// Kit treats a redirect thrown while a page renders as an answer for the whole
// document rather than for the component that asked: `transformError` refuses
// to swallow it (`page/render.js`) and `render_page`'s catch turns it into the
// same bare 3xx a load's redirect produces. There is no document at all — no
// markup, no boot script, no error page.
package redirect

import (
	"context"

	"github.com/tylergannon/skgo"
)

// Destination is what this query would have answered with.
type Destination struct {
	// Where is the path the visitor is sent to.
	Where string `json:"where"`
}

func whereTo(_ context.Context) (Destination, error) {
	return Destination{}, &skgo.Redirect{Status: 307, Location: "/about"}
}

var _ = skgo.Query(whereTo)
