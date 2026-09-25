package saveddefault

import (
	"context"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/actiondemo"
)

type PageData struct {
	Saved   bool               `json:"saved"`
	Profile actiondemo.Profile `json:"profile"`
}

func pageLoad(ctx context.Context) (PageData, error) {
	id, err := actiondemo.Visitor(ctx)
	if err != nil {
		return PageData{}, err
	}
	profile, saved := actiondemo.DefaultProfile(id)
	return PageData{Saved: saved, Profile: profile}, nil
}

var _ = skgo.Load(pageLoad)
