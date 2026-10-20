// Package testingsample holds the remote functions the Testing guide's Go
// tests call through the production handler: a query, a command and a form.
package testingsample

import (
	"context"
	"strings"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/internal/skgo/params"
)

// shout is a query with an argument.
func shout(ctx context.Context, word string) (string, error) {
	return strings.ToUpper(word) + "!", nil
}

// double is a command.
func double(ctx context.Context, event params.RequestEvent, n int) (int, error) {
	return 2 * n, nil
}

// Signup is the argument of the signUp form. It is a flat struct of scalar
// fields, so the generator also writes a Go client method for the form.
type Signup struct {
	Email string `json:"email"`
}

// Welcome is what signUp answers.
type Welcome struct {
	Email string `json:"email"`
}

// signUp is a form whose validation failure names the field it is about.
func signUp(ctx context.Context, event params.RequestEvent, in Signup) (Welcome, error) {
	if in.Email == "" {
		return Welcome{}, skgo.Invalidf("email", "An email is required")
	}
	return Welcome(in), nil
}

var (
	_ = skgo.Query(shout)
	_ = skgo.Command(double)
	_ = skgo.Form(signUp)
)
