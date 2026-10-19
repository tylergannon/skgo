package skgo_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/tylergannon/skgo"
)

// A handleError hook is a Go function set on the application's
// configuration. What it returns is merged over the error the page receives:
// here every failure gets a support id, and an unexpected one gets a message a
// person wrote in place of "Internal Error". The original error stays in
// caught.Err for logging and never reaches the visitor unless the hook copies
// it. skgo calls the hook itself; the Example calls it directly to show what
// it returns for an unexpected error.
func ExampleApplicationConfig_unexpectedError() {
	config := skgo.ApplicationConfig{
		HandleError: func(ctx context.Context, caught skgo.CaughtError) map[string]any {
			extra := map[string]any{"supportId": "case-1121"}
			if caught.Kind == "unknown" {
				extra["message"] = "Something went wrong on our end."
			}
			return extra
		},
	}

	public := config.HandleError(context.Background(), skgo.CaughtError{
		Kind:    "unknown",
		Status:  500,
		Message: "Internal Error",
		Err:     errors.New("the connection string is postgres://ada:hunter2@db"),
	})
	fmt.Println(public)
	// Output: map[message:Something went wrong on our end. supportId:case-1121]
}
