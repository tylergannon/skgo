package tasks

import (
	"context"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/plugins/mcp/example/contracts"
	"github.com/tylergannon/skgo/plugins/mcp/example/internal/skgo/params"
	"github.com/tylergannon/skgo/plugins/mcp/mcppublish"
)

var mu sync.Mutex
var tasks = []contracts.Task{}

// listTasks returns every task in creation order. The web page shares this list.
func listTasks(context.Context) ([]contracts.Task, error) {
	mu.Lock()
	defer mu.Unlock()
	return append([]contracts.Task{}, tasks...), nil
}

// addTask creates a task in the shared list and returns its ID and title.
func addTask(_ context.Context, _ params.RequestEvent, input contracts.NewTask) (contracts.Task, error) {
	title := strings.TrimSpace(input.Title)
	if title == "" || utf8.RuneCountInString(title) > 120 {
		return contracts.Task{}, &skgo.HTTPError{Status: 400, Message: "Supply a task title of 1 to 120 characters"}
	}
	mu.Lock()
	defer mu.Unlock()
	task := contracts.Task{ID: len(tasks) + 1, Title: title}
	tasks = append(tasks, task)
	return task, nil
}

var _ = skgo.Query(listTasks)
var _ = skgo.Command(addTask)

var _ = mcppublish.Tool(listTasks)
var _ = mcppublish.Tool(addTask)

// taskCount is a web remote, deliberately not published as an MCP tool.
func taskCount(context.Context) (int, error) {
	mu.Lock()
	defer mu.Unlock()
	return len(tasks), nil
}

var _ = skgo.Query(taskCount)
