# CLI Generator plugin

Generates typed Go clients, Cobra commands and MCP tools for selected SKGo
queries and commands. Go documentation comments supply help text and tool
descriptions. Both interfaces call your application's existing HTTP endpoints.

## Use

Install a plugin artifact built for your exact SKGo release:

```sh
skgo plugin install --prebuilt ARTIFACT --sha256 DIGEST
```

Add `github.com/tylergannon/skgo/plugins/cli` to your application's Go dependencies
at the matching plugin version. Mark each operation you want to expose beside
its existing declaration in a `*.remote.go` file:

```go
import "github.com/tylergannon/skgo/plugins/cli/clipublish"

var _ = skgo.Query(listTasks)
var _ = clipublish.Tool(listTasks)
```

Run `skgo generate --plugins` with your usual generation flags. Output goes in
`internal/cliclient/`; unmarked operations are excluded.

Construct a generated client with your application's base URL, then attach its
commands to your Cobra command tree:

```go
c := &cliclient.Client{Remote: remoteclient.Client{BaseURL: baseURL}}
root.AddCommand(cliclient.NewListTasksCommand(c))
```

Your application chooses command names, shared flags and authentication. Set
`c.Remote.PrepareRequest` to add authentication headers. For MCP, register
`c.Tools()` with your MCP server; `skgo add mcp-server` supplies a starter.

Generated commands provide `--help`, field flags and `--input-json`. String flags
accept `@path` for file contents and `@@text` for a literal `@text`. Successful
calls print JSON. MCP arguments remain literal strings.

See the [working task-list example](example/) for build instructions, CLI usage,
authentication, custom commands and MCP setup.
