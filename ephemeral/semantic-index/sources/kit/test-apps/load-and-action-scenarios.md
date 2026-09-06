# Kit `basics` test app: load, streaming, serialization, actions, cookies, headers, CSRF, redirects, errors, routing

All paths are relative to the token cache root `/Users/tyler/src/skgo/ephemeral/inspiration/`.
Pinned `@sveltejs/kit` 3.0.0-next.25. This is a SKIM of
`reference/kit/packages/kit/test/apps/basics/src/routes/{load,streaming,serialization-basic,serialization-stream,actions,cookies,set-cookie,csrf,headers,immutable-headers,content-length-header,content-type-header,caching,redirect,redirect-on-load,errors,nested-layout,routing,encoded,match,shadowed,endpoint-input,endpoint-output,static,data-sveltekit,xss}`
with the tests that touch each route. Test files: `basics/test/test.js` (both projects),
`basics/test/client.test.js` (JS only), `basics/test/server.test.js` (no-JS only, mostly raw HTTP),
`basics/test/cross-platform/{test,client,server}.test.js` (same split, run separately by
`test:cross-platform:*` scripts — `reference/kit/packages/kit/test/apps/basics/package.json:L14-L15`).

## Purpose

Identify which page-load and form-action behaviours kit guarantees at the HTTP level (things a Go
server must reproduce byte-for-byte: `__data.json`, action responses, cookies, CSRF, redirects,
error JSON, HEAD/OPTIONS/405, streaming) versus client-router behaviours that come for free with
kit's client, and cite the test that proves each.

## App-wide facts

- Config: custom in-file adapter with `emulate().platform`, `supports.read/instrumentation`;
  `experimental.remoteFunctions: true`; `csrf.trustedOrigins: ['https://trusted.example.com',
  'https://payment-gateway.test']`; `prerender.entries` incl. trailing-slash variants;
  `router.resolution` from `ROUTER_RESOLUTION` (client|server); `SVELTE_ASYNC` toggles async
  compiler (`reference/kit/packages/kit/test/apps/basics/vite.config.js:L18-L86`).
- `hooks.server.js` `handle` is a `sequence` that: tags tracing test ids; sets `locals.key/params/answer`;
  asserts `__data.json` requests have the suffix stripped and `isDataRequest` set; asserts SSR
  sub-requests have `isSubRequest` set (no `user-agent`); appends `set-cookie: name=SvelteKit;
  path=/; HttpOnly` to every response; special-cases `/errors/error-in-handle`,
  `/redirect/in-handle` (throw vs raw 307 Response vs cookie delete), `/actions/redirect-in-handle`
  POST → 303, prerendered endpoint guard, `getRequestEvent()` identity check, and `preload`
  filtering (`reference/kit/packages/kit/test/apps/basics/src/hooks.server.js:L77-L223`).
- `handleError` writes `{path, kind, error}` to `test/errors.jsonl` and formats messages as
  `${detail} (${status} ${message})`, returning `error` verbatim for `kind === 'app'`, overriding
  status for `/errors/handle-error-status` (404) and `-fallback` (503), and `{}` for `*404-fallback`
  (`reference/kit/packages/kit/test/apps/basics/src/hooks.server.js:L33-L79`).
- `handleFetch` rewrites `/server-fetch-request.json` → `-modified.json` (`:L225-L234`); `init()` hook (`:L236-L238`).
- Universal `hooks.js`: `reroute` (async via `fetch`, error-handling cases) and `transport` for `Foo`
  (`reference/kit/packages/kit/test/apps/basics/src/hooks.js:L12-L54`). Client `handleError` mirrors
  the server format (`reference/kit/packages/kit/test/apps/basics/src/hooks.client.js:L6-L28`).
- `app.html` has `%sveltekit.env.PUBLIC_THEME%` on body and an anchor outside the app target
  (`reference/kit/packages/kit/test/apps/basics/src/app.html:L1-L14`); `error.html` prints
  `Error - %sveltekit.status%` and the message (`.../src/error.html:L1-L10`).

## Route groups

### `load/` — universal and server loads, `fetch` in load (server-protocol: HIGH)

