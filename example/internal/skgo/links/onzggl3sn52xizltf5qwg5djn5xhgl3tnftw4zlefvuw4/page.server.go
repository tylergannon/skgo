package signedin

import (
	"context"

	"github.com/tylergannon/skgo"
)

type PageData struct {
	User     string `json:"user"`
	Required bool   `json:"required"`
}

func pageLoad(ctx context.Context) (PageData, error) {
	user, _ := skgo.EventFrom(ctx).Cookie("skgo_actions_signin")
	required, _ := skgo.EventFrom(ctx).SearchParam("required")
	return PageData{User: user, Required: required == "1"}, nil
}

var _ = skgo.Load(pageLoad)
