# What an independent app asks skgo for, and what it gets

The app is `ephemeral/inspiration/junkyard/app/` — a SvelteKit 3 guestbook written
against `adapter-node`, with its own Playwright suite. Nobody designed skgo to
pass it.

It was ported route-for-route onto skgo in `ephemeral/survey/guestbook/`: a real
SvelteKit app with the skgo adapter, Go remote functions, a Go message store, and
the junkyard's own scenarios re-pointed at the Go server. It runs on
`127.0.0.1:8090`. Every scenario screenshots the page at the moment it asserts,
into `ephemeral/survey/guestbook/e2e/shots/`. **Twenty-two scenarios: ten pass,
twelve fail.**

Reproduce:

```
cd ephemeral/survey/guestbook/web && pnpm install
ORIGIN=http://127.0.0.1:8090 vp build
cd .. && go generate ./generated/... && go build -o /tmp/guestbook ./cmd
/tmp/guestbook --listen 127.0.0.1:8090
cd e2e && pnpm install && BASE_URL=http://127.0.0.1:8090 playwright test
```

Kit citations below are against the pinned source at
`ephemeral/inspiration/reference/kit/packages/kit/src` (`3.0.0-next.25`).

---

## The gaps, ranked

Ranked by how much each one stands between skgo and real work. A thing every app
needs outranks a thing one app wanted.

### 1. Server loads — kit asks Go for `__data.json` and Go answers with HTML

This is the only gap that changes what kind of thing skgo is.

**The reproduction is one file.** `web/src/routes/serverload/+page.server.ts`
exports a `load` that throws — the same throwing-stub shape skgo already uses for
remote functions:

```ts
export const load: PageServerLoad = () => {
	throw new Error('skgo: server loads are implemented in Go');
};
```

Rebuild, open `/serverload`, watch the network. Kit's client — with `ssr = false`,
on first paint, unprompted — issues:

```
GET /serverload/__data.json?x-sveltekit-invalidated=01
```

skgo answers **404 with the SPA boot document, `Content-Type: text/html`**. The
page renders `404 / Not Found` (`shots/serverload.png`).

Worse than the 404 is the near miss. skgo matches unknown paths against kit's own
route regexes, and those end `\/?$`, so a data URL one segment deep *matches its
own page route*:

| request | skgo answers |
| --- | --- |
| `GET /messages/__data.json` | **200**, `text/html`, the SPA document |
| `GET /messages/1/__data.json` | 404, `text/html`, the SPA document |
| `GET /serverload/__data.json?x-sveltekit-invalidated=01` | 404, `text/html`, the SPA document |

Kit's client `JSON.parse`s the first one. This is wrong today, before any load is
implemented: `/**/__data.json` is kit's URL, not a page, and skgo should never put
a document on it.

**Why it outranks everything else.** `__data.json` is the only endpoint kit's
client calls on its own initiative, on every navigation, for every route in the
branch. Riding on it are: layout data composed with page data, `parent()`,
cookies written during navigation, `depends()`/`invalidate()`, per-node errors
that reach `+error.svelte` with the right status, redirects, and deferred
promises. A developer without it has none of those and must hand-wire each one
onto a remote function — which is what the rest of this report is mostly about.

**Kit has already answered every question this raises**, and the answers are not
adjacent to what skgo does, they are close to it:

- `ssr` is **orthogonal** to `__data.json`. Grepping `ssr` across
  `runtime/server/data/index.js`, `runtime/server/page/data_serializer.js` and
  `runtime/server/page/load_data.js` returns nothing. Turning SSR off removes the
  hydrate payload and the HTML data embedding; it does not remove the data
  endpoint. Under `ssr = false` the client fetches it on *every* page view
  including the first — which is a *simpler* contract than SSR, because there is
  exactly one wire format and no `devalue.uneval` HTML path at all.
- The switch is `__SVELTEKIT_HAS_SERVER_LOAD__`, a Vite define
  (`exports/vite/index.js:1250-1258`) computed from whether any *built*
  `+*.server.js` **exports** `load` (`utils/routing.js:344-346`,
  `core/postbuild/analyse.js:84-87`). Kit never calls it. The throwing stub is
  enough. I proved the bit flips: before the probe route, `__data.json` did not
  appear in the client bundle at all; after it, the bundle contains it and the
  browser makes the request.
