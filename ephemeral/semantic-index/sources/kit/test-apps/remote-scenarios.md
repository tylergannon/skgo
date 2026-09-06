# Kit `async` test app: remote-function scenarios (query / batch / live / command / form / prerender)

Segment: kit test apps → remote-function and load scenarios. Pinned `@sveltejs/kit` 3.0.0-next.25
(`reference/kit/packages/kit/package.json:L3`). All paths are relative to the token cache root
`/Users/tyler/src/skgo/ephemeral/inspiration/`.

## Purpose

Catalogue every route under `reference/kit/packages/kit/test/apps/async/src/routes/remote/` (plus
`remote-lib/`, `plain-lib/`, `server-error-boundary/`, `fork/`) as a scenario: what the `.remote`
module declares, what the page does, what the Playwright tests assert, whether the scenario survives
in a CSR-only app (skgo: `ssr=false`, no JS runtime in production, Go serves the bundle and
implements the remote functions), and what the Go server must do to pass the borrowed test.

CSR-viability legend:
- **yes** — assertions hold when the page is served as an empty shell and everything runs client-side.
- **with-changes** — the behaviour is CSR-compatible but the assertion counts SSR-seeded requests
  (`expect(request_count).toBe(0)`) or checks SSR-rendered text before hydration; rewrite the count/text.
- **no** — the scenario exists to test SSR/hydration seeding or build-time bundling of the JS server.

## App-wide setup (read first)

- Remote functions are gated behind `experimental.remoteFunctions: true` and async Svelte
  (`compilerOptions.experimental.async: true`), plus `experimental.forkPreloads: true`
  (`reference/kit/packages/kit/test/apps/async/vite.config.js:L11-L22`). No `svelte.config.js` exists;
  kit options live inside `sveltekit({...})` in `vite.config.js`.
- `#lib` replaces `$lib` via `package.json` `imports`
  (`reference/kit/packages/kit/test/apps/async/package.json:L28-L31`).
- `transport` is a universal hook in `src/hooks.js`, encoding class `Foo` as `[message]`
  (`reference/kit/packages/kit/test/apps/async/src/hooks.js:L3-L9`,
  `reference/kit/packages/kit/test/apps/async/src/lib/index.js:L1-L10`).
- `hooks.server.js` `handle`: assigns a `session` cookie (httpOnly, sameSite=lax, path=/) so
  in-memory state is per browser context; returns a 403 JSON `{message:'denied by hook'}` when
  `event.isRemoteRequest && cookies.get('deny-remote') === '1'`; special-cases `/remote/hook-command`
  to call a `command` from inside the hook
  (`reference/kit/packages/kit/test/apps/async/src/hooks.server.js:L5-L39`).
- `handleError` (server): `kind === 'validation'` → `{ message: input.issues[0].message }`; must not
  throw when it touches `event.url` for a validation failure inside a query; other known kinds are
  returned as-is; unknown → `${message} (500 Internal Error, on ${pathname})`
  (`reference/kit/packages/kit/test/apps/async/src/hooks.server.js:L41-L61`). The client hook mirrors
  the unknown-error format (`reference/kit/packages/kit/test/apps/async/src/hooks.client.js:L4-L17`).
- Env: `.env` defines PRIVATE_STATIC/PRIVATE_DYNAMIC/PUBLIC_STATIC/PUBLIC_DYNAMIC
  (`reference/kit/packages/kit/test/apps/async/.env:L1-L5`); `src/env.ts` declares them with
  `defineEnvVars` from `@sveltejs/kit/env` (`reference/kit/packages/kit/test/apps/async/src/env.ts:L1-L20`).
  Remote modules import `$app/env/private` and `$app/env/public`
  (`reference/kit/packages/kit/test/apps/async/src/routes/remote/accessing-env.remote.js:L1-L14`).
- Per-session state helper `per_session(init)` keys a Map by `getRequestEvent().cookies.get('session')`
  (`reference/kit/packages/kit/test/apps/async/src/routes/remote/per-session.js:L10-L23`).
- Root `+error.svelte` prints `This is your custom error page saying: "<message>"` in `#message` and
  `page.status` in `h1`, with an `#error-home` link
  (`reference/kit/packages/kit/test/apps/async/src/routes/+error.svelte:L11-L15`).
- Root layout calls the shared test `setup()` that exposes `goto`, `invalidate`, `preloadData` etc. on
  `window` and adds `body.started` (`reference/kit/packages/kit/test/setup.js:L12-L28`). Index page lists
  every route from `$app/manifest` `routes` (`reference/kit/packages/kit/test/apps/async/src/routes/+page.svelte:L1-L12`).
- `app.html` sets `data-sveltekit-preload-data="hover"` on body
  (`reference/kit/packages/kit/test/apps/async/src/app.html:L8`).

## Wire-level facts the tests pin down (collected)

These are the only protocol details the tests assert directly; the full format lives in kit runtime
source, not in this segment.

- Remote calls go to URLs containing `/_app/remote/` and ending with `/<export_name>`
  (`reference/kit/packages/kit/test/apps/async/test/client.test.js:L28`, `:L76`, `:L142`;
  `reference/kit/packages/kit/test/apps/async/test/test.js:L99`, `:L142`).
- Query responses carry `cache-control: private, no-store`
  (`reference/kit/packages/kit/test/apps/async/test/client.test.js:L25-L32`).
- Commands are POSTs whose JSON body may contain `refreshes: string[]`; each entry contains a `/`
  (i.e. `<hash>/<name>...`); the response text contains `"type":"result"`
  (`reference/kit/packages/kit/test/apps/async/test/client.test.js:L787-L810`).
- Forged `x-sveltekit-pathname` / `x-sveltekit-search` headers on a replayed query request must not
  expose `event.url` (`reference/kit/packages/kit/test/apps/async/test/test.js:L133-L154`).
- Form actions post to the page URL with the existing query preserved and `&/remote=<id>` appended:
  attribute matches `/^\?existing=value&\/remote=/` and after a client-side query change
  `/later=updated&\/remote=/` (`reference/kit/packages/kit/test/apps/async/test/test.js:L172-L180`).
  Keys with a space or a slash (`echo.for('a b')`, `echo.for('a/b')`) must survive `?/remote=` parsing
  (`reference/kit/packages/kit/test/apps/async/src/routes/remote/form/native-result/+page.svelte:L5-L9`).
- Field naming: `.as('number')` yields `name^="n:number_field"`, `.as('checkbox')` yields
  `name^="b:checkbox_field"`, nested → `name^="object.array[0]"`, `name^="a.b.c"`
  (`reference/kit/packages/kit/test/apps/async/test/test.js:L995-L1000`, `:L695-L745`, `:L779`).
  Underscore-prefixed fields (`_password`) are excluded from the returned input on a native submission
  (`reference/kit/packages/kit/test/apps/async/test/test.js:L659-L670`).
