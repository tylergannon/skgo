package profileeditor

import (
	"context"
	"net/http"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/actiondemo"
)

type PageData struct {
	Selected string             `json:"selected"`
	Ada      actiondemo.Profile `json:"ada"`
	Grace    actiondemo.Profile `json:"grace"`
}

type SaveResult struct {
	Receipt string `json:"receipt"`
}

func selected(ctx context.Context) (string, error) {
	name := skgo.EventFrom(ctx).Param("profile")
	if name != "ada" && name != "grace" {
		return "", skgo.Errorf(http.StatusNotFound, "Profile not found")
	}
	return name, nil
}

func pageLoad(ctx context.Context) (PageData, error) {
	name, err := selected(ctx)
	if err != nil {
		return PageData{}, err
	}
	id, err := actiondemo.Visitor(ctx)
	if err != nil {
		return PageData{}, err
	}
	pair := actiondemo.Profiles(id)
	return PageData{Selected: name, Ada: pair.Ada, Grace: pair.Grace}, nil
}

func save(ctx context.Context) (SaveResult, error) {
	name, err := selected(ctx)
	if err != nil {
		return SaveResult{}, err
	}
	id, err := actiondemo.Visitor(ctx)
	if err != nil {
		return SaveResult{}, err
	}
	r := skgo.EventFrom(ctx).Request()
	if err := r.ParseForm(); err != nil {
		return SaveResult{}, skgo.Errorf(http.StatusBadRequest, "Invalid form")
	}
	profile := actiondemo.Profile{Name: r.PostForm.Get("name"), Email: r.PostForm.Get("email"), Biography: r.PostForm.Get("biography"), State: "Active"}
	if !actiondemo.SaveSelected(id, name, profile) {
		return SaveResult{}, skgo.Errorf(http.StatusNotFound, "Profile not found")
	}
	return SaveResult{Receipt: "Saved " + profile.Name}, nil
}

var _ = skgo.Load(pageLoad)
var _ = skgo.Action(save)