- Response shape: `{"type":"data","nodes":[…]}` newline-delimited,
  `content-type: application/json`, or `text/sveltekit-data` when promises are
  deferred (`runtime/server/data/index.js:113-155`). One entry per node in
  `[...layouts, leaf]`, positionally aligned with the `x-sveltekit-invalidated`
  string; `null` for a node with no server file, `{"type":"skip"}` for one that
  was not invalidated, `{"type":"data","data":<raw devalue spliced into the JSON
  line>,"uses":{…},"slash"?:…}` otherwise
  (`runtime/server/page/data_serializer.js:187-213`).
- `uses` (`runtime/server/utils.js:77-97`): `dependencies` as **absolute hrefs**,
  `params`, `search_params`, and `parent`/`route`/`url` as the literal number
  `1`, all omitted when empty.
- Errors are **HTTP 200** with `{"type":"error","error":{…}}` at the node's index
  (`runtime/server/data/index.js:94-110`). Redirects are **HTTP 200** with
  `{"type":"redirect","status":…,"location":…}`
  (`runtime/server/data/index.js:161-169`). A server returning a real 3xx or 404
  on this channel breaks kit's client.
- Deferred promises are extra NDJSON lines
  `{"type":"chunk","id":n,"data"|"error":<devalue>}`
  (`runtime/server/page/data_serializer.js:135-182`).
- One case that is easy to miss: `GET /<unmatched-path>/__data.json?x-sveltekit-invalidated=1`
  must return the **root layout's** data, not a 404
  (`runtime/server/respond.js:760-778`), or the error page loses its layout data.

Everything skgo needs is already in the tree: a devalue encoder
(`internal/devalue`), kit's route patterns in the manifest, and a cookie jar.

### 2. There is no server-side place to write a cookie during navigation

The junkyard's `+layout.server.ts` increments a `visits` cookie on every request
and returns `{ user, visits }`. Kit runs it per navigation and it may write
cookies.

Under skgo there is exactly one cookie writer: a `command`. Kit forbids cookie
writes in a `query` (`runtime/app/server/remote/shared.js:82-128`) because a
query's result is cached by its argument and replayed, and skgo mirrors that rule
correctly (`event.go:117-125`). So the port has to fire a command from the client
after the page is already on screen:

```svelte
onMount(() => {
	void recordVisit().updates(getSession());
});
```

Two things went wrong, and both are worth recording.

**It cannot live in an `$effect`.** The single-flight refresh re-runs the effect,
which fires the command again. Measured: `visits` reached 7 after two page loads.

**Even in `onMount` it is racy.** `getSession()` (GET) and `recordVisit()` (POST)
go out concurrently. The query reads the pre-increment cookie; if its response
lands after the command's refresh, the stale value wins. Measured across four
reloads: the page displayed `["1","1","3","4"]` while the cookie was `1,2,3,4`.
The scenario is correspondingly flaky — three of five runs green. `shots/home-first-visit.png`
and `shots/home-second-visit.png` are byte-identical, both reading "visits 1",
and on the runs where it passes nothing about the page is more correct.

This is downstream of gap 1, but it earns its own rank: session rotation, flash
messages, last-seen timestamps and login are all this shape, and the failure is
silent, green-looking and intermittent.

### 3. No form actions, no `form` remote functions, no file uploads

Three separate absences that a developer meets as one.

- `+page.server.ts` `export const actions` — nothing parses `?/name`, nothing
  reads `application/x-www-form-urlencoded`. A POST to a page path is answered
  **405 `Allow: GET, HEAD`** by the static handler (`static.go:198-202`); the
  remote prefix check never sees it.
- Kit's `form` remote kind does not exist. `internal/gen/scan.go:31-35` has
  exactly three markers: `Query`, `Command`, `LiveQuery`. So does the dispatcher.
- **No Go remote function can accept a file.** `internal/remotearg` models a
  JavaScript `File` faithfully, but the argument path marshals the parsed devalue
  tree to JSON and unmarshals it into the Go type (`remote.go:183-198`), which
  destroys it — and the type projector refuses the shape anyway.

The login flow *was* portable: two Go commands plus hand-written
`onsubmit`/`preventDefault`/`goto`. It works — validation failure, sign-in,
sign-out and the cookie all render correctly (`shots/login-validation-js.png`,
`shots/login-signed-in-js.png`, `shots/login-whoami-js.png`,
`shots/login-signed-out-js.png`). What the developer pays is the wiring, plus
`fail()`, per-field validation issues, and `page.form`.

The file upload was not portable at all. The messages page ships without its
"Attach a file" half — compare `shots/messages-attach-form.png` against the
junkyard's `/messages`, which has a caption field, a file input and an upload
button below the list.