- `getRequestEvent().isRemoteRequest` is true for enhanced (fetch) submissions and false for native
  full-page POSTs; the form handlers use it to hold submissions open only when enhanced
  (`reference/kit/packages/kit/test/apps/async/src/routes/remote/form/[test_name]/form.remote.ts:L49-L53`).
- A 403 returned from `handle` for a remote request surfaces as `query.error.status === 403`
  (`reference/kit/packages/kit/test/apps/async/test/client.test.js:L1057-L1070`).
- Over-limit `requested()` refresh rejection message:
  `400: Requested refresh was rejected because it exceeded requested(get_count, 0) limit`
  (`reference/kit/packages/kit/test/apps/async/test/client.test.js:L253-L255`).
- Guard messages: `Cannot access event.url in a query. Pass the value as an argument to the query
  instead` (also `params`, `route`) (`reference/kit/packages/kit/test/apps/async/test/test.js:L116-L121`);
  `Cannot set cookies in \`query\` or \`prerender\` functions | setHeaders is not allowed in remote
  functions` (`reference/kit/packages/kit/test/apps/async/test/test.js:L923-L925`);
  `Cannot call a command` from a load function or a GET handle hook
  (`reference/kit/packages/kit/test/apps/async/test/client.test.js:L445-L456`).
- Validation failure surfaces client-side as an `HttpError` with `e.body.message` equal to the schema
  issue message (`reference/kit/packages/kit/test/apps/async/src/routes/remote/validation/+page.svelte:L102-L106`).
- `refreshAll()` on `/remote` produces exactly 5 remote requests (queries used on the page plus the
  load-invoked one) (`reference/kit/packages/kit/test/apps/async/test/client.test.js:L476-L486`).

---

## 1. `/remote` — core query/command semantics

Files: `reference/kit/packages/kit/test/apps/async/src/routes/remote/query-command.remote.js`,
`.../remote/+page.js`, `.../remote/+page.svelte`, `.../remote/accessing-env.remote.js`.

Declares (`query-command.remote.js`):
- `echo = query('unchecked', v => v)`, `add = query('unchecked', ({a,b}) => a+b)` (L3-L4).
- `get_count = query(() => session().count)` and records the calling event in a WeakSet (L38-L41).
- `set_count = command('unchecked', async ({c, slow, deferred}) => ...)`: optional deferred wait
  (resolved by `resolve_deferreds`) or 500 ms sleep; sets count; then
  `for (const { query } of requested(get_count, Infinity)) await query.refresh()` (L43-L60).
- `set_count_refresh_all` uses `requested(get_count, Infinity).refreshAll()` (L62-L66).
- `get_flaky_count = query('unchecked', key => ...)` throws once when `key==='fail'` and the session
  flag is set (L68-L77); `set_count_partial_refresh` / `set_count_partial_refresh_all` set the flag
  and refresh via `requested(get_flaky_count, Infinity)` (L79-L97).
- `set_count_server_refresh`: `get_count().refresh()` from inside a command (L106-L110);
  `..._after_read`: await get_count() first, mutate, then refresh (L112-L117);
  `..._before_mutation`: refresh BEFORE mutating, refresh is deferred so it observes the new value
  (L119-L125); `..._then_reawait`: refresh, mutate, `await get_count()` must return fresh value
  (L127-L136); `set_count_server_set`: `get_count().set(c)` must NOT re-run get_count (L138-L147).

Page (`+page.svelte`): `+page.js` load calls `echo('Hello world')` (L1-L7 of `+page.js`); template
renders `{await count} / {count.current} ({count.loading})` in `#count-result`, `add({a:2,b:2})`,
flaky queries inside `<svelte:boundary>`, `set_count.pending` in `#command-pending`, and buttons for
every command variant, `count.refresh()`, `count.set(999)`, `.updates(get_count)`,
`.updates(get_count, count.withOverride(() => 6))`, `refreshAll()`, `resolve_deferreds()`
(`+page.svelte:L43-L175`).

Tests:
- Query returns data; `#echo-result` = `Hello world`, `#count-result` = `0 / 0 (false)` with JS
  (`reference/kit/packages/kit/test/apps/async/test/test.js:L7-L13`).
- `query.set` updates without any network request
  (`reference/kit/packages/kit/test/apps/async/test/client.test.js:L73-L82`).
- Hydrated data reused: zero remote requests on load
  (`reference/kit/packages/kit/test/apps/async/test/client.test.js:L84-L92`).
- Command alone does not refresh (1 request) (`...client.test.js:L258-L270`); `.updates(get_count)`
  single-flight (1 request, count updated) (`:L272-L286`); server-initiated refresh / set / refresh-
  after-read / refresh-before-mutation / refresh-then-reawait each = 1 request and count shown
  (`:L288-L358`); override + refresh shows optimistic `6 / 6` then real `5 / 5` (`:L360-L373`);
  partial refresh failures isolated (`ok:9`, `flaky refresh failed`) (`:L375-L389`);
  `requested().refreshAll()` (`:L391-L419`); `refreshAll` = 5 requests (`:L476-L486`);
  command pending 0→1→0 with deferred resolution (`:L488-L505`, `:L544-L564`).
- Serial `afterEach` resets count via `#reset-btn` (`...client.test.js:L63-L71`).

CSR-viable: **with-changes**. All mutation tests work in CSR; only "hydrated data is reused" (0
requests) becomes "N requests on mount" and `+page.js`'s `echo` runs in the browser (an extra
request). The `{await count}` SSR path disappears.

Go-side behaviour required:
- Per-call query endpoint returning devalue-serialised result; command endpoint that runs the
  handler, collects every `requested(...)`/`.refresh()`/`.set()` side effect, and returns the fresh
  query values in the same response (single flight) so the client makes exactly one request.
- `.refresh()` inside a command must be deferred until the command body finishes (before-mutation
  case), while an explicit `await get_count()` after a refresh must see the fresh value.
- `.set(value)` must populate the response without invoking the query.
- Per-query error isolation: a failing refresh produces an error record for that query only.
- Session cookie assignment in the Go equivalent of `handle`.

---

## 2. `/remote/batch`, `/remote/batch-ssr`, `/remote/batch-validation`, `/remote/batch-redirect` — `query.batch`

