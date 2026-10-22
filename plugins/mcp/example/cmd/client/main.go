// Command task-client composes generated CLI commands and MCP tools in one binary.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"unicode/utf8"

	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
	"github.com/tylergannon/skgo/plugins/mcp/example/contracts"
	client "github.com/tylergannon/skgo/plugins/mcp/example/internal/mcpclient"
	"github.com/tylergannon/skgo/plugins/mcp/remoteclient"
)

func main() {
	if err := newCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newCommand() *cobra.Command {
	var apiKey string
	c := &client.Client{Remote: remoteclient.Client{}}
	root := &cobra.Command{Use: "task-client", Short: "Work with the shared task list through CLI commands or MCP", SilenceUsage: true, SilenceErrors: true}
	// This callback belongs to the application. It runs immediately before
	// each HTTP request, for CLI and MCP calls alike.
	c.Remote.PrepareRequest = func(r *http.Request) error {
		key := apiKey
		if !root.PersistentFlags().Changed("api-key") {
			key = os.Getenv("SKGO_API_KEY")
		}
		if key != "" {
			r.Header.Set("X-API-Key", key)
		}
		return nil
	}
	root.PersistentFlags().StringVar(&c.Remote.BaseURL, "base-url", os.Getenv("SKGO_BASE_URL"), "Application base URL (or SKGO_BASE_URL)")
	root.PersistentFlags().StringVar(&apiKey, "api-key", "", "Application API key (or SKGO_API_KEY)")

	// Command layout and names are application choices, not generator policy.
	tasks := &cobra.Command{Use: "tasks", Short: "Read and add tasks"}
	list, add := client.NewListTasksCommand(c), client.NewAddTaskCommand(c)
	list.Use, add.Use = "list", "add"
	add.Example = "  task-client tasks add --title 'Ship CLI and MCP'\n  task-client tasks add --title @task.txt"
	tasks.AddCommand(list, add)

	// A custom command can use the same generated typed client. This one always
	// treats its argument as a path, and sends the file contents literally.
	tasks.AddCommand(&cobra.Command{Use: "add-file PATH", Short: "Add a task by reading a UTF-8 text file", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		data, err := os.ReadFile(args[0])
		if err != nil {
			return err
		}
		if !utf8.Valid(data) {
			return fmt.Errorf("file must contain UTF-8 text")
		}
		task, err := c.AddTask(cmd.Context(), contracts.NewTask{Title: string(data)})
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"result": task})
	}})
	root.AddCommand(tasks)
	root.AddCommand(&cobra.Command{Use: "mcp", Short: "Serve the same published operations as stdio MCP tools", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		s := server.NewMCPServer("TaskList", "dev", server.WithInstructions("The CLI, MCP tools and web page share the same task list."), server.WithToolCapabilities(false))
		s.AddTools(c.Tools()...)
		return server.NewStdioServer(s).Listen(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout())
	}})
	return root
}
