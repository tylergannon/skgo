// Package named is a page with two named actions, ?/save and ?/archive.
package named

import (
	"context"
	"net/http"

	"github.com/tylergannon/skgo"
)

// Receipt is what either action hands back to the page as form.
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

func archive(ctx context.Context) (Receipt, error) {
	return Receipt{Receipt: "Archived"}, nil
}

// Each action is named after its function.
var _ = skgo.Action(save)
var _ = skgo.Action(archive)