Kit's answer for a remote form is specified down to the byte: POST
`/_app/remote/<hash>/<name>` with `Content-Type: application/x-sveltekit-formdata`
— version byte `0`, u32 little-endian header length, u16 little-endian
offset-table length, `devalue.stringify([data, meta])`, `JSON.stringify(offsets)`,
then file bytes sorted small-first (`runtime/form-utils.js:105-171`) — or an
ordinary form encoding with mangled field names. The no-JS fallback posts to the
page URL with `?/remote=<hash>/<name>`, which is moot under CSR.

### 4. `transport` — a custom class becomes a silently blank region of the page

The junkyard app declares one codec in `src/hooks.ts`:

```ts
export const transport: Transport = {
	Money: { encode: (v) => v instanceof Money && [v.cents, v.currency],
	         decode: ([cents, currency]) => new Money(cents, currency) }
};
```

skgo has no notion of it. `getListPrice` puts a plain object on the wire:

```
{"type":"result","data":"[{\"_\":1,\"q\":4},{\"cents\":2,\"currency\":3},1999,\"USD\",…]"}
```

where kit's client expects a devalue custom-type node keyed `Money`. The page
then calls `price.format()` on a plain object.

**`shots/prices.png` is the most valuable screenshot in this survey.** It shows
the nav, the layout line, the heading "Prices", and *nothing else*. No error
boundary, no message, no console error, no page error — Svelte's
`{#await … then}` swallows the TypeError into an inert derived, and the only
trace is six `derived_inert` warnings. A scenario that checked "the page loaded"
would be green over a page with its content missing.

Rank is high because this is how any app carries a domain type — money, decimals,
dates, branded ids — and because kit has already done most of the work: the
`decode` half is compiled into the browser bundle from the *universal* hook
(`core/sync/write_client_manifest.js:163-167`). skgo's job is only to emit the
custom-type node under the matching key, and it already owns a devalue encoder.

### 5. A panic in a Go remote function is not recovered

`/error/unexpected` panics, mirroring the junkyard's `throw new Error('SECRET-INTERNAL-DETAIL: …')`.

skgo does not recover. The panic escapes to `net/http`'s per-connection handler:
`curl` gets **"Empty reply from server"** — no status line, no body, connection
closed — and a full stack trace carrying the secret goes to the process log.

The browser *looks* almost right (`shots/error-unexpected.png`: `500` /
`Internal Error`), because kit's client falls back to `{status: 500, message:
'Internal Error'}` when the fetch fails. The secret does not reach the client.
But nothing about that outcome was skgo's doing, and it is a connection reset
under any load, not an error response.

Related, and the reason the scenario still fails: there is no `handleError`
equivalent. The junkyard app wanted "Something went wrong on our side." with
`code: 'INTERNAL'`, and its expected error wanted `code: 'TEAPOT'`. skgo has no
hook to shape the message, no way to add fields to `App.Error`, no logging seam,
and no `kind: 'validation'` path. Kit's own contract
(`runtime/server/errors.js:44-83`) collapses an unexpected throw to
`{status: 500, message: 'Internal Error'}` *before* the hook sees it and merges
the hook's return over that — which is exactly the shape skgo would need.

### 6. No standalone endpoints, and no seam to add one

`POST /api/echo` → **405**. The manifest lists `/api/echo` as a route, so a *GET*
to it returns the SPA document at **200 `text/html`** — the wrong thing to hand an
API client. `NewRemotes`, `NewStaticHandler` and `NewDevProxy` are the whole
public handler surface; there is no way to register a path.

This one is trivially closable at the app level — a developer composes their own
`http.ServeMux` in front of `remotes.Intercept(...)` — so it should be ranked as
a missing seam and a manifest bug, not a missing feature. Two things skgo should
mirror when it does: kit's endpoints are entirely unaffected by `ssr`
(`types/internal.d.ts:519-525` has no `ssr`/`csr` field), and
`/<path>/__data.json` on an endpoint route must be a bare 404
(`runtime/server/data/index.js:29-32`).

The junkyard's 413 body-limit assertion rides on this endpoint. skgo has no body
size limit anywhere, on any path.

### 7. Trailing slashes are not normalized

`GET /about/` → **200** with the SPA document. Kit returns **308** to `/about`.
skgo's `matchesRoute` uses kit's own patterns, which end `\/?$` and so match both
spellings; kit's `trailingSlash` option (default `'never'`) is not carried in the
manifest and not read. Cheap to close. It matters because two URLs for one page
splits HTTP caches and, once loads exist, query-cache keys.

### 8. No precompression

No `.br`/`.gz` variants are written by the adapter, no `Accept-Encoding`
negotiation, no `Vary` in `static.go`. The junkyard's adapter-node build
precompresses and its scenario checks the negotiation. Every byte of the client
bundle currently goes out uncompressed. Purely additive.

### 9. Prerendered output is written and never served

The adapter calls `builder.writePrerendered(build/prerendered)`; the static
handler only ever indexes `client/`. In this app it happens not to matter —
`/about` prerenders to the same shell and its universal `+page.ts` produces the
content in the browser (`shots/about-prerendered.png` is correct) — so today it is
dead weight, not a bug.

It becomes real the moment `prerender` remote functions arrive: kit fetches those
from a **path-segment** URL, `/_app/remote/<hash>/<name>/<payload>`
(`runtime/client/remote-functions/prerender.svelte.js:69`), *specifically* so they
can be served as static files. The junkyard's `getBanner` is a `prerender`; I
ported it as an ordinary query. It works (`shots/messages.png`) at the cost of a
network round trip on every page view — and no scenario can tell the difference,
which is itself worth noting (see below).

### 10. `skgo.Redirect` drops the status

`redirect(307, '/messages')` has to become `&skgo.Redirect{Location: "/messages"}`
— the struct has no `Status` field, and the compiler said so during the port.

Under CSR this is invisible: kit's client always re-navigates with a fresh GET, so
the ported page lands on `/messages` correctly (`shots/redirect-landing.png`, and
the scenario passes). But kit's own wire format carries `status`, skgo's type
silently discards information the developer wrote down, and a command returning a
redirect is downgraded to an opaque 500 (`remote.go:499-503`). Nearly free to fix.

### 11. No `locals`, no `handle`, no `setHeaders`

`whoami` renders `url = /_app/remote/xu03kg/whoami` where kit rendered
`http://127.0.0.1:8090/whoami` (`shots/whoami.png`). **That part is not a gap.**
Kit *throws* if a query reads `event.url`, `event.params` or `event.route`
(`runtime/app/server/remote/shared.js:82-128`), because a query's result is cached
by its argument and would go stale. skgo's answer — put page context in the
argument — is kit's answer, and the scenario is asserting a property kit itself
forbids for queries.

