// Package contracts defines the task list's shared Go request and result types.
package contracts

// NewTask is the input for adding a task.
type NewTask struct {
	// Title is the task to do. Supply 1 to 120 characters of nonblank text.
	Title string `json:"title"`
}

// Task is one item in the shared task list.
type Task struct {
	// ID identifies this task for the lifetime of the application process.
	ID int `json:"id"`
	// Title describes the work to do.
	Title string `json:"title"`
}