What it exercises (route ids from the tree): `[dynamic]`, `accumulated`, `cache-control/{default,
force,bust}`, `change-detection`, `devalue/regex`, `dynamic-import-styles`, `fetch-abort-signal`,
`fetch-arraybuffer-b64`, `fetch-asset`, `fetch-body-stream-b64`, `fetch-cache-control/{b64,
headers-diff,load-data}`, `fetch-credentialed`, `fetch-external-no-cookies`, `fetch-external-non-canonical`,
`fetch-no-body`, `fetch-origin-{external,internal}`, `fetch-relative`, `fetch-request*`,
`fetch-response-headers`, `fetch-same-url`, `invalidation/**`, `large-response`, `mutated-url`,
`no-server-load`, `parent/{server,shared}/[x]/[y]/[z]`, `props`, `raw-body`, `relay`, `serialization*`,
`server-data-nostore`, `server-data-reuse`, `server-fetch-request`, `server-log-search-param`,
`set-cookie-fetch`, `static-file-with-hash`, `unchanged`, `unchanged-parent`, `url-hash`,
`url-query-param`, `url-to-string`, `window-fetch/*`.

Tests (`reference/kit/packages/kit/test/apps/basics/test/test.js`):
- `loads` (L239-L243); GET fetches serialized into `<script data-sveltekit-fetched data-url=...>`
  with payload `{"status":200,"statusText":"","headers":{},"body":"..."}` and no client refetch
  (L244-L266); empty nodes removed (L267-L271); POST fetches serialized with `data-hash` (L272-L321);
  non-string bodies not serialized / no collision (L322-L348); arraybuffer and body streams serialized
  base64 (L349-L392); relay JSON string (L393-L397); static data preferred over endpoint (L398-L402);
  data inherited through `parent()` for shared and server chains, `y: 'b edited'`
  (L403-L433; `reference/kit/packages/kit/test/apps/basics/src/routes/load/parent/server/[x]/[y]/[z]/+page.server.js:L1-L8`);
  fetch accepts Request (L434-L439); relative URL resolution (L440-L446); large responses (L447-L453);
  external API via `start_server` (L454-L479); credentialed internal fetches (L480-L490); page request
  headers forwarded (L491-L523); rawBody as DataView/string/Uint8Array (L524-L547); server-side fetch
  respects set-cookie (L548-L563); dynamic import CSS in SSR (L564-L572); layout data accessible with
  and without page load (L573-L587); devalue serialises RegExp (L588-L594); AbortSignal with internal
  fetch (L645-L652); response without body (L653-L659).
- Client (`reference/kit/packages/kit/test/apps/basics/test/client.test.js`): cross-origin
  non-canonical URLs not refetched (L44-L87); load only re-runs when tracked inputs change, `invalidate`
  by URL and custom key, `refreshAll` (L88-L129); `url.hash` access error in dev (L130-L137);
  server data reuse rules (L143-L192); fetch `cache-control` max-age honoured / `cache: 'force'` /
  bust on non-GET (L193-L230); `__data.json` has `cache-control: private, no-store` (L231-L241);
  cache keyed by body hash and headers (L242-L312); same URL fetched multiple times (L313-L322);
  3rd-party fetch patching (L323-L355, L373-L405); no hydration refetch with Request object
  (L356-L372); no `__data.json` when no server load (L406-L420).
- Invalidation (`client.test.js:L511-L921`): `+layout.server.js` isolation vs `await parent()`
  (L512-L542); searchParams tracking universal/server, value-count changes (L543-L587); forced
  invalidation after `POST .../reset-states` (L588-L632); concurrent and batched invalidations
  (L633-L656); `refreshAll` through redirects (L657-L662); `depends()` server and shared keys, also via
  `goto` (L663-L811); params and `route.id` tracking (L812-L853); `page.url` mutation (L854-L861);
  invalidate-then-goto and stale-data races during navigation (L862-L921).
- Server (`reference/kit/packages/kit/test/apps/basics/test/server.test.js`): missing resource in root
  layout does not hang (L788-L794); `#` in fetched filename (L795-L799); universal-load assets readable
  on the server (L800-L804); no `accept-language` forwarded when absent (L805-L816); `origin` header on
  non-GET internal and external requests (L817-L840); `OPTIONS /load` → 204 `allow: GET, HEAD, OPTIONS`,
  `OPTIONS /actions/enhance` → `allow: GET, HEAD, OPTIONS, POST` (L841-L861); logging search params (L862-L869);
  cookies not forwarded to external domains (L42-L59).

Go must: implement `__data.json` for server loads with devalue encoding (RegExp, custom transport
types), `private, no-store`, `x-sveltekit-invalidated` handling, `parent()` data inheritance across
layout/page nodes with empty nodes elided; expose `event.fetch` semantics only if server loads in Go
call internal routes (cookie forwarding to same-origin only, `origin` header on non-GET); answer
`OPTIONS` with 204 and an accurate `allow` list. The universal-load `<script data-sveltekit-fetched>`
serialisation is SSR-only and does not apply to skgo.

