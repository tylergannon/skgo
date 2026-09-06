# Porting the junkyard app onto skgo — actionable intelligence

Full findings in `ephemeral/survey/gap-report.md`. What follows is only the
things that will change a future decision.

trap: `skgo generate` writes generated files into **the skgo library's own module
root** when a downstream app's remote function mentions a skgo type. Porting the
guestbook produced `skgo_polytype_gen.go`, `jsonschema_gen.go` and `jsonschema/`
at the repository root, because `signOut` returns `skgo.None` and polytype had to
project it. `example/` never hits this only because none of its functions return
`skgo.None`. For a real consumer skgo is in the read-only module cache and this
fails outright. Whoever touches `internal/gen/types.go` next should decide where
projections of library-owned types belong; the answer is not "beside the type".

discovery: **`__data.json` is orthogonal to `ssr`.** Grep `ssr` across kit's
`runtime/server/data/index.js`, `runtime/server/page/data_serializer.js` and
`runtime/server/page/load_data.js` — nothing. Turning SSR off removes the
hydrate payload and the HTML-embedded data, not the data endpoint. Under
`ssr = false` kit's client fetches `__data.json` on *every* page view including
the first, so a CSR-only server has **one** wire format to implement (NDJSON,
`devalue.unflatten` on the client) instead of two, and never touches
`devalue.uneval`. Server loads are the *easiest* case under CSR, not the hardest.

discovery: the switch that turns the whole data path on is
`__SVELTEKIT_HAS_SERVER_LOAD__`, a Vite define (`exports/vite/index.js:1250-1258`)
computed from whether any *built* `+*.server.js` **exports** a `load`
(`utils/routing.js:344-346`, `core/postbuild/analyse.js:84-87`). Kit never calls
it. A generated throwing stub — the same shape skgo already emits for remote
functions — is enough. Verified experimentally: with no `+page.server.ts` the
string `__data.json` is absent from the client bundle entirely; adding one
throwing stub put it in the bundle and produced
`GET /serverload/__data.json?x-sveltekit-invalidated=01` on first paint. If that
bit ever comes out `false`, kit silently stops asking Go for anything and the
failure looks like "loads don't work" rather than "the stub wasn't built".

correction (fix before implementing loads, not after): skgo currently answers
`/<path>/__data.json` with the **SPA HTML document**. `matchesRoute` uses kit's
own regexes, which end `\/?$`, so a one-segment data URL matches its own page
route: `GET /messages/__data.json` returns **200 `text/html`**, which kit's client
`JSON.parse`s. Deeper paths get 404 + the same HTML. The data suffix has to be
recognised in `static.go` regardless of whether loads exist yet.

trap: **skgo does not recover panics.** A panicking remote function escapes to
`net/http`'s per-connection handler: `curl` gets "Empty reply from server" — no
status line, no body — and the stack trace (with whatever the panic carried) goes
to the process log. The browser renders `500 / Internal Error` only because kit's
client falls back to that when a fetch fails, so it *looks* handled in a
screenshot and in a scenario.

discovery: a universal `+page.ts` that awaits a Go remote function is a working
substitute for a server load's **error and redirect** behaviour, though not for
data composition, cookies or invalidation. `skgo.Errorf(418, …)` from a query
awaited in a load renders `+error.svelte` with status 418 in the page;
`skgo.Errorf(404, …)` likewise; `skgo.Redirect` navigates. Only the outer HTTP
document status stays 200, which is inherent to CSR. Worth knowing before
designing loads: the error/redirect half of the contract already has a path.

trap: the workaround for a cookie-writing layout load — a `command` fired from
the component — is **not** merely inelegant, it is wrong. In an `$effect` the
single-flight refresh re-runs the effect and the counter runs away (measured 7
after two page loads). In `onMount` it still races the sibling query: the query
reads the pre-increment cookie and can land last. Measured display `1,1,3,4`
against a cookie of `1,2,3,4`, and the scenario is flaky 3-of-5. Do not
recommend this shape to anyone.

discovery: `internal/remotearg` models a JavaScript `File` faithfully, but
`decodeArg` marshals the parsed devalue tree through `encoding/json` into the Go
argument type (`remote.go:183-198`), which destroys it, and the type projector
refuses the shape anyway. So "add a `form` remote kind" is not sufficient for
uploads — the argument path needs a non-JSON escape hatch first.

correction: `skgo.Redirect` has no `Status` field. Kit's `redirect(307, …)` does
not port; the compiler catches it. Invisible under CSR (kit's client always
re-navigates with a GET) but it is a gratuitous divergence and kit's own wire
format carries the status.

friction: the survey app lives at `ephemeral/survey/guestbook/` and is in
`go.work`. Its `web/build/` and `e2e/test-results/` are gitignored. It runs on
`127.0.0.1:8090` so it does not collide with the example on 8080.