What skgo does lose is real but smaller: the junkyard's `hooks.server.ts` stamped
`x-guestbook-request-id` on every response and put the id in `locals` so any load
could read it. skgo generates the id fine (`shots/whoami.png` shows it) but has no
way to put it on a response header — `setHeaders` does not exist — and no
per-request scratch space shared between remote functions, so every function
re-derives the session from the cookie. Client address and `Host` both work.

---

## What is not a gap: the CSR bill

Four failures are the architecture, not the implementation, and should not be
carried on any list of work.

- **`home page is server-rendered`** — `request.get('/')` must contain "hello from
  the server". The fallback document contains no app HTML by construction.
- **`hydration does not refetch`** — asserts `requests.data()` is empty after the
  first paint. Under `ssr = false` kit never hydrates (`hydrated` is set only
  inside `_hydrate`, `runtime/client/client.js:3540`), so the client always
  fetches. skgo made 2 requests. Correct behaviour, wrong assertion.
- **The document's HTTP status for `/error/expected` (418), `/error/unexpected`
  (500) and `/messages/999` (404)** — the document is always the SPA shell.
  Note that skgo gets the *in-page* status right: `+error.svelte` renders
  `418 / expected teapot` (`shots/error-expected.png`) and `404 / no message #999`
  (`shots/unknown-message-id.png`) from a `skgo.Errorf` returned by a Go query
  that a universal load awaited. Only the outer status differs, which matters to
  crawlers and `curl`, not to a user.
- **Both `without JavaScript` scenarios** — `shots/login-nojs.png` is a blank white
  page. CSR-only means no-JS is not a mode.

And one thing that is genuinely good news: **a universal `+page.ts` that awaits a
Go remote function is a working substitute for a server load's error and redirect
behaviour.** `error(418)`, `error(404)` and `redirect()` all reach kit's route-level
machinery through that path, with the right status inside the page. That is not
data composition, cookies or invalidation — but it is more than "server loads are
unimplemented" would suggest.

---

## Scenarios that pass while proving nothing

Applying this project's own rule to the junkyard's suite.

- **`live query pushes updates made from another tab`** passes against skgo. Look
  at `shots/messages-live-push.png`: **"live count: 3" beside a list of two
  messages.** The SSE push updated the counter; the message list from the other
  tab never arrived. Kit would behave identically — a live query pushes its own
  value and nothing refreshes `getMessages` cross-tab — so the scenario asserts
  the one number that moves and ignores the inconsistency a user would see first.
- **`layout load composes with page loads and persists a cookie`** passed three of
  five runs against a workaround that is racy by construction, and its screenshots
  read "visits 1" on both the first and second visit.
