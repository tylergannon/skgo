package signedin

import (
	"github.com/tylergannon/skgo"
)

type PageData struct {
	User     string `json:"user"`
	Required bool   `json:"required"`
}

func pageLoad(event RequestEvent) (PageData, error) {
	user, _ := event.Cookie("skgo_actions_signin")
	required, _ := event.SearchParam("required")
	return PageData{User: user, Required: required == "1"}, nil
}

var _ = skgo.Load(pageLoad)
