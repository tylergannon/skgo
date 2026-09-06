# Port map — upstream unit tests → Go packages

## Purpose

One table an implementer opens first: every portable upstream spec in kit
3.0.0-next.25 and devalue 5.9.2, what it pins, which Go package should own the port, how
mechanical the port is, and where the detailed leaf lives. Ordered by value to the wire
protocol skgo must reproduce (`__data.json`, `/_app/remote/*`, form actions, endpoints,
static/prerendered output, dev proxy).

Difficulty legend: **mechanical** = table/assertion port with stdlib only; **needs
harness** = requires a fake (request store, hooks, manifest, stream reader, fixture dirs);
**JS-only** = semantics tied to JS objects/Proxies/AST/SSR; skip or record only.

## Key facts (SOURCE-DERIVED)

| # | upstream spec (relative to token cache root, prefix `reference/kit/packages/kit/src/`) | covers | Go package | difficulty | notes / leaf |
|---|---|---|---|---|---|
| 1 | `reference/devalue/test/index.test.js:L26-L1269` (+ `L1409-L1681`, `L1685-L1866`) | reduced-JSON codec: sentinels, tags, cycles, repetition, escaping, invalid inputs, sparse DoS | `internal/devalue` | mechanical (fixture table) — skip `uneval`, Temporal, `functions`, `operations` | everything else on the wire depends on it → `devalue-port.md` |
| 2 | `runtime/server/remote-functions.spec.js:L1-L97` + subject `remote-functions.js:L27-L611` | `query.live` SSE frames, cancellation, keep-alive; dispatcher rules for query/batch/form/command/prerender; `{type:'result'|'error'}` envelopes; `collect_remote_data` `{_, r, q, p, l, f, redirect}` | `internal/remote` | needs harness (streaming writer, hooks fake, manifest fake) | `remote-functions-spec.md` |
| 3 | `runtime/shared.spec.js:L32-L383` + `runtime/shared.js:L84-L356` | remote-arg canonical stringify (sorted keys, Map/Set as sorted nested strings, url-safe base64 no padding), parse, RegExp/Promise guards, File in command args, `create_remote_key`/`split_remote_key` | `internal/remote/args` | mechanical once #1 exists (needs `init_transport` fake = encoder map) | `serialization-and-streams-spec.md` |
| 4 | `runtime/form-utils.spec.js:L23-L852` + `runtime/form-utils.js:L26-L587` | field-name grammar, `convert_formdata`, binary body layout, LazyFile, 10 exact 400 error strings, prototype-pollution guards, `deep_set` | `internal/form` | needs harness (`http.Request` builders, chunked body readers) — port helpers `build_raw_request*` | `serialization-and-streams-spec.md` |
| 5 | `runtime/server/cookie.spec.js:L7-L373` | `domain_matches`/`path_matches`, `set`/`delete` defaults (`secure` true except localhost/dev, `httpOnly`, `sameSite:'lax'`, `path`, `maxAge:0` on delete, cannot override), last-set-wins, case-sensitive names, url-encoding of header, path specificity in `get`/`getAll`, unique key `${domain}${path}?${name}`, custom decode not cached, `set_internal` bypasses defaults, `parse` (expires, trailing `=`) | `internal/cookie` | mechanical (needs a `Request` with `cookie` header + URL; prod/dev split via flag) | every Go-side form action / remote call sets cookies; the `/?a` and `example.com/?key` key format is observable in `new_cookies` |
| 6 | `runtime/server/csrf.spec.js:L4-L333` | `get_self_origin(paths.origin, requestOrigin)`; `is_csrf_forbidden` (form content types, POST/PUT/PATCH/DELETE, origin mismatch, `trusted_origins`); `is_remote_forbidden` (any non-GET, content-type-agnostic, trusted origins **not** honoured) | `internal/csrf` | mechanical | the fixed-at-build origin (`paths.origin`) is a kit-3 fact; port all 29 cases (22 `test` entries, three of them `test.each`) |
| 7 | `utils/routing.spec.js:L10-L660` + `utils/routing.js` | route-id → regex (19 ids), `exec` params (44 rows + 2 schema cases), `resolve_route` (24 rows + 3 error cases), `find_route` (first match, matchers, decode) | `internal/routing` | mechanical, with RE2 rewrite of `[^]` and lazy quantifiers | `routing-and-manifest-spec.md` |
| 8 | `core/sync/create_manifest_data/index.spec.js:L58-L976` + `index.js`, `sort.js`, `conflict.js`, fixture dirs `test/samples/*` | filesystem → routes/nodes, node index order, named layouts, escapes, hidden files, conflicts, 12 error messages, `sort_routes` golden of 28 ids, static asset listing | `internal/manifest` | needs harness (copy fixture dirs to `testdata/`; page-option analysis stubbed) | `routing-and-manifest-spec.md` |
| 9 | `runtime/client/stream.spec.js:L30-L70`, `runtime/client/ndjson.spec.js:L33-L48` (+ `client/stream.js`, `client/ndjson.js`, server `page/data_serializer.js:L130-L217`) | frame splitting semantics the server must satisfy: `\n`-delimited JSON lines, `\n\n` SSE, UTF-8 split across chunks, no trailing empty frame, fatal on malformed UTF-8; `__data.json` first line + chunk lines | `internal/stream` | mechanical (byte-slice inputs) | `serialization-and-streams-spec.md` |
| 10 | `utils/streaming.spec.js:L4-L25`, `utils/shared-iterator.spec.js:L18-L178` | chunk emission in resolution order; multi-subscriber latest-wins broadcaster with start/stop/done/fail | `internal/stream` | mechanical (goroutines + channels; use timeouts instead of `vi.waitFor`) | powers deferred `__data.json` chunks and `query.live` fan-out |
| 11 | `runtime/app/server/remote/prerender.spec.js:L57-L137` | prerender wrapper self-fetch: error envelope → `HandledHttpError` bypassing `handleError`; result envelope parsed via devalue `{_:…}`; fallback to fn on reject / non-ok / non-JSON; `ValidationError` → `{kind:'validation', error:{status:400,message:'Bad Request'}, issues}` | `internal/remote/prerender` | needs harness (RoundTripper fake, request-store fake, hooks fake) | also the only unit test of `handle_error_and_jsonify` shapes |
| 12 | `runtime/utils.spec.js:L4-L91` | base64 vectors (std alphabet, padded) both ways; `stream_from_iterable` ordering and cancel→finally | `internal/devalue` (base64) / `internal/stream` | mechanical | `reference/devalue/src/base64.test.js:L5-L42` adds 13 more vectors |
| 13 | `utils/url.spec.js:L11-L152`, `L100-L121` | `normalize_path` trailing-slash modes, `resolve` relative paths, `relative_pathname` for redirects, external allowlist matching | `internal/urlutil` | mechanical | `make_trackable`/`disable_search` (L154-L267) are JS Proxy semantics → record messages only |
| 14 | `exports/url.spec.js:L4-L56` | `is_external_location` (scheme, `//`, `\\`, leading whitespace, `java\tscript:`, `blob:`); `validate_redirect_location` error text | `internal/urlutil` | mechanical | needed by `redirect()` handling in Go loads/actions |
| 15 | `utils/http.spec.js:L4-L29` | `negotiate(accept, types)` incl. OWS + no catastrophic backtracking; `matches_content_type` case/param-insensitive | `internal/httputil` | mechanical | `is_form_content_type` (`utils/http.js:L73-L79`) gates CSRF and `form` remote calls |
| 16 | `utils/error.spec.js:L5-L22` | `get_status`: HttpError/SvelteKitError status else 500 | `internal/kiterror` | mechanical | L24-L41 deprecated accessor warnings: JS-only |
| 17 | `exports/hooks/sequence.spec.js:L23-L55` | `sequence(...handles)` runs outer→inner and unwinds | `internal/hooks` | mechanical | L57-L183 (`transformPageChunk`, `preload`, `filterSerializedResponseHeaders`) are SSR options — JS-only unless Go loads expose `resolve` options |
| 18 | `utils/params.spec.js:L10-L47` | matcher-name collection + `No matcher found for parameter '<n>'` at build | `internal/manifest` | mechanical | L49-L102 standard-schema normalisation: JS-only (Go matchers are typed funcs) |
| 19 | `exports/vite/static_analysis/index.spec.js:L4-L212` | detect literal `ssr`/`csr`/`prerender`/`trailingSlash`/`config`/`entries` exports; `null` on anything dynamic; `load` special-case | — (have the adapter/Vite side emit page options into the Go manifest) | JS-only (acorn AST) | `routing-and-manifest-spec.md` |
| 20 | `runtime/server/validate-headers.spec.js:L11-L98` | dev-time warnings for bad `cache-control` directives / malformed `content-type` | `internal/devwarn` (optional) | mechanical | pure DX; low value |
| 21 | `runtime/server/internal.spec.js:L10-L58` | `format_response(status, request)` → `"200 GET /path?query"` for dev logging | `internal/devlog` (optional) | mechanical | trivial |
| 22 | `runtime/server/page/serialize_data.spec.js:L4-L103` | inline `<script data-sveltekit-fetched>` for SSR fetch replay, `data-ttl` | — | JS-only (SSR) | record only |
| 23 | `runtime/server/page/load_data.spec.js:L30-L91` | universal `fetch` CORS emulation during SSR | — | JS-only (SSR) | record only |
| 24 | `runtime/server/page/crypto.spec.js:L6-L22` | `sha256` → base64 (CSP) | — | trivial if needed | `crypto/sha256` + `base64.StdEncoding` |
| 25 | `runtime/server/page/csp.spec.js` (22 tests, L11-L565) | CSP header/meta with nonces/hashes | — | JS-only (HTML rendering) | note only |
| 26 | `utils/escape.spec.js:L4-L19` | HTML attribute escaping incl. lone surrogates | — | JS-only (HTML) | note only |
| 27 | `reference/devalue/test/operations.test.js`, `parse-operations.test.js` | `operations` hook API (foreign-runtime handles, cross-realm) | — | JS-only | do not port |
| 28 | `reference/devalue/src/utils.test.js:L7-L61` | `valid_array_indices` on JS arrays with named props | `internal/devalue` (index-string validator only) | partial | JS array semantics |