Declares:
- `get_todo = query.batch('unchecked', ids => { if dup ids throw; return id => id==='error' ?
  error(404,{message:'Not found'}) : todos.get(id) })`
  (`reference/kit/packages/kit/test/apps/async/src/routes/remote/batch/batch.remote.js:L12-L22`); commands
  `set_todo_title` (`.set`), `set_todo_title_server_refresh` (`.refresh`), `reset_todos`,
  `append_to_all_titles_requested` (`requested(get_todo, Infinity)` + `void query.refresh()`) (L24-L56).
- `batch-ssr` is the read-only twin (`.../batch-ssr/batch.remote.js:L10-L20`).
- `reverse = query.batch(v.pipe(v.string(), v.transform(reverse)), () => x => x)` — schema
  transform applied before the resolver (`.../batch-validation/batch.remote.js:L4-L12`).
- `batch_redirect = query.batch('unchecked', () => redirect(307, '/remote/batch-redirect#redirected'))`
  (`.../batch-redirect/data.remote.js:L4-L8`).

Pages: `/remote/batch` awaits ids `['1','2','1','error']` created at top level (comment: needed for
non-async mode) inside boundaries with ids `#batch-result-N`
(`.../batch/+page.svelte:L10-L32`). `/remote/batch-validation` renders `{await reverse(word)}` per word.

Tests:
- SSR renders final values and per-item 404 (`Not found`) with no `Loading todo` text
  (`reference/kit/packages/kit/test/apps/async/test/test.js:L15-L22`).
- Hydrated batch data reused (0 requests) (`...client.test.js:L104-L113`).
- `query.batch works`: results, duplicate ids deduped, refresh of 4 ids = 1 request
  (`...client.test.js:L583-L597`); `.set` no extra request (`:L599-L613`); server `.refresh` single
  flight (`:L615-L629`); `requested(...)` refresh all rolled into one batched call (`:L631-L651`).
- Redirect settles batched promises AND navigates (`#status` = `resolved`, URL has `#redirected`)
  (`...client.test.js:L653-L661`).
- Resolver always receives validated (transformed) args: `use the force` → `i am your father`
  (`...client.test.js:L681-L687`).

CSR-viable: **yes** for batch/batch-validation/batch-redirect; batch-ssr **with-changes** (drop the
0-request assertion; the 404 item still renders through the boundary after a client fetch).

Go-side: batch endpoint that receives an array of (deduplicated) args in one request, returns
per-item results or per-item error records (a 404 for one id must not fail the others), applies the
schema transform per item before the resolver, and treats a redirect thrown from the batch as a
redirect result that the client both follows and resolves.

---

## 3. `/remote/live*` — `query.live` streaming

Declares (`reference/kit/packages/kit/test/apps/async/src/routes/remote/live/live.remote.js`):
- `get_count = query.live(async function* () { signal = getRequestEvent().request.signal; ... yield
  count; while(true){ await wait_for_change(signal); if aborted return; if drop_next throw; yield } }
  finally { cleanup_count++ })` (L44-L72). Session captured once because `getRequestEvent()` may be
  unavailable after awaits (L46-L47).
- `get_finite_count` yields once and returns (L74-L78); `get_duplicate_payload` yields `{count}`
  objects (L80-L95).
- Commands: `increment`, `reset`, `notify_only`, `drop` (next tick throws `stream dropped`),
  `reconnect_live` (`get_count().reconnect()` from a command), `reconnect_requested_live`
  (`await requested(get_count, 5).reconnectAll()`), `reconnect_live_form = form('unchecked', ...)`
  calling `.reconnect()`; `get_stats` returns the session object (L97-L131).
- `live-terminal/data.remote.js`: `get_value` yields, then on trigger either `throw error(418,
  'terminal teapot')` or `redirect(307, '/remote/live-terminal/target')` (L37-L61).
- `live-ssr-value/data.remote.js`: first connection per key yields `'initial'` then returns; later
  connections block until `notify(key)` then yield `'updated'` (L10-L25).
- `live-error-seed/[key]/data.remote.ts`: first connection `error(418,'live teapot')`, later
  connections never yield (L7-L17). `live-nested-query/data.remote.js`: live generator awaits a
  nested non-exported-to-client query and yields the transformed value (L5-L11).
- Validation variants in `validation.remote.js`: `query.live(function*(arg){...})` and with schema (L22-L27).

Page `LiveView.svelte` shows `live.ready`, `live.connected`, `live.current`, `{await live}`, finite
`.done`, duplicate-update counter (`.../live/LiveView.svelte:L19-L31`); `+page.svelte` has
`for await (const value of get_count())` consumers and `refreshAll()` (`.../live/+page.svelte:L28-L80`).

Tests (all in `reference/kit/packages/kit/test/apps/async/test/client.test.js` unless noted):
- SSR renders first yielded value (`test.js:L24-L27`) — SSR-only.
- Streams updates and reconnects after `context.setOffline(true/false)` (L689-L710).
- Finite iterator: `#finite-done` true, `#finite-connected` false, no automatic reconnect; explicit
  `.reconnect()` increments server connection count (L712-L743).
- Reconnect from a server command (`cleanup_count` grows, still connected) (L745-L765).
- `requested(...).reconnectAll()` from a command: POST body has `refreshes` with `/` entries,
  response contains `"type":"result"`, no page errors, `requested_reconnect_count` > 0 (L767-L829).
- Form-triggered reconnect targets only `get_count`, not the finite query (L831-L860).
- Detach (unmount) works (L862-L869); unchanged devalue payloads are not re-sent (`#duplicate-updates`
  unchanged after `notify_only`) (L871-L882).
- Async-iterable joins the shared stream; first value emitted synchronously from cache (L884-L904);
  `refreshAll()` reconnects without orphaning `for await` consumers (L906-L928); `refreshAll()` settles
  while offline (L930-L943); server iterator cleaned up on reload (`cleanup_count` grows) (L945-L965).
- Mid-stream `HttpError` → `#error` = `418 terminal teapot`, `connected=false`, `done=true`, no
  reconnect (connection count unchanged) (L967-L990); mid-stream redirect navigates to
  `#redirect-target` (L992-L1002).
- SSR value reused on hydration (`initial` then `updated`) (`test.js:L949-L963`) — SSR-only.
- Hydrated live errors reused (`418: live teapot`) (L219-L226) — SSR-only.
- Nested query inside a live query is not serialised into the page (`test.js:L928-L935`).

CSR-viable: streaming/reconnect/finite/terminal/redirect/detach/dedupe/for-await tests — **yes**.
`live-ssr-value`, `live-error-seed`, "renders first yielded value during SSR" — **no** (they exist to
prove hydration seeding). `live-nested-query` — **with-changes** (assert the raw value is absent from
the stream response body instead of `page.content()`).