### `streaming/` — promises returned from load (server-protocol: HIGH for `__data.json`)

Routes: `universal` (universal load returns nested promises), `server` (server load returns
`lazy.success` after 1 s and `lazy.fail` rejecting), `server-error` (`fetch('http://localhost:1337/')`
rejected before render), `server/delayed-rejection` (layout delays 100 ms; page returns a
pre-rejected promise), `server/fast-n-slow` (layout fast promise, page awaited `ssrd` + streamed),
`discarded-promise` (layout returns a promise, page load calls `error(404)`)
(`reference/kit/packages/kit/test/apps/basics/src/routes/streaming/**`).

Tests: client nav renders eager text, loading placeholders, then `success` / `fail (500 Internal
Error)` / `delayed rejection (500 Internal Error)` (`reference/kit/packages/kit/test/apps/basics/test/client.test.js:L1475-L1524`);
direct-hit streaming only in DEV because `vite preview` buffers (L1525-L1586); discarded promise
does not hang the request (`reference/kit/packages/kit/test/apps/basics/test/server.test.js:L1648-L1661`);
serialization works with streaming (`reference/kit/packages/kit/test/apps/basics/test/test.js:L1701-L1709`).

Go must: stream `__data.json` (chunked) so top-level promise values arrive later as separate devalue
chunks; encode rejections as errors with the handleError-formatted message; never hang when a
streamed promise is discarded because the page load threw. In CSR every hit is the "client nav" case.

### `serialization-basic/`, `serialization-stream/` — transport hook (HIGH)

Server loads return `new Foo('...')` and `Promise.resolve(new Foo(...))`
(`reference/kit/packages/kit/test/apps/basics/src/routes/serialization-basic/+page.server.js:L1-L5`,
`.../serialization-stream/+page.server.js:L1-L8`); pages call `.bar()`. Tests: `It works!` on load
and `Client-side navigation also works!` after `clicknav` (`test.js:L1671-L1677`); streamed variant
(`test.js:L1701-L1709`). Go must encode custom types via the `transport` table in `__data.json`,
including inside streamed chunks.

### `actions/` — form actions (HIGH for the action wire protocol)

Routes: `enhance` (actions `login`, `register`, `slow`, `submitter`, `error` → `error(400)`, `echo`,
`counter` via cookie, `send_file`; load reads `enhance-counter` cookie —
`reference/kit/packages/kit/test/apps/basics/src/routes/actions/enhance/+page.server.js:L1-L68`),
`enhance-non-action-response`, `file-without-enctype`, `form-errors` (`fail(400, {errors})` —
`.../form-errors/+page.server.js:L4-L8`), `form-errors-persist-fields`, `form-errors/adjacent-error-boundary`,
`invalidate-all`, `redirect` (`redirect(303, '/actions/enhance')`), `redirect-in-handle`,
`success-data`, `update-form`, `cross-page/{source,destination,redirected,same-page,refreshall-false}`
(`destination` actions `success|failure|redirect|error`; load throws if `throw-in-load` — proves load
does not run for action errors — `.../cross-page/destination/+page.server.js:L1-L31`).

Tests (`reference/kit/packages/kit/test/apps/basics/test/test.js`): `invalidateAll` false/true
(L1095-L1120); file input without enctype errors in dev (L1121-L1135); error props (L1136-L1144);
fields persisted on failure (L1145-L1163); success data for multipart and urlencoded (L1164-L1185);
`applyAction` updates/redirects/errors (L1186-L1243); `use:enhance` variants: non-ActionResult
response renders error page, abort controller, `formAction`, dialog, button name, `formenctype`,
default content type `application/x-www-form-urlencoded`, no clear on second submit (L1244-L1401);
redirect wire format `{type:'redirect', location:'/actions/enhance', status:303}` with JS vs raw 303
+ `location` without JS (L1402-L1447); `page.status` 400 after error action (L1448-L1458); error
at correct boundary level (L1459-L1465); cross-page actions: success/failure/redirect/error, URL
`?/success` kept only for native submissions (L1500-L1569).
Client-only (`client.test.js:L1588-L1733`): page state after action; cross-page navigation is
client-side (`nav_marker` survives); same-page action drops omitted query params; reordered query is
"semantically identical" and refreshes without navigating; failures do not refresh by default;
`update({ navigate, refreshAll })` options; history entry pushed; named-action param stripped.
Server-only (`server.test.js`): `Action can return undefined` → JSON `{type:'success', status:204,
location:'/shadowed/simple/post'}` (L919-L934); failure includes stripped landing `location` and
devalue-encoded `data` string (L936-L955); `fail()` status mirrored in HTTP status (L957-L972);
POST to a page without actions → 405, JSON `{type:'error', location, error:{message:'Method Not
Allowed (405 Method Not Allowed)', status:405}}` (L660-L685); actions fall back when sibling
endpoint has no POST (L886-L901).

