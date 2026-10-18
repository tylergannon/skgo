// Package introduction is the sample on the Introduction page: a query and a
// command written in Go, declared for SvelteKit at the end of the file.
package introduction

import (
	"context"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/businesslogic"
	"github.com/tylergannon/skgo/example/businesslogic/visitor"
	"github.com/tylergannon/skgo/example/internal/skgo/params"
)

// getTodos lists this visitor's todos.
func getTodos(ctx context.Context) ([]businesslogic.Todo, error) {
	return visitor.Store(ctx).Todos(false), nil
}

// addTodo appends a todo to the list. A page that writes
// `addTodo(text).updates(getTodos())` gets the new list on the same response.
func addTodo(ctx context.Context, event params.RequestEvent, text string) (businesslogic.Todo, error) {
	if text == "" {
		return businesslogic.Todo{}, skgo.Errorf(400, "A todo needs some text")
	}
	return visitor.Store(ctx).Add(text), skgo.RefreshRequestedNoArg(ctx, getTodos)
}

var (
	_ = skgo.Query(getTodos)
	_ = skgo.Command(addTodo)
)
