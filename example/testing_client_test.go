package example_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/internal/skgo/client"
	signup "github.com/tylergannon/skgo/example/internal/skgo/links/onzggl3sn52xizltf5zwc3lqnrsxgl3umvzxi2lom4"
)

// TestTheGeneratedClientSubmitsAForm submits the signUp form over HTTP as
// Kit's enhanced form would, and gets the field's validation issue back.
//
// The client sends an Origin header derived from BaseURL, so the handler is
// built with the test server's own address.
func TestTheGeneratedClientSubmitsAForm(t *testing.T) {
	srv := httptest.NewUnstartedServer(nil)
	srv.Config.Handler = newHandlerFor(t, "http://"+srv.Listener.Addr().String())
	srv.Start()
	defer srv.Close()
	c := client.Client{FormClient: skgo.FormClient{BaseURL: srv.URL}}

	_, err := c.SignUp(context.Background(), signup.Signup{})
	var invalid *skgo.Invalid
	if !errors.As(err, &invalid) || len(invalid.Issues) != 1 ||
		invalid.Issues[0] != (skgo.Issue{Field: "email", Message: "An email is required"}) {
		t.Fatalf("an empty email answered %v", err)
	}

	welcome, err := c.SignUp(context.Background(), signup.Signup{Email: "ada@example.test"})
	if err != nil || welcome.Email != "ada@example.test" {
		t.Fatalf("a valid email answered %+v, %v", welcome, err)
	}
}