Go must: accept `POST <page>?/<name>` (urlencoded and multipart), return the action JSON envelope
(`type: success|failure|redirect|error`, `status`, `location`, `data` as a devalue string) for
`accept: application/json`, and a 303 with `location` for native submissions; set cookies from
actions; run the page's server load after a successful action but not after an error; 405 with
`allow` for methods not handled; 415 for `application/json` bodies (L1466+ in test.js, "submitting
application/json should return http status code 415").

### `cookies/`, `set-cookie/`, `headers/` — cookie API and multi-value headers (HIGH)

- `cookies/set` and `/delete` endpoints set/delete `cookiesAPITest` with `path: '/cookies'` then
  303 to `/cookies` (`reference/kit/packages/kit/test/apps/basics/src/routes/cookies/set/+server.js:L5-L8`);
  `set-more-than-one` and `nested/a` use `path: ''`; `encoding/set` stores `teapot, jane austen`;
  `enhanced/basic` actions set/delete a cookie and return a timestamp
  (`.../cookies/enhanced/basic/+page.server.ts:L6-L19`); `serialize` uses
  `cookies.serialize()` appended manually in `handle` (`hooks.server.js:L120-L131`);
  `forwarded-in-etag*`, `collect-without-re-escaping`.
- Tests (`test.js:L1598-L1670`): sanity, set, delete, more than one, default encoding, not decoded
  twice (`teapot%2C%20jane%20austen`), set in `+layout.server.js`, enhanced actions, path scoping
  (`/cookies/nested/a` sets, `/b` and `/cookies` do not see it). Cross-platform: etag forwards cookies
  (`reference/kit/packages/kit/test/apps/basics/test/cross-platform/client.test.js:L1359-L1379`).
  Server: `set-cookie` endpoint emits three separate `set-cookie` headers including one whose value
  contains commas (`server.test.js:L377-L392`; `reference/kit/packages/kit/test/apps/basics/src/routes/set-cookie/+server.js:L1-L8`);
  `setHeaders` allows multiple set-cookie from layout and page loads (`server.test.js:L1029-L1037`;
  `.../headers/set-cookie/**`); `cookies.serialize` correct (`server.test.js:L1039-L1048`);
  `Headers` instance accepted in fetch (`cross-platform/test.js:L543-L549`); external domains never
  receive cookies (`server.test.js:L42-L59`).

Go must: implement `cookies.get/set/delete/serialize` with path defaults (`path: ''` = current
path), URL-encoding of values (encode once, decode once), multiple `set-cookie` headers emitted
separately (never joined with commas), cookies settable from layout loads, page loads, actions,
endpoints and hooks, and visible to later loads in the same request.

### `csrf/` — origin check (HIGH; build mode only)

Endpoint handles GET/POST/PUT/PATCH/DELETE returning `ok`
(`reference/kit/packages/kit/test/apps/basics/src/routes/csrf/+server.js:L1-L24`). Tests
(`server.test.js:L62-L212`): POST/PUT/PATCH/DELETE with content types `application/x-www-form-urlencoded`,
`multipart/form-data`, `text/plain`, `text/plaiN` and wrong/missing origin → 403 body
`Cross-site ${method} form submissions are forbidden`; same origin allowed; `trustedOrigins`
exact-match only (`trusted.example.com.evil.com` and `evil.trusted.example.com` blocked); GET
always allowed; `application/json` allowed regardless of origin; missing origin blocked.
options-2 adds the check with a custom `NODE_ENV` (`reference/kit/packages/kit/test/apps/options-2/test/test.js:L240-L256`).
Go must reproduce the exact status, body text, method set, case-insensitive content-type prefix
match, and exact-origin trusted list.

### `immutable-headers/`, `content-length-header/`, `content-type-header/`, `caching/` (MEDIUM)

- `+server.js` returns a Response whose headers have `append`/`set` nulled to simulate undici
  (`reference/kit/packages/kit/test/apps/basics/src/routes/immutable-headers/+server.js:L1-L7`); test
  expects 200 `foo` (`server.test.js:L1058-L1063`).
- Empty pages: `content-type: text/html` on documents (`server.test.js:L23-L28`); `content-length`
  > 1000 when not encoded (`server.test.js:L30-L40`).
- `caching/+page.js` and `caching/server-data/+page.server.js` call `setHeaders({'cache-control':
  'public, max-age=30'})`; page response carries it (`server.test.js:L16-L21`) and so does the
  `__data.json?x-sveltekit-invalidated=01` response (`client.test.js:L23-L35`).
- Misc: `/_app/version.json` not `immutable`; `x-sveltekit-version` only on data responses
  (`server.test.js:L1050-L1078`).

Go must: honour `setHeaders` from loads on both the document and `__data.json` responses; set
`content-type: text/html` (no charset in this assertion) and `content-length` on shells; tolerate
immutable upstream header objects.

### `redirect/`, `redirect-on-load/`, `encoded/redirect` (HIGH)

Routes: `a` → `redirect(307, './b')` → `c`; `loopy/a` ↔ `b`; `missing-status/a` uses
`redirect(undefined, ...)`; `in-handle` handled by hook (`?throw`, `?response`, `?location=`,
`?cookies`); `package` redirects from an imported package; `redirect-on-load` redirects only in the
browser (`reference/kit/packages/kit/test/apps/basics/src/routes/redirect/**`,
`.../redirect-on-load/+page.js:L1-L10`).
Tests (`reference/kit/packages/kit/test/apps/basics/test/cross-platform/test.js:L549-L706`): relative
redirect resolves to `/redirect/c` and back button returns; redirect loop → `Redirect loop (500
Internal Error)` with JS, browser error page without; missing/invalid status → `Invalid status code`
(dev or no-JS) else `Redirect loop`; `redirect-on-load` lands on `/redirect-on-load/redirected` with JS;
redirect Response and thrown redirect from `handle`; `\r\n` in a redirect location → 500 without
crashing the server (L667-L679); cookies set/deleted alongside a redirect in handle, `__data.json`
response carries `set-cookie` (L680-L699). Encoded: redirects do not re-encode the location string,
also during SSR (`test.js:L107-L132`).

Go must: resolve relative redirect targets against the request URL, reject invalid status and
header-injection locations with 500, emit redirects for `__data.json` requests in the data envelope
(the client follows them), preserve `set-cookie` on redirect responses, and not double-encode
non-ASCII locations.

### `errors/` — error pages, handleError, endpoint errors (HIGH)

Routes: `serverside`, `module-scope-{client,server}`, `invalid-load-response`,
`invalid-server-load-response` (load returns an array), `load-server` (throws), `load-error-server`
(`error(555,'Not found')`), `load-error-string-server`, `load-error-server/layout-data`,
`load-status-without-error-client`, `handle-error-status`, `kind/{expected,unexpected}`,
`endpoint`/`endpoint.json` (throws), `endpoint-shadow`, `endpoint-shadow-not-ok`,
`endpoint-throw-error` (`error(401,'You shall not pass')`), `endpoint-throw-redirect`,
`invalid-route-response` (returns a string), `init-error-endpoint`, `error-html/make-root-fail`,
`error-in-handle`, `error-in-layout`, `missing-actions`, `nested-error-page`, `page-endpoint/*`
(FancyError preservation), `stack-trace`, `load-error-page-options/csr`
(`reference/kit/packages/kit/test/apps/basics/src/routes/errors/**`).
Tests: cross-platform `test.js:L230-L542` (server-side load errors with `footer` = `Custom layout`,
404 message `Not Found (404 Not Found)` with `read_errors` kind `framework`, 555 status preserved,
layout data present on error page, handleError status override 404, expected (`kind: 'app'`) vs
unexpected (`kind: 'unknown'`) errors, endpoint errors with stack traces, not-ok shadow endpoint
555, page-endpoint GET/POST error message preservation); cross-platform `client.test.js:L751-L836`
(client-side load errors, `error.html` fallback for root errors with `Error - 500` / `Error - 401`,
root 404 redirect via root layout, client `handleError` kinds `client app` / `client unknown`);
`server.test.js:L529-L785` (503 fallback for `x-sveltekit-error: true` sub-requests, invalid route
response 500 text, 405 `PUT method not allowed`, module evaluation error 500, malformed URI 400,
lightweight `Not Found` 404 with `vary: Sec-Fetch-Dest` for `sec-fetch-dest: image` bypassing
`handleError`, full error page for document/fetch destinations, stack traces not fixed twice,
`error()` in endpoint → HTML via `error.html` when `accept: text/html` else JSON `{message, status}`,
redirect in endpoint opaque to browser, POST to missing actions 405 JSON envelope, errors thrown in
`handle` → HTML or JSON `{message:'Error in handle (500 Internal Error)', status:500}` without
`stack`, error page respects `csr` option, root layout data for missing-route `__data.json`).
`nested-layout` tests: errors render in the right layout, deeply nested `+error.svelte` shows
`status` / `message`, layout reset `+layout@.svelte`, closest error page wins (`test.js:L660-L722`).

Go must: distinguish expected (`error()`, `kind: 'app'`), framework (404/405, `kind: 'framework'`)
and unknown errors; call a `handleError` equivalent (skipped for lightweight subresource 404s, which
must add `vary: Sec-Fetch-Dest`); serve `error.html` for HTML-accepting endpoint errors and JSON
`{message, status}` otherwise; mirror `error()` status codes (401/403/555) on the response; never
leak `stack`; return root-layout data for unknown-route data requests so the client error page can
render inside the root layout.

### `routing/`, `encoded/`, `match/`, `nested-layout/`, `shadowed/` (MEDIUM; mostly client router)

- `routing/`: static, dynamic `[slug]`, `ambiguous/[slug]` vs `[slug].json`, rest `[...rest]` with
  `deep`, `complex/[...parts].json`, `prefix-[...parts]`, `non-greedy`, matchers `[letter=lowercase]`
  etc., `split-params/[a]-[b]`, `content-negotiation` (`+server.js` for all methods beside a page with
  actions — `.../routing/content-negotiation/+server.js:L1-L24`), `params-in-handle/[x]`,
  `trailing-slash{,-server}/{always,never,ignore}`, `prerendered/trailing-slash/*`, hashes, focus,
  form-get, form-target-blank, cancellation, preloading, symlink, `clobbered-node-name`, `next-paint`.
  Tests: cross-platform `test.js:L707-L1086` (trailing-slash redirects `/routing/` → `/routing` keeping
  query, static/dynamic/rest/non-greedy/matcher routing, server routes not client-navigated,
  `data-sveltekit-reload`, reserved word route names, hash focus/`:target`, `?port=` external URLs
  ignored, cancellation, `page.route.id`, symlinks, server trailing-slash config); `test.js:L778-L808`
  (`$app/manifest` routes and capabilities), `L849-L901` (`%sveltekit.assets%` relative during SSR,
  `match()` on server and client), `L1077-L1094` (matchers); `server.test.js:L349-L376` (content
  negotiation: `accept: application/json` → endpoint `GET`, `text/html` → page), `L870-L877`
  (`event.params` in handle), `L886-L917` (actions fallback when endpoint lacks POST; Vite trailing
  slash redirect keeps query for prerendered pages); `client.test.js:L1306-L1331` (content
  negotiation + `use:enhance` uses the action, not the POST handler), `L2648-L2723`.
- `encoded/`: non-ASCII route dirs `苗条`, `反应`, `@[username]`, `[slug]`, `escape-sequences/[x+23]`
  etc., `endpoint` returning emoji JSON. Tests `test.js:L59-L178`: doubly encoded space/slash,
  encoded bracket, non-ASCII redirects, JSON emoji serialisation, `[x+2f]`-style escape sequences.
- `match/`: `match()` from `$app/paths` on server load and client (`.../match/+page.server.js:L1-L12`;
  `test.js:L872-L901`).
- `shadowed/`: `+page.server.js` loads and actions: `simple` (locals), `redirect-get(-with-cookie)`,
  `redirect-post(-with-cookie)`, `post-success-redirect`, `error-get`/`error-post`, `same-render`,
  `no-get`, `missing-get`, `dynamic/[slug]`, `parent`, `serialization` (non-POJO → 500 in dev).
  Tests `cross-platform/test.js:L62-L230`: GET/POST redirects with cookies visible to the browser,
  4xx/5xx from GET render the error page, non-GET error merges bodies with status 400, endpoint sees
  the full URL with query, parent data merging with `data` field, invalidation on slug change.
- `nested-layout/`: see errors section (`test.js:L660-L722`).

Go must: match kit's route manifest semantics for the server side (static > dynamic > rest, matchers,
trailing-slash policy incl. 308 redirects for prerendered pages that keep the query string,
decoded/encoded param handling with `%2f` and escape-sequence directories), content negotiation
between `+server` and `+page` at the same path (`accept`, and `x-sveltekit-page: true` header on HEAD
of a page), `event.params` available to hooks, `match()`-compatible route ids.

