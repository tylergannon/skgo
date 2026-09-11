package account

import (
	"context"

	"github.com/tylergannon/skgo"
)

// PageData is the account overview.
type PageData struct {
	// ParentUser is the name the layout above loaded, read back through
	// Parent. It is here so the page can show that it really got its parent's
	// data rather than deriving the same answer twice.
	ParentUser string `json:"parentUser"`
}

func pageLoad(ctx context.Context) (PageData, error) {
	parent, err := skgo.Parent[LayoutData](ctx)
	if err != nil {
		return PageData{}, err
	}
	return PageData{ParentUser: parent.AccountUser}, nil
}

var _ = skgo.Load(pageLoad)
