// Package example holds the server-side half of the example app: the remote
// functions its SvelteKit frontend calls, and the in-memory store behind them.
package example

import (
	"context"
	"fmt"
	"sync"

	"github.com/tylergannon/skgo"
)

// todosModule is the vite-root-relative path of the module whose exports these
// functions implement. Kit hashes exactly this string to build the request URL.
const todosModule = "src/lib/todos.remote.ts"

// Todo is one item. The json tags are the wire contract with the frontend.
type Todo struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// Store is the app's entire state: a list of todos and the set of live-query
// subscribers waiting to hear that it changed.
type Store struct {
	mu    sync.Mutex
	todos []Todo
	next  int
	subs  map[chan int]struct{}
}

func NewStore() *Store {
	return &Store{
		todos: []Todo{
			{ID: "t1", Text: "write the adapter"},
			{ID: "t2", Text: "serve remote functions"},
		},
		next: 3,
		subs: map[chan int]struct{}{},
	}
}

// Remotes is the store's remote-function surface, ready to hand to
// skgo.NewRemotes.
func (s *Store) Remotes() []*skgo.Remote {
	return []*skgo.Remote{
		skgo.Query(todosModule, "getTodos", s.GetTodos),
		skgo.Query(todosModule, "getTodo", s.GetTodo),
		skgo.Command(todosModule, "addTodo", s.AddTodo),
		skgo.LiveQuery(todosModule, "watchCount", s.WatchCount),
	}
}

func (s *Store) GetTodos(ctx context.Context, _ skgo.None) ([]Todo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Todo, len(s.todos))
	copy(out, s.todos)
	return out, nil
}

func (s *Store) GetTodo(ctx context.Context, id string) (Todo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, todo := range s.todos {
		if todo.ID == id {
			return todo, nil
		}
	}
	return Todo{}, skgo.Errorf(404, "No todo with id %q", id)
}

func (s *Store) AddTodo(ctx context.Context, text string) (Todo, error) {
	s.mu.Lock()
	todo := Todo{ID: fmt.Sprintf("t%d", s.next), Text: text}
	s.next++
	s.todos = append(s.todos, todo)
	count := len(s.todos)
	// Notify while still holding the lock so subscribers can never observe
	// counts out of order. The channels are buffered and latest-wins, so this
	// never blocks.
	for ch := range s.subs {
		select {
		case ch <- count:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- count:
			default:
			}
		}
	}
	s.mu.Unlock()
	return todo, nil
}

// WatchCount pushes the number of todos, now and after every addition, until
// the client disconnects.
func (s *Store) WatchCount(ctx context.Context, _ skgo.None, yield func(int) error) error {
	s.mu.Lock()
	ch := make(chan int, 1)
	s.subs[ch] = struct{}{}
	count := len(s.todos)
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
	}()

	if err := yield(count); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case n := <-ch:
			if err := yield(n); err != nil {
				return err
			}
		}
	}
}
