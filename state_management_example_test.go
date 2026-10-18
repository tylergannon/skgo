package skgo_test

import (
	"github.com/tylergannon/skgo"
)

// Session is who a visitor is signed in as.
type Session struct {
	User string
}

// Greeting is what the page shows.
type Greeting struct {
	Name string `json:"name"`
}

// Do not do this. user is one variable for every visitor at once.
var user Session

func sessionFor(event skgo.RequestEvent[struct{}, struct{}]) Session {
	id, _ := event.Cookie("session")
	return Session{User: id}
}

func greetingLoad(event skgo.RequestEvent[struct{}, struct{}]) (Greeting, error) {
	user = sessionFor(event)
	return Greeting{Name: user.User}, nil
}

// A package-level variable is shared by every request the process serves.
// This load stores the visitor in one, so two visitors loading the page at the
// same moment overwrite each other. Keep request state in locals, cookies or
// storage keyed by the visitor instead.
func ExampleLoad_sharedState() {
	_ = skgo.Load(greetingLoad)
}
