package migratingsample

import (
	"context"
	"net/http"

	"github.com/tylergannon/skgo"
)

// greet was `query(v.string(), ...)` in greeting.remote.ts. The Go argument
// type takes the validator's place: an argument that does not decode into a
// string is refused before greet runs, and greet checks the value.
func greet(ctx context.Context, name string) (string, error) {
	if name == "" {
		return "", skgo.Errorf(http.StatusBadRequest, "Greet someone")
	}
	return "Hello, " + name, nil
}

var _ = skgo.Query(greet)
