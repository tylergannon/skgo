# Task list: generated CLI and MCP

This is a working SKGo application, with one task list shared by the real Kit
web frontend, generated Cobra commands and generated MCP tools. `taskCount`
is deliberately web-only. State resets when the application exits.

Build the real generator plugin, generate its outputs, and build the application
and client from this directory:

```sh
export GOWORK=off
go build -buildmode=plugin -o .local/mcp.so github.com/tylergannon/skgo/plugins/mcp
go run ./cmd/generate -plugin "$PWD/.local/mcp.so"
go tool skgo build --root .
go build -o .local/task-list ./cmd
go build -o .local/task-client ./cmd/client
.local/task-list
```

The build installs the pinned frontend through VitePlus. Open
[the task list](http://127.0.0.1:8080), then use another terminal:

```sh
export SKGO_ORIGIN=http://127.0.0.1:8080
.local/task-client --help
.local/task-client tasks add --help
.local/task-client tasks list
.local/task-client tasks add --title 'Ship the CLI'
.local/task-client tasks add --title @task.txt
.local/task-client tasks add --title @@owner
.local/task-client tasks add --input-json '{"title":"@literal-json"}'
.local/task-client tasks add-file task.txt
.local/task-client tasks list
```

Create `task.txt` containing any nonblank UTF-8 task title up to 120 characters;
newlines count and are preserved. The at-sign convention is one pass: `@task.txt`
reads the file and `@@owner` sends `@owner`. A missing file is an error, with an
escape hint, and creates no task. JSON input and MCP strings remain literal.
`add-file` is an application-written command using the same generated typed
client; it demonstrates replacing generated argument handling.

Refresh the web list to see CLI-created tasks. Add a task in the web page, then
list through the CLI to see the same change. Empty/overlong titles are rejected
by the application; wrong input types are rejected by the generated decoder.
Failures exit nonzero; successful output is JSON with a `result` property.

## MCP from the same binary

Configure an MCP host to launch the absolute path to `.local/task-client` with
arguments `--origin http://127.0.0.1:8080 mcp`. It exposes `listTasks` and `addTask`
with their Go documentation and schemas, over stdio. CLI commands and MCP tools
share the generated client. The previous standalone entrypoint is also retained:

```sh
go build -o .local/task-mcp ./cmd/mcp
.local/task-mcp -origin http://127.0.0.1:8080
```

## Application-owned setup and authentication

`cmd/client/main.go` chooses the `tasks` hierarchy, renames the generated
commands and registers MCP. The generator makes none of these layout decisions.
Its outgoing-request callback adds an API key for either interface. To exercise
it, restart the task-list server with `SKGO_API_KEY` set to a local test key,
and set the same environment variable for `task-client`. A wrong or absent key
gets HTTP 401. The key is not printed in help. The browser uses HTTP Basic auth
with any username and the same key as its password in this local example.

Regeneration updates `internal/mcpclient` while preserving application-owned
entrypoints. `go run ./cmd/generate -plugin "$PWD/.local/mcp.so" -init` continues
to create missing original MCP setup without replacing authored files.

## Tests

From the plugin directory, `GOWORK=off go test ./...` covers generation, argument
handling and HTTP request preparation. From this example, `GOWORK=off go test
./...` checks generated CLI and MCP calls against the real task handlers,
including authentication, file contents, escapes, malformed input and shared
state. Running and using the built application remains part of verification.