### `endpoint-input/`, `endpoint-output/`, `static/` (HIGH for method handling)

- `endpoint-input/sha256` reads the body slowly via `request.body.getReader()`
  (`reference/kit/packages/kit/test/apps/basics/src/routes/endpoint-input/sha256/+server.js:L1-L20`);
  test PUTs 256 KB and compares digest (`server.test.js:L416-L423`).
- `endpoint-output/`: `OPTIONS` handler; `body` (`json({})`); `fallback` (`GET` + `fallback`);
  `fallback-with-page`; `head-handler` (GET + HEAD with `x-sveltekit-head-endpoint`); `head-write-error`
  (invalid header char → 500); `post-only-with-page`; `query` (`QUERY` method); `stream`
  (256 KB `ReadableStream` with `digest` header); `stream-throw-error`; `stream-typeerror`;
  `fetch-asset/{absolute,relative}`; `actions-with-endpoint` (`.../endpoint-output/**`).
  Tests `server.test.js:L215-L527`: HEAD equals GET headers minus body; 405 with `allow` incl. HEAD
  and no duplicates; page served for GET/HEAD when endpoint lacks them (`x-sveltekit-page: true` on
  HEAD), POST still hits endpoint; `fallback` used instead of page; content negotiation; multiple
  set-cookie; binary stream body with `digest`; stream cancel with TypeError; slow body read;
  invalid header → 500 `ERR_INVALID_CHAR`; OPTIONS; HEAD; catch-all for arbitrary methods (`MOVE`);
  `QUERY` handler and `allow: GET, QUERY, HEAD`; assets via absolute/relative `fetch`.
