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
	// subs maps a live-count subscriber to the visibility it subscribed with,
	// because the number a visitor may be told is not the same number for
	// everyone.
	subs map[chan int]bool
}

// Default is the store the example app serves.
var Default = NewStore()

func NewStore() *Store {
	return &Store{
		todos: []Todo{
			{ID: "t1", Text: "write the adapter"},
			{ID: "t2", Text: "serve remote functions"},
			{ID: "t3", Text: "ship the private roadmap", Private: true},
		},
		next:     4,
		sessions: map[string]string{},
		subs:     map[chan int]bool{},
	}
}

// Todos returns what a visitor may see. A signed-out visitor never learns that
// the private todos exist.
func (s *Store) Todos(signedIn bool) []Todo {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Todo, 0, len(s.todos))
	for _, todo := range s.todos {
		if todo.Private && !signedIn {
			continue
		}
		out = append(out, todo)
	}
	return out
}

// Todo looks one up, applying the same visibility rule.
func (s *Store) Todo(id string, signedIn bool) (Todo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, todo := range s.todos {
		if todo.ID == id && (signedIn || !todo.Private) {
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
	// Notify while still holding the lock so subscribers can never observe
	// counts out of order. The channels are buffered and latest-wins, so this
	// never blocks.
	for ch, signedIn := range s.subs {
		latest(ch, s.countLocked(signedIn))
	}
	s.mu.Unlock()
	return todo
}

// countLocked is how many todos a visitor may see. The caller holds s.mu.
func (s *Store) countLocked(signedIn bool) int {
	n := 0
	for _, todo := range s.todos {
		if todo.Private && !signedIn {
			continue
		}
		n++
	}
	return n
}

// Rename changes a todo's text in place.
func (s *Store) Rename(id, text string) (Todo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.todos {
		if s.todos[i].ID == id {
			s.todos[i].Text = text
			return s.todos[i], true
		}
	}
	return Todo{}, false
}

// Watch subscribes to the number of todos this visitor may see. Call the
// returned function to unsubscribe.
//
// The count obeys the same visibility rule as Todos. A total would tell a
// signed-out visitor that a todo they cannot read exists, which is the whole
// thing Private is for.
func (s *Store) Watch(signedIn bool) (<-chan int, func(), int) {
	s.mu.Lock()
	ch := make(chan int, 1)
	s.subs[ch] = signedIn
	count := s.countLocked(signedIn)
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
	}, count
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
func latest(ch chan int, v int) {
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
