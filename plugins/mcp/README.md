# MCP generator plugin

The MCP plugin generates typed Go clients and a standalone stdio MCP server
for explicitly selected SKGO queries and commands. Function Go comments become
tool descriptions. Field and type comments become JSON Schema documentation.
Generated clients import the application's contract types and use Polytype's
strict devalue codecs. The runtime calls the existing application's HTTP
endpoints. It does not load the generator plugin or application handlers.

The first version supports ordinary queries and commands, exported importable
contract types, scalars, structs, slices, fixed arrays, enums, time strings,
and Optional/Nullable fields supported by Polytype. Unsupported selections,
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
`skgo plugin install --prebuilt ARTIFACT --sha256 DIGEST --select MODULE#EXPORT`,
then `skgo generate --plugins` with their normal generation flags. The core
release must include the `RemoteOperation.Description` field. Artifacts must be
built for that exact official CLI through `skgo-plugin-build`; this proposal
does not publish a release artifact.

Generated output lives in `internal/mcpclient/` and `cmd/mcp/main.go`.
SKGO owns publication, stale-file cleanup and protection of authored edits.
An empty selection is an error. New application remotes are never automatically
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