Go-side: a streaming response per live query (kit uses a long-lived connection the client can
reconnect), per-connection abort signal on client disconnect, a `done` marker for finite generators,
suppression of byte-identical consecutive payloads, terminal error and redirect frames, and a way
for commands/forms to request reconnects of specific live queries in the single-flight response.

---

## 4. `/remote/prerender*` — `prerender()` functions

Declares (`reference/kit/packages/kit/test/apps/async/src/routes/remote/prerender/prerender.remote.js`):
- Top-level `read(text)` of a `?url` asset must not throw while remote files are evaluated at build (L5-L8).
- `prerendered = prerender(() => 'yes')` throws if called at runtime in prod (`!building && !dev`) (L10-L16).
- `prerendered_entries = prerender('unchecked', x => x, { inputs: () => ['a','b','中文'], dynamic: true })`
  throws in prod for `a,b,c,中文` (`c` is discovered by prerendering a page) (L18-L31).
- `with_read = prerender(() => content)` (L33-L35).
- `prerender-dedupe/data.remote.js`: `counted = prerender(() => ++invocations)` used by two prerendered
  pages, must run once at build (L3-L7). `prerender-inline/data.remote.js`: `get_prerendered =
  prerender(() => 'hello')`. `prerender-in-query/data.remote.js`: `nested = query(() => prerendered())`
  — must be served from the prerendered response (L4-L5).
- Validation variants: `prerender(fn)` and `prerender(schema, fn, {inputs, dynamic})`
  (`.../validation/validation.remote.js:L29-L40`).

Pages: `/remote/prerender` buttons fetch `prerendered()`, `prerendered_entries('d')`, `with_read()`;
`whole-page/+page.js` sets `prerender = true` and loads a,c,中文,yes; `functions-only/+page.js` same
load without page prerender (`.../prerender/*`).

Tests:
- Prerendered entries not called in prod: `#prerendered-data` = `a c 中文 yes` for both whole-page and
  functions-only (`reference/kit/packages/kit/test/apps/async/test/test.js:L672-L680`).
- Prerendered entries use the prerender cache (2nd click = no request) while unlisted `d` refetches
  (`...client.test.js:L458-L474`); hydrated prerender data reused (`:L94-L102`).
- Queries can read prerendered data during SSR (`test.js:L156-L159`).
- Dedupe across prerendered pages, build only (`test.js:L965-L978`).
- Build artefacts (`...server.test.js`): no `dist/` written when treeshaking (L13-L16); non-dynamic
  prerender functions treeshaken from `.svelte-kit/output/server/chunks/prerender.remote.js` (L18-L24);
  colliding basenames treeshaken but still in sourcemaps (L26-L47); manifest has no duplicate remote
  module hashes (L49-L61).