- **`query renders server data, prerendered remote renders its banner`** — the
  word "prerendered" does no work. Nothing distinguishes a banner served from a
  static file from one computed per request. It passes against an ordinary query,
  which is exactly what it got.
- **`request bodies over the limit are rejected with 413`** is an app-server
  property (adapter-node's `BODY_SIZE_LIMIT`) smuggled into an app spec through a
  route that exists only to be POSTed to.
- **`classic form actions work as plain HTML forms`** and **`hydration does not
  refetch`** measure the architecture, not the implementation. They are
  load-bearing for adapter-node and unsatisfiable here by design.

---

## Is anything impossible?

No. Nothing in this app requires rethinking skgo's shape.

The one demand that looked structural — server loads — turns out to be the
*cleanest* case under CSR. `__data.json` is orthogonal to `ssr`, so there is
exactly one wire format to implement instead of two, no HTML-embedded
`devalue.uneval` path, and the same throwing-stub trick that already proves Go
answered a remote function is what flips `__SVELTEKIT_HAS_SERVER_LOAD__`. The
pieces are present.

Everything else on the list is additive: a marker kind, a codec table, a
`recover()`, a status field, a `Vary` header, a 308.

---

## Things skgo already does that this app shows are wrong

1. **`/<path>/__data.json` gets an HTML document.** 404 for most paths, and **200
   `text/html`** for a one-segment path that satisfies its own route regex
   (`/messages/__data.json`). Kit's client will `JSON.parse` that. Worth fixing
   before loads exist, not after.
2. **`skgo generate` writes into the skgo library's own module.** Generating the
   guestbook produced `skgo_polytype_gen.go`, `jsonschema_gen.go` and
   `jsonschema/` at the repository root, because a remote function returned
   `skgo.None` and that type needed projecting. In a real downstream project skgo
   lives in the read-only module cache and this fails outright. The example never
   hits it only because none of its functions return `skgo.None`.
3. **No panic recovery** in the remote dispatcher — a panicking handler resets the
   connection with no HTTP response at all.
4. **`skgo.Redirect` has no `Status`**, and a command returning one becomes an
   opaque 500.
5. **`serveDocument` sets an ETag but never answers 304** — it always writes the
   body, even when the request carries a matching `If-None-Match`.

---

## Scenario results

| Scenario (junkyard file) | skgo | Screenshot |
| --- | --- | --- |
| streamed load: fast part before slow part (loads) | pass | `slow-pending.png`, `slow-resolved.png` |
| layout load composes, cookie persists (loads) | **flaky** (3/5) | `home-first-visit.png`, `home-second-visit.png` |
| transport codec round-trips (loads) | **fail** | `prices.png` |
| expected error renders status and message (errors) | **fail** (page right, HTTP status 200) | `error-expected.png` |
| unexpected error, 500, no leak (errors) | **fail** (no leak; no response at all) | `error-unexpected.png` |
| form actions: validation, login, logout (forms) | pass, hand-wired to commands | `login-*.png` |
| form actions without JavaScript (forms) | **fail** by design | `login-nojs.png` |
| query + prerendered banner (remote) | pass, banner is an ordinary query | `messages.png` |
| command updates query in a single flight (remote) | pass, with client-side `.updates()` | `messages-after-command.png` |
| live query pushes cross-tab (remote) | pass, list stale | `messages-live-push.png` |
| remote form uploads a file (remote) | **fail**, no form kind, no File args | `messages-attach-form.png` |
| remote form upload without JavaScript (remote) | **fail** by design | — |
| home page is server-rendered (routing) | **fail** by design | — |
| hydration does not refetch (routing) | **fail** by design | `routing-home-hydrated.png` |
| deep link and client nav agree (routing) | pass | `message-deep-link.png`, `message-client-nav.png` |
| unknown route renders 404 page (routing) | pass | `unknown-route.png` |
| unknown message id is 404 with app message (routing) | **fail** (page right, HTTP status 200) | `unknown-message-id.png` |
| prerendered page + trailing-slash 308 (routing) | **fail** (page right, no 308) | `about-prerendered.png` |
| redirect in a load lands on destination (routing) | pass | `redirect-landing.png` |
| url, client address, hook-set locals (platform) | **fail** | `whoami.png` |
| static assets: type, immutable, etag 304 (platform) | pass | — |
| precompressed assets negotiated (platform) | **fail** | — |
| body over the limit is 413 (platform) | **fail** | — |
| *(added)* does kit ask Go for `__data.json`? | yes; Go answers HTML | `serverload.png` |
