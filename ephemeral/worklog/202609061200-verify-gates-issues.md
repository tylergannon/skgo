# Landing the gates on sprint-003, and the six issues — what will change a future decision

## The two live-count fixes were not equivalent, and the difference is kit's protocol

Both branches found the leak and both wrote `Store.Watch(signedIn)`. The
divergence was how a live query learns the visitor changed:

- verify-gates called `watchCount().reconnect()` from the component, after the
  command resolved. A second round trip the server never asked for.
- sprint-003 named `watchCount` in the command's `updates()`, so the refresh
  travels in the command's own `l` map and the client reconnects in the same
  flight.

sprint-003's is kit's own mechanism and was proven on the wire. Keep in mind
when merging future duplicate fixes: `updates()` *does* carry a live query, as
the bare function — `categorize_updates` matches it by `QUERY_FUNCTION_ID` and
looks in `query_map` first, `live_query_map` second. The verify-gates worklog
says "`updates()` does not carry live queries"; that is wrong and is corrected
here.

## Authorization applied per function will be missed per function

`watchCount` (fixed in a723f77) and `Store.Rename` (#12) are the same defect
twice: the rule lived in each reader instead of in one place. sprint-003's
version of `store.go` had already factored `visible(todo, signedIn)`, which is
why #12 was two lines once the right side of the merge was kept. The third
instance will be whatever mutating path lands next. Issue #5 is the general
answer; until it lands, every new function that reaches a record by id is a
place to look.

Note what made #12 worse than a missed check: the command *returned the row*.
A refusal that still echoes the record is still the leak, which is why the
test asserts the response body separately from the status.

## A panic on a live-query producer ends the process, not the request

Query and command panics reset one connection — `curl` says "Empty reply from
server". The live-query producer runs on its own goroutine, where an
unrecovered panic is fatal to the whole binary; the test that proves it kills
the test process outright rather than failing. Any future goroutine started per
request needs the same guard. `http.ErrAbortHandler` must be re-panicked: it is
net/http's deliberate abort, not a fault.

## Kit answers an unexpected throw with HTTP 200

Verified in `runtime/server/remote-functions.js`: the catch returns
`Response.json({type:'error', error}, {status: state.prerendering ?
transformed.status : undefined})` — `undefined` at runtime, so 200. The client
reads the envelope on both the ok and non-ok paths, so a 500 would also work,
but 200 is what kit does and what skgo already did.

## A remote-function redirect loses its status, by design

`remote-functions.js` demotes a `Redirect` into the *success* envelope as a
bare `{redirect: location}` and drops `error.status`; the client `goto()`s it.
For a live query the frame is `{type:'redirect',location}` and the client
*fabricates* a 307 (`shared.svelte.js`) purely so its reconnect loop can
recognise the shape. So a status on the wire here would be an invention. #16
carries and validates the status on the value — kit's own 300-308 range, and
kit's header-safety check on the location — and does not transmit it.

## `__data.json` matches its own page route, and so does `__route.js`

Kit's route patterns end `\/?$`, so `/todos/__data.json` matches `/todos`. It
is not a one-segment problem: `/docs/a/b/__data.json` was swallowed by the rest
route too. Kit avoids this by recognising the suffix *before* routing
(`has_data_suffix`, top of `respond.js`) and carrying `is_data_request` through
every rendering decision.

Measured against this app's own `vp dev` server, which is kit answering
directly:

| URL | kit | skgo now |
| --- | --- | --- |
| `/todos/__data.json` (route matches) | 200 `application/json` `{"type":"data","nodes":[null,null]}` | 404 `application/json` App.Error |
| `/nope/__data.json` (no route) | 404 `text/html` | 404 `application/json` |
| `/todos/__route.js` | 400 `Server-side route resolution disabled` | identical |

The `__data.json` row is a knowing divergence, recorded in `serveDocument`'s
neighbourhood in `static.go`. Reproducing kit's envelope needs the node count
for each route's branch (`[...page.layouts, page.leaf]`), which the skgo
manifest does not carry and kit's public `RouteDefinition` does not expose —
`page` has only `methods`. It is in `builder.generateManifest()`'s text, which
the adapter already regexes for `remotes`, so it is reachable; it was not worth
inventing before server loads exist. Nothing observable changes today: the
built client contains no `__data.json` string at all, because kit fetches one
only for a node with a server load.

## polytype: relocating a foreign wire type needs a DEFINED type, not an alias

This is the fact the #14 fix turns on, and getting it wrong is easy because the
wrong answer generates output. It is measured, not read.

- `polytype.Declare` takes a method expression *or* a free function whose sole
  parameter is T. `examples/indirecttypes/schema.go` uses the free form.
- It refuses a directly-referenced foreign type: `undeclared local type found:
  HTTPError`, from `requestType` checking `LocalNamedTypes`
  (`internal/syntax/scan_result.go`). So the type must be named locally.
- **A local alias satisfies the scanner and then does not compile.** `type
  WireHTTPError = skgo.HTTPError` generates cleanly and produces correct
  TypeScript, so it looks right — but polytype emits its entrypoint as a
  *method on the receiver type even when the registration was a free
  function*, and `go build` fails with `cannot define new methods on non-local
  type WireHTTPError`. The free-function shape is kept only when
  `hasInvalidMethodReceiverBase` is true, i.e. only for pointer and interface
  underlying types — which is why `examples/indirecttypes` uses it, those are
  named *pointer* types. **Generate and then compile; generating is not the
  check.** This cost a wrong instruction to an implementer.
- **A defined type is the answer.** `type SkgoHTTPError skgo.HTTPError`
  compiles, and its TypeScript is identical to the alias form: polytype
  resolves through the definition and emits the foreign type under its own
  name, which is what the stubs import.
- Losing the method set across the definition does **not** lose safety.
  polytype does not honour a custom marshaller, it *refuses* the type —
  `rejectCustomWireType`: "defines MarshalJSON; custom JSON/text wire mappings
  are not statically derivable" — and it resolves through the defined type to
  find it. Verified: a defined type over a struct with `MarshalJSON` is
  refused, naming the underlying type. So relocation cannot smuggle a type
  whose Go encoding contradicts its declaration.
- The defined form is also strictly better for a foreign enum. An alias
  inherits the `enum()` marker without the constants that give it meaning
  (`declares enum() but has no typed constants`); the defined form projects
  `"on" | "off"` correctly.
- Name the local type *differently* from the foreign one. A same name makes
  polytype disambiguate and emit hash-suffixed identifiers
  (`HTTPError$6769746875622e...`), which breaks stub generation.
- Trap: polytype's generated `jsonschema_gen.go` carries `//go:build
  !jsonschema` while the registration file carries `//go:build jsonschema`.
  The type declarations have to live in an untagged file or the generated one
  refers to names that do not exist in an ordinary build. Build the generated
  app both ways in any test.

## Where a relocated declaration goes

`<Out>/wiretypes/<import path>`. Inside `Out` because that is already skgo's
disposable output and is documented to be in the same module as the remote
functions — which is what makes the directory compilable, nameable by Go, and
reachable by `go:embed`. Not under the route tree, whose own `go.mod` would put
it in a different module from the bindings. The directory is the **import
path**, not the package name, on both halves: two dependencies called `wire`
are ordinary, and import paths are the only names guaranteed to differ.

Residual and pre-existing, now reachable: separate directories do not help once
two types called `Thing` land in one `.remote.ts`, because TypeScript has one
namespace per module. That used to emit a module declaring `Thing` twice;
`writeStubs` now refuses it by name. Fixing it properly needs import aliasing
threaded through `project`.

## Ownership is a directory question, not a module-path question

skgo writes a `go.mod` boundary into `web/src/routes` — the thing that stops
`go build ./...` walking into `[id]` — so a route package the developer
authored reports a module path that is not the app's. An implementation that
compares module paths refuses `pricing.Plan`, the developer's own type. Compare
containment, and `EvalSymlinks` both sides: `go list -f {{.Dir}}` returns
resolved paths while the app root may not be (`/var` vs `/private/var`).

Also: the damage lands before polytype runs. `writePolytypeMarkers` writes the
Go file into the foreign package unconditionally; both pre-fix failures
reproduced with polytype not installed at all.

## Process

- The e2e store is in-memory and shared with anything else touching :8080. A
  `curl` probe against the running binary mutated `t1` and made the next suite
  run fail on a seeded todo. Restart the server between wire probes and suite
  runs, or probe a throwaway todo.
- A hand-written remote payload is base64url of the devalue JSON, not the JSON.
  `["t3"]` must be sent as `WyJ0MyJd`. Sending the JSON gets a 400 that reads
  like a server bug.
- The root module's tests alone are not the gate. Changing a status in
  `static.go` left `example/server_test.go` asserting the old one, and a run of
  `go test ./...` from the root said nothing. Run both modules.
- Deleting a remote function makes the *next* `skgo generate` fail at
  `packages.Load`, because the stale `skgo_remotes_gen.go` still references it.
  The generator cannot recover from its own previous output. Pre-existing,
  found while testing #14, not filed.
