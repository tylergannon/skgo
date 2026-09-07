# Issue #66 — keyed form.for(key) submitted without JavaScript

correction: the issue's own scope note ("this may be only a lookup and
parsing fix, but that is for the implementer to map") was wrong. Fixing the
405 (`document_form.go`'s `runFormAction`/`Remotes.Lookup`) is necessary but
not sufficient. Kit's `form.for(key)` caches the keyed instance in
`state.remote.forms`, keyed by object identity plus `JSON.stringify(key)`, and
a fresh render's page component calling `sendMessage.for(key)` creates a brand
new instance unless something seeds `state.remote.forms` first. Without a
second fix in the SSR render engine's glue (`internal/adapter/skgo-adapter.js`,
`seed_form`), the Go handler runs and returns the right answer, but the
re-rendered page still shows nothing: the page's own `.for(key)` call resolves
to an empty instance, not the one Go just seeded. Confirmed this by tracing
kit's pinned source (`runtime/app/server/remote/form.js`'s `create_instance`
and its `for` property) before writing any code — mapping first, as the repo's
CLAUDE.md requires, is what caught this; the issue text alone would have led
to a fix that passes a naive "does the 405 go away" check but still renders
wrong.

decision: the fix threads one composite id string end to end —
`<hash>/<name>` or `<hash>/<name>/<key-json>` — matching kit's own
`${__.id}/${__.key}` convention for the `<global>.data.f` key. This same
string already had to be right for `document.go`'s existing `data.f` seeding
(hydration for kit's real client), so making `formAction.id` carry the
composite reused that existing contract instead of adding a second one. No
change was needed to `internal/ssr` (owned by #59/#62) or to `ssr.Form`'s
field set — the composite fits in the field that was already there.

friction: `internal/adapter/skgo-adapter.js`'s `SSR_ENTRY`/`SSR_APP_SERVER`/
`SSR_POLYFILL` blocks are JS source text held in a `String.raw` template
literal *inside a real .js file that itself gets parsed as JS* (by
Rolldown/Vite, to load `vite.config.ts`). A comment inside that block using
`${...}` is interpreted as a real template-literal interpolation by the OUTER
parser (not inert until the string is later extracted), and an *unpaired* or
non-trivial-content backtick inside it prematurely closes the outer template
literal, silently splitting the rest of the file into "real code" until the
next stray backtick. The failure mode is a cryptic Rolldown parse error
pointing at the comment text ("Expected a semicolon..."), nothing about
backticks or template literals. Convention in this file, confirmed by
counting backticks in the unedited original: **no backticks and no `${...}`
inside these three blocks' comments at all** — every apparent counter-example
on inspection turned out to be either outside the block or two isolated
single-word spans that happened to parse as valid standalone JS by accident.
Any future edit to comments inside these three `String.raw` blocks must avoid
both backtick and `${` entirely; plain prose or angle brackets
(`<hash>/<name>`) instead of code-span backticks.

friction: self-inflicted e2e failure the first time through — I started
`just serve` with a custom `SKGO_LOG` path (to isolate this task's server
logs from other concurrent work) but then ran `just e2e` without setting the
same `SKGO_LOG`, so the console-log-tailing scenario in `stream.feature`
looked for the probe line in the *default* `example/e2e/server.log` while the
server was actually writing to my custom path. Looked exactly like a
pre-existing regression (consistent failure, not flaky) until I checked both
files and found the probe line present in mine, absent in the default one.
Lesson: when isolating a port for a task, either leave `SKGO_LOG` at its
default (matching what `just e2e` expects) or pass the *same* override to
both `just serve` and `just e2e`.
