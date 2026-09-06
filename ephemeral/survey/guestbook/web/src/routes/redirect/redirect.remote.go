// Package redirect ports the junkyard app's `redirect(307, '/messages')` load.
// skgo.Redirect carries no status: the 307 is dropped.
package redirect

import (
	"context"

	"github.com/tylergannon/skgo"
)

func goSomewhereElse(ctx context.Context, _ skgo.None) (string, error) {
	return "", &skgo.Redirect{Location: "/messages"}
}

var _ = skgo.Query(goSomewhereElse)
