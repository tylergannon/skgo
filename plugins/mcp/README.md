# MCP generator plugin

The MCP plugin generates typed Go clients and a standalone stdio MCP server
for explicitly marked SKGO queries and commands. Function Go comments become
tool descriptions. Field and type comments become JSON Schema documentation.
Generated clients import the application's contract types and use Polytype's
strict devalue codecs. The runtime calls the existing application's HTTP
endpoints. It does not load the generator plugin or application handlers.

The first version supports ordinary queries and commands, exported importable
contract types, scalars, structs, slices, fixed arrays, enums, time strings,
and Optional/Nullable fields supported by Polytype. Unsupported published remotes,
custom transports, unions and supplied-schema fields fail generation. Integer
tool values must fit JavaScript's safe-integer range. Use strings for large IDs.
Scalar/array tool inputs use an `argument` property. Successful results use a
`result` property. Tool errors preserve public application messages. Commands
are sent once and errors do not imply rollback.

## Example application

[`example/`](example/) is a task list served by Go and SvelteKit. The generated
`listTasks` and `addTask` tools share the web page's state. State resets when
the application exits. Its loopback example has no login. The generated MCP
entrypoint accepts `SKGO_TOKEN` for applications that already support bearer
credentials, and `-base`/`-app-dir` for nondefault Kit paths.

From `plugins/mcp/example`, with the repository's Go toolchain and VitePlus:

```sh
export GOWORK=off
mkdir -p .local
go build -buildmode=plugin -o .local/mcp.so github.com/tylergannon/skgo/plugins/mcp
go run ./cmd/generate -plugin "$PWD/.local/mcp.so"
cd web
vp install --frozen-lockfile
cd ..
go tool skgo build --root .
CGO_ENABLED=0 go build -o .local/task-list ./cmd
CGO_ENABLED=0 go build -o .local/task-mcp ./cmd/mcp
.local/task-list
```

Open `http://127.0.0.1:8080`. Configure an MCP host to launch the absolute path
to `.local/task-mcp` with `-origin http://127.0.0.1:8080`. Adding a task from
either interface makes it available to the other. Refresh the page's list
after a tool call.

The example's small generation host loads the real shared object using the
same Go module graph. This is checkout development, not an official-release
plugin pairing. Released consumers use the existing exact-host installation:
`skgo plugin install --prebuilt ARTIFACT --sha256 DIGEST`,
then `skgo generate --plugins` with their normal generation flags. The core
release must include the `RemoteOperation.Description` field and the
`MarkedGenerator` contract. Artifacts must be
built for that exact official CLI through `skgo-plugin-build`; this proposal
does not publish a release artifact.

## Publication and server setup

Put publication markers beside the existing remote declarations in the same
`*.remote.go` file. Function names can stay unexported:

```go
import "github.com/tylergannon/skgo/plugins/mcp/mcppublish"

var _ = skgo.Query(listTasks)
var _ = mcppublish.Tool(listTasks)
```

The marker is the MCP allowlist. Do not configure `--select` entries for this
plugin. Unmarked web remotes are excluded before projecting their contracts.
Published names use one flat namespace, with no aliases or disambiguation.
Two published remotes with the same name produce conflicting Go declarations
and the MCP build fails.

The plugin supplies the `mcp` template and `mcp-server` add-on. With the plugin
installed, `skgo add mcp-server` creates `cmd/mcp/main.go.tmpl` and
`cmd/mcp/instructions.md`. The files belong to your application immediately;
applying the add-on again preserves authored contents. The `mcp` template
applies the same add-on to the normal minimal application scaffold.

Maintain `main.go` to configure server name/version, connection flags,
credentials, headers, your `http.Client`, and additional tools/resources/prompts.
The skeleton supports `SKGO_ORIGIN`, `SKGO_TOKEN` (bearer) and `SKGO_API_KEY`
(`X-API-Key`). These must match authentication implemented by your application.
The skeleton is initially inactive (`main.go.tmpl`), keeping normal web builds,
development and `go mod tidy` working before MCP setup. Add the runtime dependency
with `go get github.com/tylergannon/skgo/plugins/mcp@VERSION`, matching your
installed plugin release; mark remotes and generate. Activate once with
`mv cmd/mcp/main.go.tmpl cmd/mcp/main.go`, then `go mod tidy` and
`go build -o task-mcp ./cmd/mcp`. After activation, `main.go` belongs to your
application. Reapplying the add-on preserves either pending or active setup.
The checkout example already supplies its dependencies and activated entrypoint.

Edit `instructions.md` for application-wide guidance; it is embedded and sent
in MCP's initialize response. Rebuild the binary after changing it. Tool and
schema documentation continue to come directly from Go comments.

Recurring `skgo generate --plugins` with your usual generation flags updates
only `internal/mcpclient/`. Register the marked tools with `s.AddTools(c.Tools()...)`.
SKGO owns publication, stale-file cleanup and protection of authored edits.

Generated output lives in `internal/mcpclient/`.
SKGO owns publication, stale-file cleanup and protection of authored edits.
No marked remotes is an error. New application remotes are never automatically
exposed. Move request/result declarations into a shared contract package when
their route package cannot be imported from generated client code.

## Checks

`GOWORK=off go test ./...` in this plugin module runs unit tests with literal
models, JSON values and an injected HTTP transport. Mission Control owns the
application validation at `skgo-project/scripts/mcp/run.sh`, also exposed as the
**Validate MCP plugin** Actions workflow there. It builds this example, launches
both binaries, discovers tools and documentation, and observes changes through
MCP and the real Kit web frontend in both directions. Core release qualification
and native-shell qualification remain separate.
