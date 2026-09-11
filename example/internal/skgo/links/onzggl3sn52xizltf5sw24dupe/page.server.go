package empty

import (
	"context"

	"github.com/tylergannon/skgo"
)

// PageData is the load's half of the same claim. Load data does not travel on
// the remote endpoint at all — it is serialized into the document and into
// `__data.json` — so it is a surface of its own.
type PageData struct {
	// Notes is left at its zero value on purpose.
	Notes []string `json:"notes"`
}

func pageLoad(ctx context.Context) (PageData, error) {
	return PageData{}, nil
}

var _ = skgo.Load(pageLoad)
