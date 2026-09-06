# Junkyard guestbook — CSR-only portability

**Purpose.** Answers, route by route and test by test, what survives when the guestbook is served by a Go binary with **no SSR and no JS runtime in production**: Go serves the built client bundle, Go answers every load / remote-function / action / endpoint at the URLs kit's client expects, and the generated TS stubs throw if ever executed. Use this to decide what the skgo adapter and generator must implement, and which junkyard assertions to keep.

## Key concepts

### What the kit client needs from a server under CSR
Kit's client-side runtime, once booted from a shell HTML page, makes only these kinds of requests. Each maps to something Go must serve; everything else in the junkyard is SSR machinery.
- **Shell document** for every page route (kit's SPA fallback shape: `app.html` with `%sveltekit.head%`/`%sveltekit.body%` filled with the client entry and no data) — `junkyard/app/src/app.html:L1-L12`. Go must serve this for `/`, `/messages`, `/messages/1`, `/login`, ... and for unknown paths (with **404** status, see routing #12 in e2e-suite.md).
- **Static bundle** under `/_app/immutable/**` with `immutable` cache-control, ETag/304, and `.br` negotiation — asserted by `junkyard/app/e2e/platform.spec.ts:L14-L49`; produced by `vp build` with `precompress: true` (`junkyard/app/vite.config.ts:L8`).
- **Server-load data** for routes that have `+page.server.ts`/`+layout.server.ts` (the client fetches kit's data endpoint for the route and expects devalue-encoded nodes, including transport-encoded classes and deferred promise chunks). Routes: `/` (`routes/+page.server.ts:L4-L7` + `routes/+layout.server.ts:L3-L8`), `/messages/[id]` (`routes/messages/[id]/+page.server.ts:L6-L10`), `/prices` (`routes/prices/+page.server.ts:L4-L6`), `/slow` (`routes/slow/+page.server.ts:L3-L6`), `/whoami` (`routes/whoami/+page.server.ts:L3-L9`), `/login` (`routes/login/+page.server.ts:L4-L7`), `/redirect` (`routes/redirect/+page.server.ts:L4-L6`), `/error/*` (`routes/error/expected/+page.server.ts:L4-L6`, `routes/error/unexpected/+page.server.ts:L3-L5`). The layout load runs for **every** route, so `visits` increments on every page's data fetch.
- **Remote function calls** — `query` GET / `command` POST / `form` multipart POST / `prerender` GET at kit's `/_app/remote/<hash>/<name>` style paths: `routes/messages/messages.remote.ts:L6-L40`, `routes/prices/prices.remote.ts:L5`. Responses use the same transport/devalue encoding; `command`/`form` responses may carry refreshed query results (single-flight, `messages.remote.ts:L20,L35`).
- **Form actions** — POST to `/login?/login` and `/login?/logout` from `use:enhance` with `x-sveltekit-action: true`, answered with kit's JSON action result (`success`/`failure`/`redirect`/`error`) — `routes/login/+page.server.ts:L9-L23`.
- **Endpoints** — `POST /api/echo` (`routes/api/echo/+server.ts:L4-L7`) with a body limit that yields 413.
- **Cookies on data/action responses** — `visits` (`+layout.server.ts:L4-L5`), `session` (`login/+page.server.ts:L16,L20`). Under CSR every `Set-Cookie` rides on a fetch response, not the document; kit's client fetches with `credentials: same-origin` so this works.

### What runs in the browser unchanged
- All `.svelte` files, `+error.svelte` (`routes/+error.svelte:L1-L6`), the universal load in `routes/about/+page.ts:L3-L5`, the universal `hooks.ts` `transport` (`src/hooks.ts:L5-L10`), `lib/money.ts`. These need no Go counterpart.
- `use:enhance` and remote-function client stubs are kit's own client code; they will call whatever URLs the build assigned. The Go generator must mount handlers at **exactly** those URLs (the framing's core promise).

### Route-by-route verdict
| Route / piece | Under CSR + Go | Change needed |
|---|---|---|
| `+layout.server.ts` (visits cookie) | works | Go `layout.server.go` sets cookie on the data response; runs once per navigation |
| `/` greeting + count | works | Go `page.server.go` |
| `/about` (universal load, `prerender = true`) | works as a CSR page; **prerendered HTML does not exist** unless the adapter prerenders at build time (that is SSR-at-build in Node) | Decide: allow build-time prerender via `vp build` (Node exists at build time), or treat `/about` as a plain CSR route and drop the "rendered at build time" body assertion |
| `/messages` query/command/form/live | works if Go speaks kit's remote protocol (`messages.remote.ts:L6-L38`) | live query = long-lived streaming response; form = multipart; both need refreshed-query piggyback |
| `getBanner = prerender(...)` (`messages.remote.ts:L40`) | client expects a static prerendered remote file at build | Either the adapter emits it at build time (needs Node at build), or the generator maps `prerender` remotes to plain Go GET queries |
| `/messages/[id]` 404 | rendered message works; document status 404 only if Go evaluates the load when serving the shell | Decide document-status policy (see below) |
| `/prices` transport | works | Go emits `["Money", cents, currency]`-style transport tuples exactly as `hooks.ts:L7` encodes |
| `/slow` streaming | works only with chunked deferred data | Implement devalue deferred chunks in Go, or accept a non-streaming data response and relax loads #6 |
| `/whoami` | works | Go supplies `url.href` (page URL), client IP, host, locals; request-id header must be on the **data** response |
| `/login` actions + `enhance` | works | Go implements action JSON protocol + cookies + redirect envelope |
| `/redirect` | works | Go returns redirect envelope in data response (client follows), and/or 307 on the shell |
| `/error/expected`, `/error/unexpected` | rendered status/message work via error envelope in the data response | `handleError` scrubbing (`hooks.server.ts:L12-L21`) becomes Go middleware; document status policy again |
| `hooks.server.ts` `handle` (`L3-L10`) | no Node to run it | Go middleware: cookie → user, request id, response header |
| `/api/echo` + 413 | works | Go handler + `http.MaxBytesReader` at 512 KiB |
| Static + precompressed assets | works | Go file server with immutable/ETag/`Vary: Accept-Encoding` and `.br` sidecar selection |
| `hydrated` sentinel (`+layout.svelte:L7-L9`) | flips immediately | harmless; keep for `waitForHydration` |

### Test-by-test outcome (see e2e-suite.md for detail)
- **Keep as-is (12):** remote #2 single-flight, #3 live push, #4 upload; loads #7 cookie, #8 transport; routing #11 deep link, #12 unknown route 404, #15 redirect; forms #16 enhanced actions; platform #21 static headers, #22 brotli, #23 413.
- **Keep with edits (8):** remote #1 (prerender remote at runtime), loads #6 (streaming), routing #10 (drop "0 data requests after hydration"), #13 and errors #18 (document status), routing #14 (keep only the 308), errors #19 (leak check on data response), platform #20 (request-id from data response).
- **Drop as SSR-only (3):** remote #5 no-JS upload (`remote.spec.ts:L71-L77`), routing #9 "home page is server-rendered" (`routing.spec.ts:L5-L9`), forms #17 no-JS actions (`forms.spec.ts:L39-L45`). These are the *only* tests that need HTML rendered by a server.
- **Add (new, CSR-specific):** "GET / returns the shell with no load data and the greeting appears only after the data fetch" (proves Go, not SSR, answered); "every generated TS stub throws when imported/executed in Node" (unit-level, proves stubs never run); "data endpoint for `/error/unexpected` body does not contain `hunter2`".

### Design decisions this fixture forces
1. **Document status for load-time errors/redirects.** Kit under SSR sets 404/418/500/307 on the document because the load runs before rendering. Under CSR, Go can (a) run the page's Go load on the shell request too and mirror its status/redirect onto the shell response (keeps #13/#18/#15 byte-for-byte; costs a double load per initial visit), or (b) always serve 200 shells and let the client render the error (breaks the `response.status()` lines; those tests get edited). (a) is closer to the junkyard suite; (b) is simpler. Either way unknown routes must be 404 at the shell.
2. **Prerender at build time.** `about/+page.ts:L3` and `messages.remote.ts:L40` rely on kit running code during `vp build`. Node *is* present at build time in this project ("one build gesture"), so honouring prerender is possible without a production JS runtime — but it means those outputs come from JS, not Go, and violate "any valid response proves Go answered" for those two paths. Decide per route; the suite's #1 and #14 follow.
3. **Streaming loads.** `/slow` is the only route needing chunked deferred data. If skgo's Go load API returns plain structs, drop or relax loads #6; if it supports promise-like fields, Go must write devalue chunks progressively.
4. **`query.live`.** `getMessageCount` needs a streaming response and a server-side change notification (`store.ts:L43-L51` → Go channel). This is the most protocol-sensitive remote flavour; #3 is the proof.
5. **Request-id coupling.** `platform.spec.ts:L11` ties the document header to the load's locals. Under CSR, put the header on every Go response and compare against the data response in the test, or adopt decision 1(a).

## Citations
- `junkyard/app/src/app.html:L7-L10`
- `junkyard/app/src/routes/messages/messages.remote.ts:L8-L14` (live), `:L20,L35` (single-flight refresh), `:L40` (prerender remote)
- `junkyard/app/src/routes/about/+page.ts:L3`
- `junkyard/app/src/routes/slow/+page.server.ts:L5`
- `junkyard/app/src/routes/whoami/+page.server.ts:L3-L9`
- `junkyard/app/src/hooks.server.ts:L3-L10`, `:L12-L21`
- `junkyard/app/src/hooks.ts:L5-L10`
- `junkyard/app/src/routes/login/+page.server.ts:L9-L23`
- `junkyard/app/e2e/remote.spec.ts:L71-L77`, `junkyard/app/e2e/routing.spec.ts:L5-L9`, `junkyard/app/e2e/forms.spec.ts:L39-L45` (the three SSR-only tests)
- `junkyard/app/e2e/routing.spec.ts:L16-L18`, `junkyard/app/e2e/platform.spec.ts:L11`, `junkyard/app/e2e/loads.spec.ts:L6-L9` (the SSR-coupled assertions)

## Reusable verdicts
- The guestbook as skgo's fixture app — **REUSE WITH CHANGES**: keep all client files; replace every `*.server.ts` and `*.remote.ts` body with Go, leaving generated throwing stubs; delete `hooks.server.ts` in favour of Go middleware.
- The 12 portable tests — **REUSE AS-IS** as the first acceptance gate for the Go server.
- The 8 editable tests — **REUSE WITH CHANGES** per the notes above; the edits are one or two lines each.
- The 3 no-JS/SSR tests — **DROP** (retain only in an optional adapter-node comparison lane).
- `hydrated` sentinel + `waitForHydration` — **REUSE AS-IS**; semantically weaker under CSR but harmless.

## Gotchas
- **The shell must not contain data.** If Go ever inlines load data into the HTML, the "stubs throw" proof weakens and kit's client may try to hydrate. Serve a pure SPA shell.
- **Layout load runs on every navigation** under CSR; `visits` will count client navigations too, not just document loads. loads #7 only reloads, so it still passes, but a test that navigates client-side then checks `visits` would see extra increments.
- **`url.href` in Go loads must be the page URL** kit's client would report, not the data-endpoint URL Go actually received (platform #20).
- **`paths.origin` is compiled into the bundle** (`vite.config.ts:L10`); the Go server's listen address in tests must match `BASE_URL` used at build, or #20's `url` assertion fails.
- **Cookie `path:'/'`, `httpOnly`, `sameSite:'lax'`** are asserted only indirectly (subsequent requests carry them); Go must set them on fetch responses and the browser will apply them (same-origin).
- **Kit's client sends `x-sveltekit-action: true`** with enhanced form posts and expects JSON, while a plain HTML form post expects HTML — Go only needs the JSON path under CSR.
- **413 threshold** is kit's default 512 KiB (`platform.spec.ts:L52-L57` uses 100 000 vs 600 000 bytes); keep the threshold in that window.

## Recipes
- To scope the first Go milestone: implement shell + static + `/` data endpoint (layout+page loads with cookie) and run loads #7, routing #11/#12, platform #21-#23 — start at `junkyard/app/src/routes/+layout.server.ts:L3` and `junkyard/app/e2e/platform.spec.ts:L14`.
- To scope the remote-functions milestone: `messages.remote.ts:L6-L38` in order query → command (single-flight) → form (multipart) → live; proofs are remote #1-#4.
- To scope form actions: `login/+page.server.ts:L9-L23`; proof is forms #16.
- To scope error/redirect envelopes: `error/*/+page.server.ts`, `redirect/+page.server.ts:L5`, `hooks.server.ts:L12-L21`; proofs are errors #18-#19, routing #13/#15.
- To decide the document-status policy: re-read routing #12/#13 and errors #18/#19 lines `routing.spec.ts:L42,L48` and `errors.spec.ts:L5,L12`.