CSR-viable: `/remote/prerender` button flow and `functions-only` — **with-changes** (the load runs in
the browser; the prerendered outputs must be static files the Go server serves). `whole-page` and
`prerender-dedupe` — **with-changes** (pages become shells; the client fetches the prerendered remote
output, and dedupe means both pages read the same static file). Build-artefact tests — **no** (they
inspect kit's JS server bundle, which skgo does not ship).

Go-side: at build time, evaluate `prerender()` functions for `inputs()` (with correct URL encoding of
non-ASCII args), write them as static responses under the remote path, and refuse (or fall back to
runtime when `dynamic: true`) for unlisted inputs; `prerender-in-query` means a Go query that calls a
prerendered function must read the static output rather than re-run it.

---

## 5. Redirects from queries — `/remote/query-redirect`, `/remote/redirect-external`

Declares: `layoutRedirect = query('unchecked', path => path !== target ? redirect(307, target) : path)`
and `pageRedirect = query(() => redirect(307, '/remote/query-redirect/redirected'))`
(`reference/kit/packages/kit/test/apps/async/src/routes/remote/query-redirect/redirect.remote.js:L4-L14`);
`redirectOutsideApp = query(() => redirect(307, '/robots.txt'))` — same-origin non-route
(`.../redirect-external/redirect.remote.js:L7-L10`).

Pages: links have `data-sveltekit-preload-data={false}` because preloading would eagerly redirect
(`.../query-redirect/+page.svelte:L1-L5`); the common layout awaits `layoutRedirect(page.url.pathname)`
(`.../from-common-layout/+layout.svelte:L8-L10`).

Tests:
- Redirect on page load from a layout query and from a page query, landing on `#redirected`
  (`reference/kit/packages/kit/test/apps/async/test/test.js:L29-L42`).
- Raw HTTP GET of `/remote/query-redirect/from-page` returns 307 with `location:
  /remote/query-redirect/redirected` and does not trigger `handleError` (`test.js:L66-L82`).
- Redirect to `/robots.txt` performs a full-page navigation (`test.js:L44-L64`).

CSR-viable: client-side navigations **yes**; the raw-HTTP 307 assertion **no** in CSR (the shell is
served with 200; the redirect happens when the query runs in the browser). Keep the test but assert
the query response encodes a redirect and the browser lands on the target.

Go-side: a redirect thrown in a query must be encoded in the query response (not as an HTTP 3xx that
`fetch` would follow), including same-origin non-route targets.

---

## 6. Errors from queries — `/remote/error-hydration`, `/remote/query-runtime-errors`, `/remote/transport-status`, `/remote/requested-limit`

- `failing = query(() => error(418, 'teapot'))`; `failing_batch = query.batch(v.string(), () => () =>
  error(418, 'batch teapot'))` (`.../error-hydration/data.remote.ts:L5-L13`). Pages render
  `q.error.status: q.error.message` via the reactive getter (`.../error-hydration/+page.svelte:L7-L9`;
  batch variant awaits inside a boundary because batch execution is deferred to a macrotask
  (`.../error-hydration/batch/+page.svelte:L7-L22`)).
- Tests: hydrated query errors reused (0 requests, `418: teapot`) and batch variant
  (`...client.test.js:L199-L217`). **no** (hydration seeding) — but the `error` getter shape
  (`{status, message}`) is what a CSR client shows after its own fetch, so keep the text assertions
  and drop the count.
- `query-runtime-errors/not-tracked`: a query created outside a tracking context can be awaited and
  `.current` read (`...client.test.js:L1022-L1037`). **yes**.
- `transport-status`: sets `deny-remote=1` cookie then `result.refresh()`; expects `#status` = `403`
  (`...client.test.js:L1057-L1070`). **yes**. Go-side: any non-2xx from the remote endpoint (including
  a hook-level rejection) must map to `query.error.status`.
- `requested-limit/[key]`: `bump = command(v.string(), key => { ...; requested(get_count, 0) })`
  (`.../requested-limit/[key]/data.remote.ts:L8-L14`); expects the 400 rejection message
  (`...client.test.js:L245-L256`). **yes**. Go-side: enforce the `requested(fn, limit)` cap and
  return a per-query error record.

---

## 7. Refresh graph semantics — `/remote/nested-refresh`, `/remote/re-refresh`, `/remote/refresh-cycle`, `/remote/query-set-inline`, `/remote/sidechannel-store`, `/remote/private-query`, `/remote/query-non-exported`

- `nested-refresh`: `get_a` calls `get_b().refresh()` when it runs; `bump` refreshes `get_a` → both
  `#a=5`, `#b=50` arrive in one command response (1 request)
  (`.../nested-refresh/data.remote.js:L17-L35`; `...client.test.js:L147-L165`). **yes**.
- `re-refresh`: a query re-refreshed by another query during the same drain is cached, not re-run
  (value stays 10 not 11) (`.../re-refresh/data.remote.js:L21-L38`; `...client.test.js:L167-L185`). **yes**.
- `refresh-cycle`: A refreshes B refreshes A; must terminate (`#value` = 1)
  (`.../refresh-cycle/data.remote.js:L17-L33`; `...client.test.js:L187-L197`). **yes**.
- `query-set-inline` (+ `/refresh`): `get_things` calls `get_thing(id).set(...)` (or `.refresh()`)
  for every id during SSR so the client reuses them with 0 requests
  (`.../query-set-inline/data.remote.js:L14-L35`; `...client.test.js:L115-L145`). **with-changes**: in
  CSR the parent query's response must inline the child values so that `get_thing(id)` on reveal
  makes 0 requests — assert 1 request total (the parent) instead of 0.
- `sidechannel-store/[key]`: `update` command calls `get_value(key).set('updated')` for a query with
  no active client resource; later `show` renders `updated` with 0 requests
  (`.../sidechannel-store/[key]/data.remote.ts:L8-L14`; `...client.test.js:L228-L243`). **yes**.
- `private-query`: non-exported `get_secret` used by exported `reveal`; response body contains
  `PRIVATE-DATA` but never `private-data` (`.../private-query/data.remote.js:L3-L10`;
  `test.js:L90-L110`, `...client.test.js:L663-L679`). **yes**.
- `query-non-exported`: two module-private queries `one`, `two` do not clobber each other (`h1` = 3)
  (`.../query-non-exported/data.remote.ts:L3-L10`; `test.js:L84-L88`). **yes** (in Go this is
  simply two private functions; the scenario guards against name-keyed registries).

Go-side: the single-flight collector must (a) keep sweeping until no new refresh/set entries appear,
(b) memoise a query once per drain to break cycles, (c) include `.set()` side-channel values even when
the client never requested them, and (d) only ever serialise exported functions.

---

## 8. Guards and event access — `/remote/event`, `/remote/query-event-guards`, `/remote/server-load-command`, `/remote/hook-command`

- `get_event` reads `event.url/params/route` and reports the thrown messages
  (`.../event/data.remote.ts:L4-L16`). Expected string in `test.js:L112-L131`; forged header replay in
  `test.js:L133-L154`. **with-changes** (the "direct navigation renders errors during SSR" half goes away).
- `blocked_operations`: `cookies.set` and `setHeaders` inside a query throw the two messages in
  `test.js:L920-L926` (`.../query-event-guards/data.remote.js:L4-L24`). **yes**.
- `server-load-command/+page.server.ts` calls a command from `load` → 500 with `Cannot call a command`
  (`...client.test.js:L445-L449`); GET to `/remote/hook-command` → 500 JSON `{error}` containing the
  same (`:L451-L456`); POST with `origin` header → 200 `{result:'action: from-hook'}` (`:L438-L443`).
  **yes** for the HTTP assertions; the `#message` assertion for the load case is SSR-rendered
  (**with-changes**: in CSR the `__data.json` request fails and the client error page shows it).

Go-side: query context must not expose the page URL/params/route; queries and prerenders must reject
cookie/header mutation; commands must be callable only from POST contexts (commands, form handlers,
endpoints, POST-method hooks) and rejected from loads/GET.

---

## 9. Validation — `/remote/validation`, `/remote/form/transform`, `/remote/reserved`

- `validation.remote.js` builds a Standard-Schema-V1 object by hand (`~standard.validate`) that
  rejects non-strings with `Input must be a string` (L3-L13), and declares no-arg and one-arg
  variants of `query`, `query.live`, `prerender` (with `inputs`/`dynamic`), `command`, and
  `query.batch` with/without validation (L15-L58).
- Page buttons: valid; extra arg when none expected → rejected; wrong type → `isHttpError(e) &&
  e.body.message === 'Input must be a string'` for every kind; more than one arg → only the first is
  sent (`.../validation/+page.svelte:L36-L187`). Test clicks all four buttons and expects `success`
  (`...client.test.js:L507-L522`). **yes**.
- `form/transform`: `get_transformed_data = query(v.pipe(v.number(), v.transform(String)), ...)`;
  `update_data` uses `requested(..., 1).refreshAll()`; `set_data` iterates `for await (const {arg,
  query} of requested(...))` and `query.set(...)` with the transformed arg
  (`.../form/transform/form.remote.ts:L7-L27`); tests `Count for 42 is 1` and `Set value for 42`
  (`...client.test.js:L1004-L1020`). **yes**.
- `reserved.remote.ts` exports `delete`, `class`, `return` (`.../reserved/reserved.remote.ts:L3-L8`);
  test expects `deleted/classy/42` (`...client.test.js:L524-L530`). **yes** — the generated TS stubs
  must handle reserved-word export names (`export { _delete as delete }` pattern).

Go-side: schema validation on every remote kind including batch items and live args; a schema's
output transform must be what the handler and `requested()` iteration see; validation failures return
an HttpError-shaped body whose `message` is the first issue; `'unchecked'` skips validation; only the
first argument is transmitted.

---

## 10. Transport and env — `/remote/transport`, `/remote/accessing-env`

- `greeting = query(() => new Foo('hello from remote function'))`; page calls `.bar()` → `h1` =
  `hello from remote function!` (`.../transport/data.remote.ts:L4`; `...client.test.js:L1051-L1055`). **yes**.
- `accessing-env.remote.js` throws at module load if `PRIVATE_DYNAMIC`/`PUBLIC_DYNAMIC` are unset
  (regressions #14219, #14439) and exports placeholder `q` imported by `/remote/+page.svelte` so it is
  not treeshaken (L5-L14; `+page.svelte:L19,L26`). **n/a** in Go (env access is ordinary Go), but
  the example app should keep a query that reads dynamic env to prove the Go side sees runtime env.

Go-side: devalue encoding of custom classes must consult the universal `transport` hook table; the
Go server serialises `Foo` as the encoder output (`[message]`) keyed by transport name.

---

## 11. Server integration — `/remote/server-endpoint`, `/remote/server-action`, `/remote-lib`, `/plain-lib`, `/remote/dev/preload`

- `+server.ts` GET awaits a query and POST awaits a command (`.../server-endpoint/api/+server.ts:L4-L12`);
  page fetches both (`...client.test.js:L421-L429`). **yes** if the endpoint is written in Go or stays
  in the sidecar; skgo: endpoints are not remote functions, so this is a Go `+server` equivalent.
- `+page.server.ts` `actions.default` calls `do_something(fields.get('input'))` (a `command(v.string())`)
  (`.../server-action/+page.server.ts:L4-L9`; `...client.test.js:L431-L436`). **yes** (form actions in
  CSR still POST to the server).
- `remote-lib`: a package re-exports a remote function from `data.remote.js`
  (`reference/kit/packages/kit/test/apps/async/_test_dependencies/remote-lib/index.js:L1`); test
  `...client.test.js:L49-L54`. `plain-lib`: a package file named `jwks/remote.js` is NOT a remote module
  (`.../plain-lib/jwks/remote.js:L1-L3`; `...client.test.js:L56-L59`). Go-side: only the `.remote.(js|ts)`
  suffix marks a remote module; the generator must not treat `remote.js` as one.
- `dev/preload`: dev-only; a page whose server load and component both import the same remote
  function with a valibot schema must preload correctly (`...client.test.js:L8-L23`). **no** (dev-mode
  analysis of kit's Vite plugin).

---

## 12. Forms — `/remote/form/*` (`form(schema, handler)`)

Central fixture `[test_name]/form.remote.ts`: `get_message = query(v.string(), ...)`; `set_message =
form(v.object({ test_name, id?, message: v.picklist([...], 'message is invalid'), uppercase?, action?:
picklist(['normal','reverse']) }), async data => { 'unexpected error' → throw; 'expected error' →
error(500,'oops'); 'redirect' → redirect(303,'/remote'); reverse or uppercase; get_message(test_name).set(msg);
if isRemoteRequest await deferred; return msg + (id ? ` (from: ${id})` : '') })` (L10-L57);
`resolve_deferreds = form(...)` (L69-L78). The page renders three forms: unscoped `{...set_message}`,
scoped `set_message.for('scoped:'+name)`, enhanced `.enhance(async form => { ... await
form.submit().updates(message.withOverride(...)) })`, plus `fields.message.as('text')`,
`fields.test_name.as('hidden', ...)`, two submit buttons via `fields.action.as('submit','normal'|'reverse')`,
and readouts of `.pending`, `.submitted`, `.result`, `.fields.message.value()`, `.element`
(`reference/kit/packages/kit/test/apps/async/src/routes/remote/form/[test_name]/+page.svelte:L1-L105`).

Scenario table (tests in `reference/kit/packages/kit/test/apps/async/test/test.js` unless noted):

| Route | Declares / page | Test | CSR |
|---|---|---|---|
| `form/basic-*` | see above | `form works`: action attr, query preserved, pending 1→0 after resolve, `submitted`, result `hello`, input cleared (L161-L203) | with-changes: drop `javaScriptEnabled=false` branch |
| `form/submitter` | `my_form = form(v.object({submitter: v.string()}))`; button `fields.submitter.as('submit','hello')` | `#result` = `hello` (L205-L211) | yes |
| `form/live-update` | same fixture | `fields.message.value()` tracks typing, cleared after submit (L213-L233) | yes |
| `form/validation-issues` | picklist message | `message is invalid` shown (L235-L242) | yes |
| `form/unexpected-error` | throw → handleError | root error page text with `(500 Internal Error, on /remote/form/unexpected-error)` (L244-L255) | yes |
| `form/expected-error` | `error(500,'oops')` | error page `"oops"` (L257-L264) | yes |
| `form/throwing-error-page` | child `+error.svelte` throws (`.../throwing-error-page/+error.svelte:L1-L3`) | falls through to root error page with `error page render error (500 ...)` (L266-L277) | yes (client boundary) |
| `form/redirect` | `redirect(303,'/remote')` | `waitForURL('/remote')` (L279-L286) | yes |
| `form/redirect-target` | `redirectForm = form(object({id: optional(string())}), () => redirect(303, .../destination))` (`.../redirect-target/form.remote.ts:L5-L12`); forms with `target=_blank`, none, `formtarget=_blank` input | popup opens for `_blank`, same tab otherwise (L288-L332) | yes |
| `form/multiple-submit` | two submit buttons | `set reverse message` → `sdrawkcab` (L334-L349) | yes |
| `form/form-scoped` | `.for('scoped:...')` | result `hello (from: scoped:form-scoped)` (L351-L370) | yes |
| `form/enhanced` (serial) | `.enhance(cb)`; `cb.element === form.element`; `submit()` returns boolean; imperative `submit()` | L372-L457 | yes |
| `form/preflight` | `set_number = form(v.object({number: v.pipe(v.number(), v.minValue(10,'too small'))}))`; page `preflight(schema maxValue(20,'too big'))` (`.../preflight/form.remote.ts:L10-L18`) | client issue `too big`, server issue `too small`, then success (L459-L490) | yes |
| `form/preflight-pending` | async valibot `checkAsync` schemas | `pending` is 1 immediately during async preflight, returns to 0; failing shows `async check failed` (L492-L527) | yes |
| `form/preflight-for` | `set_value.preflight(schema).for('a')` ordering bug | L529-L547 | yes |
| `form/preflight-only` | `set.validate({ preflightOnly: true })` on input, `set.validate()` on change | server issues preserved unless overridden by client issues (L549-L572) | yes |
| `form/validate` | `my_form(..., async (data, issue) => { if foo==='c' invalid(issue.foo('Imperative: ...')) })`; `my_form_2` `error(400,'Nope')`; `issue_path_form` nested path; `unmount_form` (`.../validate/form.remote.ts:L5-L48`) | blur-gated validation, `validate({all:true})`, imperative `invalid()`, `allIssues()` path `["nested","value"]` (L574-L616); unmount during validate no throw (L618-L630); issues cleared and `submit()` rejects on `error(400)` (L632-L657) | yes |
| `form/underscore` | `_password` field | native submit returns username, not `_password` (L659-L670; no-JS only) | with-changes: no-JS project does not exist in skgo; assert via enhanced result instead |
| `form/value` | nested/array schema | `fields.value()`, `.object.value()`, `.array.value()` snapshots (L682-L769) | yes (pure client) |
| `form/snapshot` | server holds submission until `release` form (`.../snapshot/snapshot.remote.ts:L6-L32`) | `fields.value()` snapshot immutable (L771-L796) | yes |
| `form/set-ssr` | `form.fields.set({description:'ssr'}); form.fields.description.set('nested')` in script | `#description` = `Description: nested` SSR-rendered (L798-L801) | with-changes (renders client-side identically) |
| `form/touched` | `touched()` per field, reset button | L803-L827 | yes |
| `form/select-untouched` | select keeps value when unrelated input changes | L829-L839 | yes |
| `form/file-upload` | `upload = form(v.object({text, file1: v.file(), file2: v.file(), read_files?}))` returns sizes or contents (`.../file-upload/form.remote.ts:L4-L25`) | small files echoed; 10 MB file size reported (L840-L886) | yes |
| `form/requested` | `submit = form('unchecked', async () => { await requested(get_time, 5).refreshAll(); return {message} })` (`.../requested/form.remote.ts:L5-L11`) | works enhanced and non-enhanced (L937-L947) | yes |
| `form/as-value` | `as_value_form = form(ValueSchema)` with `hidden` nested object; `.as('hidden', 1)`, `.as('hidden', true)`, `.as('number', v)`, `.as('select', v)`, `.as('color', v)`, `.as('range', v)`, `.as('checkbox', v)` (`.../as-value/Form.svelte:L8-L32`) | initial values (L988-L1009); updates after submit and after `field.set()` (`...client.test.js:L1081-L1123`); hidden typed values received server-side as string/number/boolean (`...server.test.js:L75-L84`, no-JS) | yes; hidden-value test with-changes (run enhanced) |
| `form/imperative` | `fields.message.set('hello'); await validate({all:true})` | DOM updated before validate (`...client.test.js:L532-L542`) | yes |
| `form/skip-submit` | enhance callback returns without `submit()` | pending sequence `0, 1, 0` then `0, 1, 0, 1, 0` (`...client.test.js:L566-L580`) | yes |
| `form/for-duplicate` | `increment.for(key).enhance(...)` with reactive spread | no duplicate requests; count 1..3 (`...client.test.js:L1072-L1079`) | yes |
| `form/noop-refresh-non-enhanced/[key]` | `set = form(..., async ({key}) => { await increment_count(key).refresh(); await get_count(key).refresh(); })` (`.../noop-refresh-non-enhanced/[key]/form.remote.ts:L14-L17`) | enhanced: refreshes to 1 (`...client.test.js:L1124-L1134`); native (no-JS): does NOT refresh, stays 0 (`...server.test.js:L63-L73`) | enhanced half yes; native half is the SSR contract (see gotchas) |
| `form/submit-field-value` (+ `page2`) | `fields.quantity.as('submit', 1|5)`; `my_form.enhance(...)` called in script | captured value equals clicked button, empty for no-value; page2 numeric input (`...client.test.js:L1136-L1157`) | yes |
| `form/native-result` | `echo = form(v.object({message: minLength(3,'too short')}))`; non-enhanced forms with `action={echo.action}` and keyed `.for('a b')`, `.for('a/b')` (`.../native-result/+page.svelte:L24-L38`) | result/issues/input survive the full-page POST and hydration; keyed result does not leak to unkeyed (`...client.test.js:L1159-L1208`) | with-changes (see gotchas: needs shell-embedded form state) |
| `form/updates-no-invalidate/[key]` | handler refreshes nothing; client `submit().updates()` | no implicit `invalidateAll` (count stays 0) (`...client.test.js:L1210-L1224`) | yes |
| `form/reset-id` | input `id="reset"` plus `type=reset` button (`.../reset-id/+page.svelte:L5-L9`) | issues cleared by reset; success clears input (`...client.test.js:L1226-L1241`) | yes |
| `form/reset-on-redirect` | `reset_form` returns or `redirect(303, url.href + 'foo')`; `redirect_form` redirects to another page (`.../reset-on-redirect/form.remote.ts:L5-L23`) | result cleared on redirect (`...client.test.js:L1338-L1347`); `submit()` resolves after redirect navigation (`:L1349-L1358`) | yes |

Go-side behaviour for forms:
- Parse `?/remote=<id>` where id may be `<hash>/<name>` plus an encoded `.for(key)` suffix containing
  spaces and slashes; preserve the page's own query string.
- Decode the typed field-name convention (`n:` number, `b:` boolean, dotted/bracketed nested paths,
  `_`-prefixed fields dropped from echoed input) into the schema input; accept `multipart/form-data`
  with files (10 MB).
- Return, for enhanced submissions: result, issues (with `path` arrays), refreshed query values
  (single flight), redirect, or error; honour `requested(fn, limit)`; distinguish enhanced from native
  via `isRemoteRequest`.
- For native submissions: run the handler, then serve the page again with the result/issues/input
  embedded so the client can hydrate them (kit does this during SSR; a CSR-only server must embed
  them in the shell payload — verify against kit's `ssr=false` rendering before promising it).
- Redirects use status 303; `error()` bodies render through the nearest error boundary and fall
  through boundaries that themselves throw.

---

## 13. Error boundaries and forks — `/server-error-boundary/*`, `/fork`

- `+page.svelte` throws during render; `nested/+error.svelte` shows `error.message | page.error?.message
  === error.message | page.status`; `layout-throws/+layout.svelte` throws, its sibling `+error.svelte`
  must NOT render; `async/+page.svelte` awaits a function that calls `error(404)`
  (`reference/kit/packages/kit/test/apps/async/src/routes/server-error-boundary/**`).
- Server tests: root error page text `render error (500 Internal Error, on /server-error-boundary)`;
  nested error page inside the still-visible `#nested-layout` with `| true | 500`; layout error skips
  the wrapped `+error.svelte` (`reference/kit/packages/kit/test/apps/async/test/test.js:L1012-L1038`).
- Client tests via `app.goto`: same three, plus navigating away tears down the stale `+error.svelte`,
  including after an async 404 and after `preloadData` created a fork (#15694)
  (`reference/kit/packages/kit/test/apps/async/test/client.test.js:L1244-L1336`).
- `fork`: hovering a preload link then `goto('/fork?key=value', {replace:true})` must not throw
  (`...client.test.js:L1361-L1370`; `.../fork/+page.svelte:L1-L16`).

CSR-viable: client-side variants **yes** (these are Svelte boundary semantics, not server behaviour);
the server-rendered variants **no**.

---

## Citations

- `reference/kit/packages/kit/test/apps/async/vite.config.js:L1-L33`
- `reference/kit/packages/kit/test/apps/async/package.json:L1-L32`
- `reference/kit/packages/kit/test/apps/async/src/hooks.server.js:L1-L61`
- `reference/kit/packages/kit/test/apps/async/src/hooks.js:L1-L9`
- `reference/kit/packages/kit/test/apps/async/src/hooks.client.js:L1-L17`
- `reference/kit/packages/kit/test/apps/async/src/env.ts:L1-L20`, `.env:L1-L5`
- `reference/kit/packages/kit/test/apps/async/src/routes/remote/query-command.remote.js:L1-L147`
- `reference/kit/packages/kit/test/apps/async/src/routes/remote/+page.svelte:L1-L175`, `+page.js:L1-L7`
- `reference/kit/packages/kit/test/apps/async/src/routes/remote/per-session.js:L1-L23`
- `reference/kit/packages/kit/test/apps/async/src/routes/remote/batch/batch.remote.js:L1-L56`
- `reference/kit/packages/kit/test/apps/async/src/routes/remote/live/live.remote.js:L1-L131`
- `reference/kit/packages/kit/test/apps/async/src/routes/remote/live-terminal/data.remote.js:L1-L71`
- `reference/kit/packages/kit/test/apps/async/src/routes/remote/prerender/prerender.remote.js:L1-L35`
- `reference/kit/packages/kit/test/apps/async/src/routes/remote/validation/validation.remote.js:L1-L58`
- `reference/kit/packages/kit/test/apps/async/src/routes/remote/form/[test_name]/form.remote.ts:L1-L78`
- `reference/kit/packages/kit/test/apps/async/src/routes/remote/form/[test_name]/+page.svelte:L1-L105`
- `reference/kit/packages/kit/test/apps/async/test/test.js:L1-L1038`
- `reference/kit/packages/kit/test/apps/async/test/client.test.js:L1-L1370`
- `reference/kit/packages/kit/test/apps/async/test/server.test.js:L1-L85`
- `reference/kit/packages/kit/test/apps/async/_test_dependencies/remote-lib/*`, `plain-lib/*`

## skgo implications

Copy these into the example app's Playwright suite first (each maps to a Go behaviour the server
cannot fake):

1. `/remote` single-flight matrix (`client.test.js:L258-L419`) — forces the command→refresh collector.
2. `/remote/batch` + `batch-validation` + `batch-redirect` (`client.test.js:L583-L687`) — batch
   endpoint, per-item errors, schema transform per item.
3. `/remote/live` streaming + `live-terminal` (`client.test.js:L689-L1002`) — streaming transport,
   abort on disconnect, done/terminal/redirect frames, payload dedupe.
4. `/remote/form/basic-*`, `form/validate`, `form/preflight*` (`test.js:L161-L203`, `L459-L657`) —
   `?/remote=` parsing, typed field names, issues with paths, single flight from forms.
5. `/remote/form/file-upload` (`test.js:L840-L886`) — multipart with 10 MB bodies.
6. `/remote/validation` (`client.test.js:L507-L522`) — validation for every kind, first-arg-only.
7. `/remote/private-query` + `live-nested-query` (`test.js:L84-L110`, `L928-L935`) — never serialise
   non-exported results.
8. `/remote/event` + `query-event-guards` + `server-load-command` + `hook-command`
   (`test.js:L112-L154`, `L920-L926`; `client.test.js:L438-L456`) — context guards.
9. `/remote/prerender` (`test.js:L672-L680`; `client.test.js:L458-L474`) — build-time prerendered
   remote outputs served as static files, `dynamic` fallback.
10. `/remote/transport` + `transport-status` (`client.test.js:L1051-L1070`) — transport hook encoding
    and hook-level 403 mapping.

Skip or rewrite: every `expect(request_count).toBe(0)` after `page.goto` (hydration reuse), the
`javaScriptEnabled=false` project, `server.test.js` bundle-inspection tests, `dev/preload`,
`live-ssr-value`, `live-error-seed`, `error-hydration`, `set-ssr`, "renders first yielded value
during SSR".

## Gotchas

- Async Svelte (`{await ...}` in templates, `$derived(await ...)`) is required by these pages; the
  example app must enable `compilerOptions.experimental.async`.
- In-memory state in the fixtures is per `session` cookie; the Go server must set that cookie in its
  hook equivalent or tests running in parallel workers will clobber each other (comment in
  `query-command.remote.js:L6-L12`).
- `getRequestEvent()` may be unavailable after an `await` inside a live generator (comment at
  `live.remote.js:L46`); capture per-request state before the first await.
- Native (non-enhanced) form POSTs assume the server re-renders the page with the result; with
  `ssr=false` this is the one form path whose CSR behaviour is not demonstrated by these tests.
- Preloading (`data-sveltekit-preload-data="hover"` on body) eagerly runs queries and can trigger
  redirects; the redirect pages opt out per link (`query-redirect/+page.svelte:L1-L5`).
- `hooks.server.js` imports a `.remote` module directly (`L2`) to call a command in the hook; in skgo
  the hook is Go, so the "command in hook" test becomes "command callable from a Go POST handler".
- Tests use `waitForTimeout(100)` after actions before asserting request counts; keep that idiom.
- `prerender.remote.js` uses top-level `await read(text)` — remote modules are evaluated at build time
  by kit; the Go generator must tolerate (or forbid) top-level awaits in `.remote.ts` stubs.

## Recipes

- Request-count assertion for a remote call:
  ```js
  let n = 0;
  page.on('request', (r) => (n += r.url().includes('/_app/remote') ? 1 : 0));
  await page.click('#btn');
  await expect(page.locator('#result')).toHaveText('...');
  await page.waitForTimeout(100);
  expect(n).toBe(1);
  ```
  (`reference/kit/packages/kit/test/apps/async/test/client.test.js:L262-L269`)
- Capture the wire body of a command: `Promise.all([page.waitForResponse(r =>
  r.url().includes('/name')), page.click(...)])` then `await response.text()` (`test.js:L98-L109`).
- Hold a submission open from the server so pending state can be observed: keep a
  `Promise.withResolvers()` list and release it from a second form/command
  (`form/[test_name]/form.remote.ts:L49-L53`, `L69-L78`).
- Per-session fixture state: `per_session(() => ({...}))` keyed by the `session` cookie
  (`remote/per-session.js`).
- Redirect that keeps the page alive so a promise can be observed settling: redirect to the same
  path with a hash (`batch-redirect/data.remote.js:L5-L7`).
