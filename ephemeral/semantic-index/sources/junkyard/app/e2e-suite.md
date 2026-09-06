# Junkyard guestbook — Playwright task suite

**Purpose.** The 23-test, black-box Playwright suite the junkyard wrote against user-observable behaviour of the guestbook, plus its two helpers and the `BASE_URL` targeting convention. It is the closest thing to an acceptance suite skgo already has; this file records what each test asserts so the Go server's suite can be derived by keeping, editing, or dropping each one.

## Key concepts

### Helpers (`junkyard/app/e2e/helpers.ts`)
- **`recordRequests(page)`** — `L4-L16`: subscribes to `page.on('request')` from the moment it is called and buckets by `resourceType()`: `documents()` = full document loads, `data()` = `fetch` + `xhr` ("programmatic requests to the server — no URL shapes assumed", `L13-L14`), `matching(re)`. Tests call it **after** the initial `goto` + hydration when they want to count only what an interaction caused (`remote.spec.ts:L18`, `forms.spec.ts:L31`), or **before** `goto` when they want to measure the initial load itself (`routing.spec.ts:L12`).
- **`waitForHydration(page)`** — `L18-L20`: waits until `data-testid="hydrated"` reads `yes` (set by `$effect` in `+layout.svelte:L7-L9`). This is the gate between "SSR HTML is on screen" and "client is interactive"; several counting tests depend on it.
- **JS-disabled variants** use `test.describe(...) + test.use({ javaScriptEnabled: false })` (`remote.spec.ts:L71-L77`, `forms.spec.ts:L39-L45`) and share the same flow function as the JS run (`uploadFlow`, `loginFlow`) so one flow proves both progressive-enhancement paths.
- **Unique text** per run: `unique(label)` = label + timestamp + random (`remote.spec.ts:L5`) so tests are order-independent against a persistent in-memory store.
- **Two-context tests**: `browser.newContext()` for a second "tab" with separate cookies (`remote.spec.ts:L37-L45`).
- **Raw HTTP** via the `request` fixture (`APIRequestContext`) for status/headers/body without a browser (`routing.spec.ts:L5-L9,L52-L60`, `platform.spec.ts:L15-L33,L52-L57`).
- **Targeting**: `baseURL = BASE_URL ?? http://127.0.0.1:4173`; setting `BASE_URL` disables the built-in `webServer` (`junkyard/app/playwright.config.ts:L5,L17`). Tests read `baseURL` from the fixture when they need it (`platform.spec.ts:L3-L7`). The suite is serial, 1 worker, 0 retries (`playwright.config.ts:L9-L11`) because the store is shared mutable state.

### Test inventory (23 tests)
Legend for the **CSR** column: **P** = portable as written to a CSR-only Go server; **E** = portable after an edit; **S** = asserts SSR/Node-only behaviour.

