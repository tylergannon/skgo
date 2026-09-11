package pricing

import (
	"context"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/businesslogic"
)

// PageData is what the pricing page is given before it renders.
//
// It exists to carry a domain type down inside the document. A remote query
// under a boundary with a `pending` snippet is not called while the page is
// server-rendered — Svelte renders the snippet instead — so `getPlans` reaches
// the browser in a request of its own. A server load always runs, which makes
// this the path a custom type takes through the document itself.
type PageData struct {
	// Featured is the plan the page leads with. Its price is a
	// businesslogic.Money, and it is not one of the plans getPlans answers
	// with, so nothing on the page can show this amount by accident.
	Featured Plan `json:"featured"`
}

func pageLoad(ctx context.Context) (PageData, error) {
	return PageData{Featured: Plan{Name: "Startup", Price: businesslogic.USD(45)}}, nil
}

var _ = skgo.Load(pageLoad)
