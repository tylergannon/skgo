# Streaming on both wires, nested promises, and the engine's missing globals (#57)

## "A Deferred may only be a top-level field" was never kit's rule

`deferred.go` refused a Deferred below the top level of a load's result and
attributed the rule to kit's client. Kit has no such rule. Its serializer finds
a promise with a devalue *reducer* (`Promise` in `server_data_serializer_json`,
the `thing?.then` arm of `get_replacer` in `server_data_serializer`) and its
client reads one back with a devalue *reviver*
(`process_stream`: `devalue.unflatten(data, { ...app.decoders, Promise })`).
A reducer and a reviver both walk the whole tree, so a promise is found wherever
the load left one, at any depth.

The same fact makes a promise *inside a promised value* work: each chunk is
serialized with the same reducers that wrote the first one, so a nested promise
takes a new id and becomes another chunk on the same response. Kit's own
`create_async_iterator` says so in a comment — "`deferred` can grow while we
iterate, as resolved values may add further promises".

Consequence for skgo: the load result no longer goes through `encoding/json` and
then gets its top-level fields patched back. It takes the same walk a
transported value takes (`treeEncoder{promises: true}`), because a Deferred and
a transported value need the identical treatment — survive as the Go value the
reducer is watching for. The engine's node data lost its `deferred: []string`
side channel and now carries kit's own `["Promise", <id>]` placeholder, read
back in the bundle with a `Promise` reviver. Nothing on either side agrees on a
list of names any more.

## goja is missing three globals, and the console one is invisible

`Symbol.asyncIterator`, `Promise.withResolvers` and `console` are all absent
(measured, not assumed). So is async-generator *syntax* — `(async function*(){})`
is a `SyntaxError` at parse time — which means kit's `create_async_iterator`
could never run in the engine even with `withResolvers`. It does not have to:
Go does the streaming, and only the document/`__data.json` writers need the
ordering that helper implements.

`console` is the one that was doing damage. Svelte's `unresolved_hydratable`
calls `console.warn` unconditionally in a *production* build, and kit's
`log_handle_error_hook_failure` calls `console.error`; with no such global each
of those is a ReferenceError thrown mid-render. Measured on the example app's
`/console` page with the globals disabled: **HTTP 500, the error boundary on
screen, and not one line in the server's log.** That is the failure mode — not a
missing message, a silent one.

They are installed in Go, in `newRuntime`, *before* the bundle is evaluated, so
a bundle that cannot come up can say why.

## Under pnpm, `devalue` does not resolve from the app

The bundle's entry now needs devalue directly (kit's `parse` has no `Promise`
decoder). `example/web/node_modules/devalue` does not exist — pnpm's strict
layout puts it under kit's own real directory — so it is aliased explicitly:
`createRequire(join(kit, 'package.json')).resolve('devalue')`, where `kit` is
already the realpath'd copy. Resolving it from the app's cwd fails, and the
failure is an esbuild "could not resolve" during `vp build`, hundreds of lines
into a log.

The backtick trap in `SSR_ENTRY` (see 202609071300) bit again, from a JSDoc
comment mentioning `process_stream` in backticks. It reads as
`[PARSE_ERROR] Expected a semicolon` against the *vite config*, from
`internal/newapp`'s scaffold test. No backticks, no `${` in those literals.

## A streamed `__data.json` body cannot be read out of the browser at all

`response.text()` on the `__data.json` response fails with
`Protocol error (Network.getResponseBody): No data found for resource`. Reading
it from inside the `page.on('response')` handler instead of from the Then step
does *not* help — that was tried and failed identically. Chrome does not keep
the body of a streamed fetch the page has consumed itself, and kit's client
consumes this one with its own reader. A document response is fine; only this
kind of subresource is not.

So the two claims about a client-side navigation are split. What the browser
does with the response is asserted in the DOM — the ticker fills in while the
digest and the forecast are still loading, which is precisely the settlement
order — and what is *in* the response is asserted by asking for the same URL
again through Playwright's `request` fixture. Mutating `writeNodes` back to id
order was checked against this: the DOM step fails, at the right step, with a
readable message.

## The suite can now read the server's log, and one claim needs it

`just serve` pipes the process through `tee` into `example/e2e/server.log`
(gitignored, path overridable with `SKGO_LOG`, which `just e2e` forwards). One
scenario is about a line only an operator ever sees — what the engine wrote to
the console — and a browser has no way to observe it.

The step notes the log's *byte offset* before navigating and only reads what was
appended. The fixture writes the same sentence on every visit, so "the log
contains it" would have been satisfied by a line from an earlier run.

## `just dev` binds IPv6 localhost, and `-proxy http://127.0.0.1:5173` cannot reach it

The recipe is a bare `vp dev`, so vite listens on `[::1]:5173` only. The Go
server's proxy dials the address it is given, and the dev leg comes up as a 502
saying `connection refused` on a port `lsof` clearly shows is listening. Point
the proxy at `http://localhost:5173` (or start vite the way the Procfile does,
`--host 127.0.0.1`). The dev leg is `just dev` alongside
`go run ./example/cmd -listen 127.0.0.1:8080 -proxy http://localhost:5173`;
there is no recipe for the second half.

## The example server's store outlives a run, and one scenario counted rows

`businesslogic.Default` is process state. `routes.feature`'s POST scenario
asserted that exactly one row said "walk to the harbour", so a second suite run
against the same server went red while nothing was wrong: the endpoint has no
uniqueness rule and correctly created a second record.

Fixed at the source rather than by remembering to restart: the `/api` page now
shows the id out of the 201's own body, and the scenario looks for *that
record* in the list rather than counting rows with that text. Every other
scenario that adds a todo already asserted something id- or count-delta-shaped
and was fine.
