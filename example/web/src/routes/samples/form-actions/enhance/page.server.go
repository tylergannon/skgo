// Package enhance is a page whose form is progressively enhanced.
package enhance

import (
	"context"
	"net/http"

	"github.com/tylergannon/skgo"
)

// Receipt is what the page sees as form after a save.
type Receipt struct {
	Receipt string `json:"receipt"`
}

func save(ctx context.Context) (Receipt, error) {
	request := skgo.EventFrom(ctx).Request()
	if err := request.ParseForm(); err != nil {
		return Receipt{}, skgo.Errorf(http.StatusBadRequest, "Invalid form")
	}
	return Receipt{Receipt: "Saved " + request.PostForm.Get("name")}, nil
}

var _ = skgo.Action(save)
