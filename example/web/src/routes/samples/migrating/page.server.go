// Package migratingsample is the Migrating guide's sample: a form action and a
// remote query that were TypeScript in a Node SvelteKit app, written in Go.
package migratingsample

import (
	"context"
	"net/http"
	"net/mail"

	"github.com/tylergannon/skgo"
)

// SaveResult is the action's `form` when it succeeds.
type SaveResult struct {
	Receipt string `json:"receipt"`
}

// SaveFailure is the action's `form` when it fails: what was submitted, so the
// page can keep the field filled, and what was wrong with it.
type SaveFailure struct {
	Email      string `json:"email"`
	EmailError string `json:"emailError"`
}

// save reads the submission with net/http. Where the Node action returned
// `fail(422, { ... })`, it returns skgo.Fail.
func save(ctx context.Context) (SaveResult, error) {
	request := skgo.EventFrom(ctx).Request()
	if err := request.ParseForm(); err != nil {
		return SaveResult{}, skgo.Errorf(http.StatusBadRequest, "Invalid form")
	}
	email := request.PostForm.Get("email")
	if _, err := mail.ParseAddress(email); err != nil {
		return SaveResult{}, skgo.Fail(http.StatusUnprocessableEntity, SaveFailure{
			Email: email, EmailError: "Enter a valid email address",
		})
	}
	return SaveResult{Receipt: "Saved " + email}, nil
}

var _ = skgo.ActionWithFailure(save, SaveFailure{})
