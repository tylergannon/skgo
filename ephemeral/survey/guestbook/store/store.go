// Package store is the Go translation of the junkyard guestbook's
// src/lib/server/store.ts: an in-memory message list with a change fan-out.
package store

import (
	"sync"
	"time"
)

// Attachment mirrors the { name, size } shape the app renders.
type Attachment struct {
	Name string `json:"name"`
	Size int    `json:"size"`
}

// Message is one guestbook entry.
type Message struct {
	ID         int         `json:"id"`
	Author     string      `json:"author"`
	Text       string      `json:"text"`
	Attachment *Attachment `json:"attachment"`
	PostedAt   string      `json:"postedAt"`
}

// Store holds the messages and wakes watchers when one is added.
type Store struct {
	mu       sync.Mutex
	messages []Message
	watchers map[chan int]struct{}
}

// Default is the process-wide store, matching the module-level singleton the
// TypeScript app used.
var Default = New()

// New builds a store seeded with the same first message the junkyard app had.
func New() *Store {
	return &Store{
		messages: []Message{{
			ID:       1,
			Author:   "kit",
			Text:     "welcome to the guestbook",
			PostedAt: "2026-01-01T00:00:00Z",
		}},
		watchers: map[chan int]struct{}{},
	}
}

// List returns the messages newest-first.
func (s *Store) List() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Message, 0, len(s.messages))
	for i := len(s.messages) - 1; i >= 0; i-- {
		out = append(out, s.messages[i])
	}
	return out
}

// Get looks one message up by id.
func (s *Store) Get(id int) (Message, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.messages {
		if m.ID == id {
			return m, true
		}
	}
	return Message{}, false
}

// Add appends a message and wakes every watcher.
func (s *Store) Add(author, text string, attachment *Attachment) Message {
	s.mu.Lock()
	m := Message{
		ID:         len(s.messages) + 1,
		Author:     author,
		Text:       text,
		Attachment: attachment,
		PostedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	s.messages = append(s.messages, m)
	count := len(s.messages)
	watchers := make([]chan int, 0, len(s.watchers))
	for w := range s.watchers {
		watchers = append(watchers, w)
	}
	s.mu.Unlock()

	for _, w := range watchers {
		select {
		case w <- count:
		default:
		}
	}
	return m
}

// Count is the number of messages.
func (s *Store) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.messages)
}

// Watch returns a channel carrying the count after every change, a function
// that stops the subscription, and the count right now.
func (s *Store) Watch() (<-chan int, func(), int) {
	ch := make(chan int, 8)
	s.mu.Lock()
	s.watchers[ch] = struct{}{}
	count := len(s.messages)
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		delete(s.watchers, ch)
		s.mu.Unlock()
	}, count
}
