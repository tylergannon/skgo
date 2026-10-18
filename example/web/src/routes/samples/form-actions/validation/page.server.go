// Package validation is a page whose action can refuse a submission and hand
// the visitor's input back to the page.
package validation

import (
	"context"
	"net/http"
	"net/mail"
	"sync"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/businesslogic/visitor"
)

// Profile is what the form edits.
type Profile struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// PageData is the saved profile.
type PageData struct {
	Profile Profile `json:"profile"`
}

// SaveResult is form after a successful save.
type SaveResult struct {
	Receipt string `json:"receipt"`
}

// ValidationFailure is form after a refused save: the fields as the visitor
// typed them, and what was wrong.
type ValidationFailure struct {
	Name       string `json:"name"`
	Email      string `json:"email"`
	EmailError string `json:"emailError"`
}

var profiles = struct {
	sync.Mutex
	byVisitor map[string]Profile
}{byVisitor: map[string]Profile{}}

func pageLoad(event PageRequestEvent) (PageData, error) {
	who := visitor.Of(event.Context())
	profiles.Lock()
	defer profiles.Unlock()
	profile, ok := profiles.byVisitor[who]
	if !ok {
		profile = Profile{Name: "Ada Lovelace", Email: "ada@example.test"}
	}
	return PageData{Profile: profile}, nil
}

// save refuses an invalid email with a 422 and the visitor's own input.
func save(ctx context.Context) (SaveResult, error) {
	request := skgo.EventFrom(ctx).Request()
	if err := request.ParseForm(); err != nil {
		return SaveResult{}, skgo.Errorf(http.StatusBadRequest, "Invalid form")
	}
	profile := Profile{
		Name:  request.PostForm.Get("name"),
		Email: request.PostForm.Get("email"),
	}
	if _, err := mail.ParseAddress(profile.Email); err != nil {
		return SaveResult{}, skgo.Fail(http.StatusUnprocessableEntity, ValidationFailure{
			Name:       profile.Name,
			Email:      profile.Email,
			EmailError: "Enter a valid email address",
		})
	}
	profiles.Lock()
	profiles.byVisitor[visitor.Of(ctx)] = profile
	profiles.Unlock()
	return SaveResult{Receipt: "Saved " + profile.Name}, nil
}

var _ = skgo.Load(pageLoad)
var _ = skgo.ActionWithFailure(save, ValidationFailure{})