Additional facts that shape package boundaries:

- Transport hooks (user `encode`/`decode`) plug into devalue as reducers/revivers keyed by
  name (`reference/kit/packages/kit/src/runtime/app/internal/transport.js:L32-L53`); Go
  needs an equivalent registry consulted by `internal/devalue` for every stringify/parse.
- `handle_error_and_jsonify` (`runtime/server/errors.js:L44-L89`) is the single error →
  `App.Error` mapper used by remote calls, `__data.json` error nodes, and live queries; its
  shapes are only unit-tested through `prerender.spec.js:L57-L104`.
- `is_remote_forbidden` (csrf) runs before `handle_remote_call`; `command` has no method
  check in the dispatcher, so CSRF is the only guard against cross-origin non-GET.

## Citations

- Versions: `reference/kit/packages/kit/package.json:L3` (`3.0.0-next.25`), `L26` (`devalue ^5.9.0`); `reference/devalue/package.json:L4` (`5.9.2`)
- Per-row citations are in the table; detail leaves: `remote-functions-spec.md`, `devalue-port.md`, `serialization-and-streams-spec.md`, `routing-and-manifest-spec.md` (same directory).

## Go port notes

- Build order that unblocks the most tests: `internal/devalue` (#1, #12) → `internal/remote/args` (#3) → `internal/stream` (#9, #10) → `internal/remote` (#2, #11) → `internal/form` (#4) → `internal/cookie` (#5) + `internal/csrf` (#6) + `internal/httputil` (#15) → `internal/routing` (#7) → `internal/manifest` (#8, #18) → `internal/urlutil` (#13, #14) → `internal/kiterror` (#16) → `internal/hooks` (#17).
- Shared test fakes worth writing once: `fakeHooks{handleError}`, `fakeRequestState`,
  `fakeManifest{remotes}`, `chunkedBody(bytes, chunkSize)`, `recordingResponseWriter`
  (exposes frames via channel), `fixtureDir(t, name)` for manifest samples.
- Keep every upstream error string verbatim; kit's client and Playwright suites match on them.

## Gotchas

- Several specs stub `__SVELTEKIT_DEV__`; dev-vs-prod behaviour (cookie `secure`, error
  messages with extra hints) must be a runtime flag in Go, not a build tag, so both halves
  of `cookie.spec.js` can run.
- Specs that use `vi.mock('@sveltejs/kit/internal/server')` (`prerender.spec.js`,
  `sequence.spec.js`) are replacing AsyncLocalStorage request stores; Go passes the store
  explicitly (`context.Context` value) — no mocking needed.
- Some fixtures rely on Node `Request`/`Response`/`FormData`; translate to
  `net/http` + `mime/multipart` rather than emulating WHATWG objects.
- The manifest fixture `samples/symlinks` may not survive checkout without symlink support;
  upstream skips the test when the link is gone (`index.spec.js:L132-L137`).

## Recipes

- Start a port: open the leaf for the row, copy the cited spec lines into a Go
  `_test.go` table with `name`, inputs, `want`, then implement until green; cite the
  upstream lines in a comment above the table.
- Cross-check a wire string against Node without a running app: `node -e "import('devalue').then(d=>console.log(d.stringify(<value>)))"` from `reference/devalue` (pinned clone), and compare to the Go output in a test helper guarded by an env var.
