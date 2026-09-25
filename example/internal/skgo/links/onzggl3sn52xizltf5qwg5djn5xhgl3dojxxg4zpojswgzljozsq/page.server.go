package crossreceive

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"strings"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/actiondemo"
	"github.com/tylergannon/skgo/example/businesslogic"
)

type PageData struct {
	Profile actiondemo.Profile `json:"profile"`
}

type SaveResult struct {
	Receipt string              `json:"receipt"`
	Price   businesslogic.Money `json:"price"`
}

type ValidationFailure struct {
	Name       string `json:"name"`
	Email      string `json:"email"`
	Biography  string `json:"biography"`
	EmailError string `json:"emailError"`
}

func pageLoad(ctx context.Context) (PageData, error) {
	id, err := actiondemo.Visitor(ctx)
	if err != nil {
		return PageData{}, err
	}
	return PageData{Profile: actiondemo.CrossProfile(id)}, nil
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
	profile := actiondemo.Profile{
		Name: r.PostForm.Get("name"), Email: r.PostForm.Get("email"),
		Biography: r.PostForm.Get("biography"), State: "Active",
	}
	address, parseErr := mail.ParseAddress(profile.Email)
	if parseErr != nil || address.Address != profile.Email || !strings.Contains(strings.SplitN(profile.Email, "@", 2)[1], ".") {
		return SaveResult{}, skgo.Fail(http.StatusUnprocessableEntity, ValidationFailure{
			Name: profile.Name, Email: profile.Email, Biography: profile.Biography,
			EmailError: "Enter a valid email address",
		})
	}
	actiondemo.SaveCrossProfile(id, profile)
	return SaveResult{Receipt: "Saved " + profile.Name, Price: businesslogic.Money{Cents: 750}}, nil
}

func signIn(ctx context.Context) error {
	r := skgo.EventFrom(ctx).Request()
	if err := r.ParseForm(); err != nil {
		return skgo.Errorf(http.StatusBadRequest, "Invalid form")
	}
	if r.PostForm.Get("username") != "ada" {
		return skgo.Errorf(http.StatusBadRequest, "Unknown demo user")
	}
	if err := skgo.EventFrom(ctx).SetCookie("skgo_actions_signin", "ada", skgo.CookieOptions{Path: "/actions"}); err != nil {
		return err
	}
	return &skgo.Redirect{Status: http.StatusSeeOther, Location: "/actions/signed-in"}
}

func forbidden(context.Context) error {
	return skgo.Errorf(http.StatusForbidden, "You cannot edit this profile")
}

func unavailable(context.Context) error {
	return errors.New("private-actions-database-token-4731")
}

var _ = skgo.Load(pageLoad)
var _ = skgo.ActionWithFailure(save, ValidationFailure{})
var _ = skgo.ActionNoData(signIn)
var _ = skgo.ActionNoData(forbidden)
var _ = skgo.ActionNoData(unavailable)