- `static/+page.svelte` collides with the `static/` directory; `/static/static.json` must not be
  served from source (`server.test.js:L993-L997`; 403 in dev, 404 in build).

Go must (for any endpoint written in Go): auto-HEAD from GET, `allow` header computation including
`fallback` and `QUERY`, 405 for unhandled methods, streaming request and response bodies, per-header
validation errors → 500, `OPTIONS` handler support, page-vs-endpoint precedence rules by method and
`accept`.

### `data-sveltekit/`, `xss/` (client router; XSS is HIGH for the shell)

- `data-sveltekit-preload-{code,data}` with `hover|tap|eager|viewport`, `reload`, `reset`,
  `replacestate` attributes; offline preload failures do not navigate
  (`reference/kit/packages/kit/test/apps/basics/src/routes/data-sveltekit/preload-data/+page.svelte:L1-L27`;
  `client.test.js:L922-L1305`). Go impact: `__data.json` and route chunks must be fetchable on hover
  without side effects; `data-sveltekit-reload` triggers a full document request.
- `xss/`: inline data with `</script><script>window.pwned = 1</script>`, dynamic path, query param,
  shadow endpoint, tracked search params (GHSA-6q87-84jw-cjhp)
  (`reference/kit/packages/kit/test/apps/basics/src/routes/xss/**`; `cross-platform/test.js:L1086-L1148`).
  Go must escape everything it embeds in the shell (data payload, URL-derived values, `%sveltekit.*%`
  replacements — `$&` must survive `L1087-L1093`).

