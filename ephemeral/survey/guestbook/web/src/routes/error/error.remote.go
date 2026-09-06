// Package errorroutes holds the two failure modes the junkyard app exercised:
// an expected `error(418, ...)` and an unexpected internal panic.
package errorroutes

import (
	"context"

	"github.com/tylergannon/skgo"
)

// expectedError is the port of `error(418, { message: 'expected teapot' })`.
func expectedError(ctx context.Context, _ skgo.None) (string, error) {
	return "", skgo.Errorf(418, "expected teapot")
}

// unexpectedError is the port of a load that throws a raw Error carrying a
// secret. Nothing about the message may reach the browser.
func unexpectedError(ctx context.Context, _ skgo.None) (string, error) {
	panic("SECRET-INTERNAL-DETAIL: database password is hunter2")
}

var (
	_ = skgo.Query(expectedError)
	_ = skgo.Query(unexpectedError)
)
