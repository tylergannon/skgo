package businesslogic

import "sync"

// stores holds one Store per visitor. A visitor is whoever carries one visitor
// id, which in the example app is one browser: its tabs share a cookie and so
// share a list, and a second browser starts from the fixtures.
var stores = struct {
	sync.Mutex
	byVisitor map[string]*Store
}{byVisitor: map[string]*Store{}}

// For returns the visitor's own Store, seeded with the same fixtures as
// NewStore the first time that visitor is seen. The empty visitor is Default,
// so a caller that names nobody gets the one list every such caller shares.
//
// Sessions are not the visitor's: a session id is already unique to the browser
// that holds it, and the app resolves it against Default wherever it is read.
// What For separates is the todos and the live subscribers watching them, so a
// visitor's count is never moved by another visitor's command.
func For(visitor string) *Store {
	if visitor == "" {
		return Default
	}
	stores.Lock()
	defer stores.Unlock()
	s, ok := stores.byVisitor[visitor]
	if !ok {
		s = NewStore()
		stores.byVisitor[visitor] = s
	}
	return s
}
