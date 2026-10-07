package ordinaryold

import (
	"github.com/tylergannon/skgo"
)

type Data struct{}

func pageLoad(PageRequestEvent) (Data, error) {
	return Data{}, &skgo.Redirect{Status: 307, Location: "/ordinary?from=legacy"}
}

var _ = skgo.Load(pageLoad)
