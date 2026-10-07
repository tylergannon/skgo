package about

import (
	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/businesslogic"
)

// PageData exercises the values a Go load hands Kit while it prerenders.
type PageData struct {
	ParentDeployment string                `json:"parentDeployment"`
	Price            businesslogic.Money   `json:"price"`
	Later            skgo.Deferred[string] `json:"later"`
}

func pageLoad(event PageRequestEvent) (PageData, error) {
	ctx := event.Context()
	parent, err := skgo.Parent[struct {
		Deployment string `json:"deployment"`
	}](ctx)
	if err != nil {
		return PageData{}, err
	}
	return PageData{
		ParentDeployment: parent.Deployment,
		Price:            businesslogic.Money{Cents: 750},
		Later:            skgo.Resolved("GO_PRERENDER_DEFERRED"),
	}, nil
}

var _ = skgo.Load(pageLoad)