## Citations

- `reference/kit/packages/kit/test/apps/basics/vite.config.js:L1-L120`
- `reference/kit/packages/kit/test/apps/basics/src/hooks.server.js:L1-L240`, `src/hooks.js:L1-L54`,
  `src/hooks.client.js:L1-L32`, `src/app.html:L1-L14`, `src/error.html:L1-L10`
- `reference/kit/packages/kit/test/apps/basics/test/test.js:L59-L178`, `L233-L722`, `L778-L901`,
  `L1077-L1094`, `L1094-L1670`, `L1670-L1709`
- `reference/kit/packages/kit/test/apps/basics/test/client.test.js:L22-L43`, `L44-L420`, `L511-L921`,
  `L922-L1331`, `L1474-L1733`
- `reference/kit/packages/kit/test/apps/basics/test/server.test.js:L15-L212`, `L214-L527`, `L528-L785`,
  `L787-L869`, `L869-L972`, `L993-L997`, `L1028-L1079`, `L1647-L1661`
- `reference/kit/packages/kit/test/apps/basics/test/cross-platform/test.js:L62-L230`, `L230-L542`,
  `L542-L706`, `L706-L1086`, `L1086-L1148`
- `reference/kit/packages/kit/test/apps/basics/test/cross-platform/client.test.js:L751-L836`, `L1359-L1379`
- Route sources cited inline above under
  `reference/kit/packages/kit/test/apps/basics/src/routes/{streaming,serialization-basic,serialization-stream,set-cookie,csrf,headers,immutable-headers,caching,redirect-on-load,endpoint-input,endpoint-output,match,nested-layout,actions,cookies,errors,redirect,shadowed,xss,load,routing,encoded,data-sveltekit}/**`