#### `junkyard/app/e2e/remote.spec.ts`
| # | Test | Asserts | CSR |
|---|---|---|---|
| 1 | query renders server data, prerendered remote renders banner (`L7-L12`) | `/messages`: banner text `guestbook — prerendered banner`, first message visible, list contains `welcome to the guestbook` | **E** — passes if Go answers `getBanner` at runtime (or the adapter still emits the prerendered remote file). |
| 2 | command updates the query in a single flight (`L14-L30`) | after hydration, post a message: list contains it, `live-count` = before+1, **exactly 1** `fetch/xhr` request and **0** document loads | **P** — Go must return the refreshed `getMessages` result inside the command response (kit's single-flight envelope) and the live count must tick. |
| 3 | live query pushes updates made from another tab (`L32-L46`) | second context posts; first page's `live-count` updates within 10 s | **P** — requires Go to implement `query.live` streaming (`messages.remote.ts:L8-L14`). |
| 4 | remote form uploads a file without a page reload (`L63-L69`) | multipart upload via `attachFile`; `attach-result` shows `uploaded hello.txt (16 bytes)`; list item shows attachment; only 1 document load (the explicit goto) | **P** — Go must parse multipart remote-form POST and return `{id,name,size}` + refreshed query. |
| 5 | **without JavaScript**: remote form uploads via the page-reload fallback (`L71-L77`) | same `uploadFlow` with JS off | **S** — a JS-disabled page can only show results if the server renders HTML. DROP. |

#### `junkyard/app/e2e/loads.spec.ts`
| # | Test | Asserts | CSR |
|---|---|---|---|
| 6 | streamed load shows fast part before slow (`L5-L10`) | `goto('/slow', {waitUntil:'commit'})`; `fast` = `fast part`; `slow` = `loading…` then `slow part` within 5 s | **E** — needs Go to stream deferred promise chunks in the data response (kit's devalue chunked protocol). If Go resolves everything before responding, `fast` appears only after 1.5 s and `loading…` may never be observable; relax to "eventually `slow part`" or implement streaming. |
| 7 | layout load composes with page loads and persists a cookie (`L12-L19`) | `visits` = 1 on first `/`, greeting text, `visits` = 2 after reload | **P** — Go layout load must `Set-Cookie: visits` on the data response; layout + page loads must both run. Watch for double-invocation of the layout load per navigation (would give 2, 4). |
| 8 | custom transport codec round-trips through a load and a query (`L21-L31`) | `price` `USD 19.99` + `price-is-money` `Money`; `sale-price` `USD 14.99` + `Money`; still `Money` after hydration | **P** — Go must emit the `Money` transport encoding (`hooks.ts:L6-L9`) in both load data and query responses. |

#### `junkyard/app/e2e/routing.spec.ts`
| # | Test | Asserts | CSR |
|---|---|---|---|
| 9 | home page is server-rendered (`L5-L9`) | raw `GET /` is 200 and body contains `hello from the server` | **S** — literally asserts SSR. DROP, or invert to "GET / is the shell (200, contains `_app/immutable`)" and assert the greeting via the data endpoint. |
| 10 | hydration does not refetch, client navigation does not reload (`L11-L27`) | before `goto('/')`: after hydration **0** data requests, 1 document; click "Who am I" → URL `/whoami`, `request-id` non-empty, still 1 document, >0 data requests | **E** — first half is SSR-only (CSR must fetch data on first paint, so `data().length ≥ 1`). Keep the second half (client nav = no document reload, data fetch happens); change first assertion to "exactly one data request for initial render" if desired. |
| 11 | deep link and client navigation render the same dynamic page (`L29-L38`) | `/messages/1` text equals text reached via client nav from `/messages` | **P** |
| 12 | unknown route renders the 404 error page (`L40-L44`) | `goto('/does-not-exist')` response status 404 **and** `error-status` = 404 | **P** — Go serves the shell with HTTP 404 for unmatched non-asset paths; kit's client router renders `+error.svelte` with 404 on its own. |
| 13 | unknown message id is a 404 with the app message (`L46-L50`) | `goto('/messages/999')` status 404 and `error-message` = `no message #999` | **E** — the message part is portable (data response carries `error(404,...)`). The **document** status is 404 only if Go runs the page's server load when serving the document (or the assertion is moved to the data response). |
| 14 | prerendered page is served and trailing slash redirects (`L52-L60`) | raw `GET /about` body contains `rendered at build time`; `GET /about/` is **308** with `location` ending `about` | **E/S** — body assertion needs a prerendered HTML file (build-time SSR). The 308 trailing-slash redirect is portable and worth keeping. |
| 15 | redirect thrown in a load lands on the destination (`L62-L66`) | `goto('/redirect')` ends at `/messages` with the Messages heading | **P** — either Go returns HTTP 307 on the document, or the data response carries kit's redirect envelope and the client router follows it. |

#### `junkyard/app/e2e/forms.spec.ts`
| # | Test | Asserts | CSR |
|---|---|---|---|
| 16 | classic form actions: validation failure, login redirect, logout (`L28-L37`) | `loginFlow` (`L5-L26`): empty submit → `login-error` `name is required`, still on `/login`; fill `tyler` → lands on `/`, `user` = tyler; `/whoami` shows tyler; logout → `logged-out` visible, `user` = anonymous. Document loads include `/whoami` and `/login` (explicit gotos) | **P** — Go must implement kit's form-action protocol for `?/login`/`?/logout` under `use:enhance` (JSON action result: `fail` → `{type:'failure',status,data}`, redirect → `{type:'redirect',location}`), set/delete the `session` cookie, and invalidate loads so `user` updates. |
| 17 | **without JavaScript**: classic form actions work as plain HTML forms (`L39-L45`) | same flow, JS off | **S** — requires server-rendered HTML after a plain POST. DROP. |

#### `junkyard/app/e2e/errors.spec.ts`
| # | Test | Asserts | CSR |
|---|---|---|---|
| 18 | expected error renders its status and message (`L3-L8`) | `goto('/error/expected')` status **418**, `error-status` 418, `error-message` `expected teapot` | **E** — same document-status caveat as #13; the rendered status/message are portable from the data response. |
| 19 | unexpected error renders a 500 and leaks nothing (`L10-L19`) | status 500; `error-status` 500; message `Something went wrong on our side.`; response body contains neither `SECRET` nor `hunter2` | **E** — rendered parts portable if Go scrubs unknown errors like `handleError` (`hooks.server.ts:L16-L20`). The body-leak check on the document is vacuous under CSR (the shell never contains the secret); move it to the data response body. |

#### `junkyard/app/e2e/platform.spec.ts`
| # | Test | Asserts | CSR |
|---|---|---|---|
| 20 | the app sees its own URL, client address, and hook-set locals (`L3-L12`) | `url` = `${baseURL}/whoami`; `client-address` non-empty; `host` = baseURL host; `request-id` matches UUID regex **and equals the document response's `x-guestbook-request-id` header** | **E** — all fields portable from a Go load. The header equality couples the document response to the load's request; under CSR they are separate requests. Either compare against the **data** response's header, or have Go run the load while serving the document. Also: `url.href` must be the *page* URL, not the `__data.json` URL. |
| 21 | static assets: content type, immutable caching, etag revalidation (`L14-L34`) | `/logo.svg` 200 `image/svg+xml`; some `/_app/immutable/` request occurs; that asset is 200 with `cache-control` containing `immutable`, has an `etag`, and `if-none-match` → **304** | **P** — pure static-file behaviour for Go. |
| 22 | precompressed assets are negotiated (`L36-L49`) | `/_app/immutable/*.js` with `accept-encoding: br` → `content-encoding: br` and `vary` contains `accept-encoding`; `identity` → no `content-encoding` | **P** — Go serves `.br` siblings produced by `precompress: true` (or compresses on the fly) and sets `Vary`. |
| 23 | request bodies over the limit are rejected with 413 (`L51-L58`) | POST `/api/echo` 100 000 bytes → 200 `{bytes:100000}`; 600 000 bytes → **413** | **P** — Go endpoint + body limit (kit's default is 512 KiB; keep the same threshold). |

## Citations
- `junkyard/app/e2e/helpers.ts:L4-L16`, `:L18-L20`
- `junkyard/app/e2e/remote.spec.ts:L14-L30` (single-flight count), `:L32-L46` (live), `:L48-L61` (uploadFlow), `:L71-L77` (no-JS)
- `junkyard/app/e2e/loads.spec.ts:L5-L10`, `:L12-L19`, `:L21-L31`
- `junkyard/app/e2e/routing.spec.ts:L5-L9`, `:L11-L27`, `:L40-L50`, `:L52-L60`
- `junkyard/app/e2e/forms.spec.ts:L5-L26`, `:L39-L45`
- `junkyard/app/e2e/errors.spec.ts:L10-L19`
- `junkyard/app/e2e/platform.spec.ts:L3-L12`, `:L14-L49`, `:L51-L58`
- `junkyard/app/playwright.config.ts:L5`, `:L17-L24`

## Reusable verdicts
- `helpers.ts` — **REUSE AS-IS**. Resource-type bucketing makes no assumptions about URL shapes, so it works unchanged against Go's endpoints.
- Directly reusable as the Go acceptance suite (**P**): #2, #3, #4, #7, #8, #11, #12, #15, #16, #21, #22, #23 — twelve tests.
- Reusable after one-line edits (**E**): #1 (only if `prerender` remotes are answered at runtime), #6 (streaming or relax), #10 (drop the "0 data requests after hydration" line), #13/#18 (document status), #14 (keep the 308 half), #19 (move leak check to data response), #20 (compare request-id against the data response).
- SSR-only, **DROP** (or keep in a separate adapter-node reference lane): #5, #9, #17 — the two `javaScriptEnabled: false` tests and the literal "home page is server-rendered" test. Optionally add a converse test: "GET / returns the CSR shell and never contains load data".
- `playwright.config.ts` — **REUSE WITH CHANGES** (see toolchain.md).
- `check-level.sh` exact-count gate — **DROP the shell; REUSE the idea** in Go or in the Playwright reporter config.

## Gotchas
- **`waitForHydration` is nearly instant under CSR**, because the whole page is client-rendered. Any assertion that was meant to run "between SSR paint and hydration" (#6's `loading…`, #10's zero-data-requests) changes meaning.
- **Document `response.status()` assertions** (#12, #13, #18, #19) require Go to decide the status before serving the shell. For unmatched routes this is trivial; for load-time errors it requires running the load (or a cheap pre-check) on the document request.
- **Request counting starts at `recordRequests()` call time**; put it after `waitForHydration` when measuring an interaction, before `goto` when measuring initial load (`routing.spec.ts:L12-L14` vs `remote.spec.ts:L16-L18`).
- **The store is process-global and never reset**; tests use `unique()` text and relative counts (`before + 1`). A Go server must likewise keep state across the serial run, and the suite must stay `workers: 1`.
- **Cookie tests assume `Set-Cookie` on whatever response carries load data**; under CSR that is the data fetch, not the document. Playwright's `request` fixture and the page share a cookie jar per context.
- **#20 `url` assertion** requires `paths.origin` at build time to equal Playwright's `baseURL`, and the Go load's `url.href` to be the page URL (strip any `__data.json`/query-marker Go receives).
- **Brotli negotiation (#22)** is checked with a raw `accept-encoding: br` header; Playwright's `request` does not auto-decompress-and-hide the header, so `content-encoding` is visible.

## Recipes
- To point the suite at a running Go server: `BASE_URL=http://127.0.0.1:PORT mise x -- node node_modules/@playwright/test/cli.js test` — `junkyard/app/playwright.config.ts:L5,L17`.
- To run only the CSR-portable set today: `--grep-invert "without JavaScript|server-rendered|prerendered page is served"` (titles at `remote.spec.ts:L71`, `routing.spec.ts:L5`, `routing.spec.ts:L52`); or tag tests `@ssr` and reuse `GREP_INVERT` from `junkyard/ephemeral/projects/skgo/targets/node.sh:L2`.
- To write a new request-counting test: copy `remote.spec.ts:L14-L30` and `helpers.ts:L4-L16`.
- To write a two-client push test: copy `remote.spec.ts:L32-L46`.
- To test a multipart upload: `remote.spec.ts:L48-L61` (`setInputFiles` with an in-memory buffer).
- To assert static-asset headers from Go: `platform.spec.ts:L14-L49`.
