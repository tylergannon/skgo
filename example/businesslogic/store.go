// Package businesslogic is the example app's domain: a list of todos, some of
// them private, and the sessions that decide who may see them.
//
// Nothing here knows about HTTP or about SvelteKit. The remote functions in
// web/src call into it.
package businesslogic

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
)

// Todo is one item in the list.
type Todo struct {
	// ID is the stable identifier the todo is addressed by.
	ID string `json:"id"`
	// Text is what the visitor typed.
	Text string `json:"text"`
	// Private reports that only a signed-in visitor may see this todo.
	Private bool `json:"private"`
}

// Session says who is signed in. An empty User means nobody.
type Session struct {
	// User is the signed-in visitor's name, or "" when signed out.
	User string `json:"user"`
}

// Store is the app's entire state.
type Store struct {
	mu       sync.Mutex
	todos    []Todo
	next     int
	sessions map[string]string
	subs     map[*subscriber]struct{}
}

// subscriber is one open Watch. It remembers whether its visitor is signed in,
// because the count it is sent has to be the count that visitor may see — a
// subscriber is never told about a todo it could not have listed.
type subscriber struct {
	ch       chan Snapshot
	signedIn bool
}

// Default is the store the example app serves.
var Default = NewStore()

func NewStore() *Store {
	return &Store{
		todos: []Todo{
			{ID: "t1", Text: "write the adapter"},
			{ID: "t2", Text: "serve remote functions"},
			{ID: "t3", Text: "ship the private roadmap", Private: true},
			// The pair page shows these two side by side, so a command can
			// refresh one of them and be seen to leave the other alone. They
			// are separate rows from t1 and t2 because that page rewrites
			// them, and a fixture two pages disagree about is a scenario that
			// passes or fails on the order the suite happened to run in.
			{ID: "p1", Text: "the left of the pair"},
			{ID: "p2", Text: "the right of the pair"},
		},
		next:     6,
		sessions: map[string]string{},
		subs:     map[*subscriber]struct{}{},
	}
}

// Todos returns what a visitor may see. A signed-out visitor never learns that
// the private todos exist.
func (s *Store) Todos(signedIn bool) []Todo {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Todo, 0, len(s.todos))
	for _, todo := range s.todos {
		if !visible(todo, signedIn) {
			continue
		}
		out = append(out, todo)
	}
	return out
}

// visible is the one rule about who may see what. Todos, Todo and Count all
// apply it, so a count can never disagree with the list it counts.
func visible(todo Todo, signedIn bool) bool {
	return signedIn || !todo.Private
}

// Snapshot is what a live subscriber is sent after every change: how many
// todos this visitor may see, and the text of the newest one they may see.
//
// It is one value rather than two subscriptions because the two have to agree:
// a board that showed a count from one moment beside a newest from another
// would be a page that never existed.
type Snapshot struct {
	Count  int
	Newest string
}

// snapshot is what a visitor may see right now. Callers hold s.mu.
func (s *Store) snapshot(signedIn bool) Snapshot {
	snap := Snapshot{}
	for _, todo := range s.todos {
		if !visible(todo, signedIn) {
			continue
		}
		snap.Count++
		snap.Newest = todo.Text
	}
	return snap
}

// count reports how many todos a visitor may see. Callers hold s.mu.
func (s *Store) count(signedIn bool) int {
	n := 0
	for _, todo := range s.todos {
		if visible(todo, signedIn) {
			n++
		}
	}
	return n
}

// Todo looks one up, applying the same visibility rule.
func (s *Store) Todo(id string, signedIn bool) (Todo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, todo := range s.todos {
		if todo.ID == id && visible(todo, signedIn) {
			return todo, true
		}
	}
	return Todo{}, false
}

// Add appends a todo and wakes every live-query subscriber.
func (s *Store) Add(text string) Todo {
	s.mu.Lock()
	todo := Todo{ID: fmt.Sprintf("t%d", s.next), Text: text}
	s.next++
	s.todos = append(s.todos, todo)
	s.notify()
	s.mu.Unlock()
	return todo
}

// notify wakes every live subscriber with what its own visitor may now see.
// Callers hold s.mu: notifying under the lock is what stops a subscriber from
// ever observing two changes out of order. The channels are buffered and
// latest-wins, so this never blocks.
func (s *Store) notify() {
	for sub := range s.subs {
		latest(sub.ch, s.snapshot(sub.signedIn))
	}
}

// Rename changes a todo's text in place, for a visitor allowed to see it.
//
// It reports the same miss as Todo for a todo this visitor may not read, so a
// signed-out visitor gets one answer for `t3` whether they ask to read it or
// to write it. Guarding only the readers leaks twice over: the write lands,
// and the response hands back the record — text, id and Private — that the
// reader was refused.
func (s *Store) Rename(id, text string, signedIn bool) (Todo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.todos {
		if s.todos[i].ID == id && visible(s.todos[i], signedIn) {
			s.todos[i].Text = text
			// A rename can change what the live board says the newest todo is,
			// so it wakes subscribers exactly as an addition does.
			s.notify()
			return s.todos[i], true
		}
	}
	return Todo{}, false
}

// Watch subscribes to what this visitor may see, now and after every change.
// Call the returned function to unsubscribe.
//
// The visitor is fixed for the life of the subscription: the caller decides who
// is watching when the stream opens, and a subscription that outlives a change
// of identity has to be replaced rather than updated.
func (s *Store) Watch(signedIn bool) (<-chan Snapshot, func(), Snapshot) {
	s.mu.Lock()
	sub := &subscriber{ch: make(chan Snapshot, 1), signedIn: signedIn}
	s.subs[sub] = struct{}{}
	now := s.snapshot(signedIn)
	s.mu.Unlock()
	return sub.ch, func() {
		s.mu.Lock()
		delete(s.subs, sub)
		s.mu.Unlock()
	}, now
}

// SignIn opens a session and returns its id, which the caller stores in a
// cookie.
func (s *Store) SignIn(user string) string {
	id := newSessionID()
	s.mu.Lock()
	s.sessions[id] = user
	s.mu.Unlock()
	return id
}

// SignOut closes a session.
func (s *Store) SignOut(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}

// Session resolves a session id to the visitor it belongs to.
func (s *Store) Session(id string) Session {
	if id == "" {
		return Session{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return Session{User: s.sessions[id]}
}

func newSessionID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

// latest pushes v onto a latest-wins buffered channel without ever blocking.
func latest(ch chan Snapshot, v Snapshot) {
	select {
	case ch <- v:
	default:
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- v:
		default:
		}
	}
}
