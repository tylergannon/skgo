// Package hooks is the app's universal hooks, the Go half of what
// `src/hooks.ts` declares beside it.
//
// Kit's `transport` is a universal hook because both sides of the wire need it:
// the browser needs `decode` to rebuild a class instance, and the server needs
// `encode` to say what that instance is made of. The two halves are one
// declaration, so they live next to each other.
package hooks

import (
	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/businesslogic"
)

// Money crosses the wire as a custom type. Without this a price arrives in the
// browser as a plain `{ cents: 2000 }` and `price.format()` throws.
var _ = skgo.Transported[businesslogic.Money]("Money")
