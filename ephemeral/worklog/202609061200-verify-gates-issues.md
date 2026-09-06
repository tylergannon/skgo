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

## polytype requires the declared type to be local — but an alias counts

This is the fact the #14 design turns on, and it is measured, not read.

- `polytype.Declare` takes a method expression *or* a free function whose sole
  parameter is T. `examples/indirecttypes/schema.go` uses the free form for
  named pointer types, where a method is impossible.
- It still refuses a directly-referenced foreign type:
  `undeclared local type found: HTTPError`, from `requestType` checking
  `LocalNamedTypes` (`internal/syntax/scan_result.go`).
- A local **alias** satisfies it. `type WireHTTPError = skgo.HTTPError` in the
  target package, plus a free function over the alias, generates cleanly into
  the app's own directory. An alias is the same type, so no method set and no
  custom `MarshalJSON` is lost — a defined type (`type X wire.Thing`) would
  silently drop one and the schema would then lie.
- Name the alias *differently* from the foreign type. A same-name alias makes
  polytype disambiguate and emit hash-suffixed identifiers
  (`HTTPError$6769746875622e...`), which breaks stub generation. A distinct
  name emits `export type HTTPError = {...}` under the real name plus an alias
  line, so stubs keep referring to the foreign type by its own name.
- Trap: polytype's generated `jsonschema_gen.go` carries `//go:build
  !jsonschema` while the declaration file carries `//go:build jsonschema`. An
  alias that exists only in the tagged file leaves the generated file
  referring to a name that does not exist in an ordinary build.

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
