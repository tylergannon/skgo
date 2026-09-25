package defaultaction

import (
	"context"
	"net/http"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/actiondemo"
)

type SaveResult struct {
	Receipt   string `json:"receipt"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Biography string `json:"biography"`
	State     string `json:"state"`
}

func save(ctx context.Context) (SaveResult, error) {
	id, err := actiondemo.Visitor(ctx)
	if err != nil {
		return SaveResult{}, err
	}
	r := skgo.EventFrom(ctx).Request()
	if err := r.ParseForm(); err != nil {
		return SaveResult{}, skgo.Errorf(http.StatusBadRequest, "Invalid form")
	}
	profile := actiondemo.SaveDefault(id, actiondemo.Profile{
		Name: r.PostForm.Get("name"), Email: r.PostForm.Get("email"),
		Biography: r.PostForm.Get("biography"), State: "Active",
	})
	return SaveResult{Receipt: "Saved " + profile.Name, Name: profile.Name,
		Email: profile.Email, Biography: profile.Biography, State: profile.State}, nil
}

var _ = skgo.DefaultAction(save)