## skgo implications

Borrow into the example app's Playwright suite (in priority order for a Go server that owns loads
and actions):
1. `__data.json` contract: `client.test.js:L231-L241` (no-store), `L406-L420` (not requested without
   server load), `server.test.js:L761-L785` (root layout data for unknown routes), `L1069-L1078`
   (`x-sveltekit-version`), `client.test.js:L23-L35` (`setHeaders` cache-control on data).
2. Streaming server loads: `client.test.js:L1475-L1524` (client-nav variants are exactly the CSR case).
3. Transport in server loads: `test.js:L1671-L1677`, `L1701-L1709`.
4. Invalidation and `depends()` with server loads: `client.test.js:L512-L542`, `L588-L632`, `L663-L811`.
5. Form actions envelope: `test.js:L1136-L1185`, `L1402-L1447`, `L1500-L1569`; `server.test.js:L919-L972`,
   `L660-L685`, `L841-L861`.
6. Cookies: `test.js:L1598-L1670`; `server.test.js:L377-L392`, `L1029-L1048`.
7. CSRF: `server.test.js:L62-L212` verbatim (pure HTTP).
8. Errors: `server.test.js:L529-L760`; `cross-platform/test.js:L293-L436` adjusted for CSR (footer and
   `#message` come from the client error page, status from `__data.json`).
9. Redirects: `cross-platform/test.js:L549-L706` (client-nav variants), incl. header-injection 500.
10. Endpoint method handling if `+server` is ported to Go: `server.test.js:L215-L527`.

Not applicable to skgo: universal-load fetch serialisation into `<script data-sveltekit-fetched>`
(SSR only), `%sveltekit.assets%` relative paths during SSR, dev-only `invalid-load-response`
messages, tracing/`read_traces`, service-worker and Vite `define` tests.

## Gotchas

- `test.js` runs in both projects and branches on `javaScriptEnabled`; when porting, take the JS
  branch only.
- Several suites reset server state with `POST .../reset-states` using
  `headers: { origin: 'https://trusted.example.com' }` because CSRF blocks form-typed POSTs; Go
  must honour `trustedOrigins` for those helpers to work (`client.test.js:L593-L595`).
- `read_errors` depends on the server appending `test/errors.jsonl` from `handleError`; the Go
  server needs an equivalent sink or those assertions must be dropped.
- The `hooks.server.js` `handle` chain adds `set-cookie: name=SvelteKit` to every response, and the
  `set-cookie` test expects it as the third cookie (`server.test.js:L387-L391`).
- `vite preview` buffers streamed responses, so direct-hit streaming tests only run in DEV; a Go
  server that streams can enable them unconditionally.
- Trailing-slash 308 redirects for prerendered pages come from Vite preview in kit's tests
  (`server.test.js:L902-L917`); skgo serves prerendered output itself and must implement the redirect.

## Recipes

- Prove a server load runs only when needed: navigate with `app.goto`, then `app.invalidate(url |
  'custom:key')`, then read counters rendered by the page (`client.test.js:L88-L129`).
- Assert action JSON envelope: `request.post(url, { form: {...}, headers: { accept:
  'application/json', origin } })` then `expect(await response.json()).toEqual({ type, status,
  location, data })` (`server.test.js:L919-L955`).
- Assert redirect envelope on enhanced submit: `Promise.all([page.waitForResponse('/actions/redirect'),
  page.waitForNavigation()])` then `expect(await redirect.json()).toEqual({ type:'redirect',
  location, status:303 })` (`test.js:L1402-L1423`).
- Multi-value `set-cookie` check: `response.headersArray().filter(h => h.name === 'set-cookie')`
  (`server.test.js:L380-L391`).
- Stream check: compare `sha256` of `await response.body()` with the `digest` header
  (`server.test.js:L393-L403`).
