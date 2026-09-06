# Kit 3.0.0-next.25 remote-function wire protocol — facts observed by the junkyard oracle

**Purpose.** Every concrete fact about the SvelteKit remote-function wire format (`query`, `command`, `query.live`) that the junkyard's oracle laboratory observed against a real kit `3.0.0-next.25` client, or that its reviewers read from pinned kit source `a59327223dd1c6bde4dac71c7946b9603f6995f7`. This is the leaf a Go implementation of the protocol should start from; each fact is tagged OBSERVED / SOURCE-DERIVED / ASSERTED and cited.

Pins that all facts below are relative to: kit `3.0.0-next.25` (commit `a593272…`), devalue `5.9.2` (`a3d30d9…`), adapter-node `6.0.0-next.10`, svelte `5.57.0`, Vite+ `0.3.0`, Node `24.16.0`, TypeScript `6.0.3` — `junkyard/oracle/source-lock.json:L4-L97`, `junkyard/oracle/package.json:L15-L28`.

Tag legend:
- **OBSERVED** — a real kit client (Chromium via Playwright) or the built adapter-node server produced it in a recorded run, or a Go handler that implemented it passed all lanes against the real client (interop-observed).
- **SOURCE-DERIVED** — a reviewer/author read it from pinned kit source and cited a line; not exercised at runtime.
- **ASSERTED** — stated in prose without evidence or citation.

---

## 1. URL shape and remote-function IDs

- **OBSERVED.** Every remote call goes to `${base}/${appDir}/remote/<id>` where `<id>` is `<hash>/<exportName>`. Default config yields `/_app/remote/2rsbgs/getDocument`; mounted config (`base:'/lab'`, `appDir:'oracle-assets'`) yields `/lab/oracle-assets/remote/2rsbgs/getDocument`. — `junkyard/oracle/proof/switchboard.mjs:L16-L21` (prefix + `decodeURIComponent` of the remainder), `junkyard/oracle/proof/run.mjs:L366` (prefix derived from manifest base/appDir), `junkyard/oracle/proof/run.mjs:L873-L892` (assertion that every `/remote/` request starts with the prefix), `junkyard/ephemeral/reviews/or2-fable-round-01.md:L52` (browser paths `/lab/oracle-assets/remote/2rsbgs/{watchDocument,getDocument,saveDocument}` and `/lab/oracle-assets/remote/16ghqs9/getDocument`).
- **OBSERVED hash vectors** (kit's own transform output, captured from the generated client): `src/lib/document.remote.ts` → `2rsbgs`; `src/lib/secondary.remote.ts` → `16ghqs9`; `src/lib/renamed-document.remote.ts` → `1oadct1`. — `junkyard/ephemeral/remote-oracle/validation/blackbox-02.md:L55,L75-L77`, `junkyard/ephemeral/reviews/or2-fable-round-01.md:L42`, `junkyard/ephemeral/remote-oracle/reviews/opus-01.md:L77-L80`. These three (path, hash) pairs are ready-made unit-test vectors for a Go port of kit's `hash()`.
- **SOURCE-DERIVED.** The hash input is the **root-relative POSIX path of the `.remote.ts` module**: `hash(posixify(path.relative(root, id)))`, then `/` + export name appended. — `junkyard/ephemeral/remote-oracle/reviews/or2-plan-03.md:L26`, `junkyard/ephemeral/remote-oracle/sprint-02.md:L35-L38` (kit `exports/vite/index.js:650, 678, 735`; `utils/hash.js:5`). Root-relativity is corroborated at runtime: the fixture was copied to a different absolute root and `16ghqs9` stayed stable, while renaming the file changed all three of its IDs — `junkyard/oracle/proof/run.mjs:L798-L829`, `junkyard/ephemeral/remote-oracle/validation/blackbox-02.md:L54-L55`.
- **SOURCE-DERIVED.** Client URL construction is `${base}/${app_dir}/remote/${id}` in all three client kinds: `runtime/client/remote-functions/query/index.js:27`, `command.svelte.js:48`, `query-live/iterator.js:25`. — `junkyard/ephemeral/remote-oracle/reviews/or2-plan-01.md:L34-L37`, `junkyard/ephemeral/remote-oracle/sprint-02.md:L37-L38`.
- **OBSERVED.** The kit client transform of a `.remote.ts` module (non-SSR build) emits calls of the literal form `.query('<hash>/<name>')`, `.command('<hash>/<name>')`, `.query_live('<hash>/<name>')`; the oracle's registry plugin regex-scrapes exactly those. — `junkyard/oracle/vite/protocol-switchboard.ts:L31-L38` (regex `\.(query|command|query_live)\(['"]([^'"]+\/([^/'"]+))['"]\)`). This is the authoritative way the junkyard got IDs: **read kit's generated client, never recompute the hash** (`:L6`).
- **SOURCE-DERIVED (trap).** Vite resolves symlinks; a symlinked `src/lib/x.remote.ts` makes `path.relative(root, id)` start with `../` and produces a different hash. Copy real files at identical repo-relative paths. — `junkyard/ephemeral/remote-oracle/reviews/or2-plan-03.md:L66-L75`, `junkyard/oracle/proof/run.mjs:L712-L727` (copies `src/` and `vite/`, symlinks only `node_modules`).
- **SOURCE-DERIVED.** Remote-module handling is graph-scoped, not glob-scoped: a `.remote.ts` file nobody imports produces no ID and no route. In dev, the transform runs lazily on first browser import (`exports/vite/index.js:702-731`). — `junkyard/ephemeral/remote-oracle/reviews/or2-plan-02.md:L38-L40`, `junkyard/ephemeral/remote-oracle/reviews/or2-plan-01.md:L96-L100`.
- **SOURCE-DERIVED.** Kit sets Vite's dev `base` to `paths.base` (`exports/vite/index.js:1061`) and the dev middleware enforces it (`exports/vite/dev/index.js:528-567`), so a `base:'/lab'` app is at `/lab/` in dev as well as production. — `junkyard/ephemeral/remote-oracle/reviews/or2-plan-03.md:L38-L41`.
- **OBSERVED.** In dev, the browser also fetches `/lab/src/lib/document.remote.ts` (module source) — these are *not* protocol requests. Detect protocol traffic by the `/remote/` **path segment**, not the `*.remote.*` filename. — `junkyard/ephemeral/remote-oracle/reviews/or2-plan-03.md:L79-L85`, `junkyard/ephemeral/remote-oracle/implementation-handoff.md:L74-L77`.

## 2. HTTP method per kind

- **OBSERVED.** `query` → GET; `query.live` → GET (long-lived); `command` → POST. In every Go lane the recorded remote traffic was "primary query, secondary query, live GET, and exactly one command POST". — `junkyard/ephemeral/remote-oracle/validation/blackbox-02.md:L57`, `L75-L77` ("one `2rsbgs/saveDocument` POST"), `junkyard/ephemeral/remote-oracle/reviews/opus-01.md:L80`.
- **TO VERIFY.** Whether kit ever falls back to POST for oversized query payloads, and what the server does for wrong methods (HTTP-002 "method handling as actually implemented", `runtime/server/remote-functions.js:154, 322, 339, 345`). — `junkyard/ephemeral/remote-oracle/inventory.md:L45`.

## 3. Argument encoding (payload)

- **OBSERVED (interop).** The argument travels as **base64 of `devalue.stringify(arg)`** with base64 padding stripped. Evidence: the refresh/cache key for `getDocument("document.json")` is `<id>/WyJkb2N1bWVudC5qc29uIl0`, and `WyJkb2N1bWVudC5qc29uIl0` decodes to `["document.json"]` — exactly `devalue.stringify("document.json")`, 22 chars i.e. no `=` padding. — `junkyard/ephemeral/remote-oracle/reviews/opus-01.md:L81`, `junkyard/ephemeral/remote-oracle/reviews/adjudication-01.md:L37`. The Go decoder (`decodePayload`) base64-decodes then JSON-unmarshals and passed all lanes against the real client. — `junkyard/ephemeral/remote-oracle/reviews/opus-01.md:L165-L167`.
- **OBSERVED.** Command POST body is JSON: `{"payload":"<base64 devalue of arg>","refreshes":["<key>", ...]}`. — `junkyard/ephemeral/remote-oracle/reviews/opus-01.md:L80-L81`.
- **SOURCE-DERIVED.** Empty payload string means `undefined` argument (kit `parse_remote_arg`, `runtime/shared.js:321-322`); Go rejecting `""` was a recorded defect. — `junkyard/ephemeral/remote-oracle/reviews/opus-01.md:L163-L169`, `opus-02.md:L160-L164`.
- **SOURCE-DERIVED.** Query/live payloads are **canonicalised** (object/Map/Set key order sorted by reducers in `runtime/shared.js:96, 252`) so equal args yield byte-identical keys; **command payloads are not sorted** (`shared.js:265, 301`). This matters because the query payload doubles as the cache/refresh key. — `junkyard/ephemeral/remote-oracle/inventory.md:L32-L33`, `junkyard/ephemeral/remote-oracle/overview.md:L67-L68`.
- **SOURCE-DERIVED.** Query key = `create_remote_key(id, payload)` / `split_remote_key` in `runtime/shared.js` (~L337). — `junkyard/ephemeral/remote-oracle/reviews/opus-01.md:L30-L31`, `overview.md:L149-L150`.
- **TO VERIFY.** GET query string parameter name (expected `?payload=<urlencoded base64>`), whether the base64 alphabet is URL-safe, and how "no argument" vs `null` differ on the wire (ADDR-002). Check `runtime/client/remote-functions/query/index.js:27`, `runtime/shared.js` (`stringify_remote_arg`/`parse_remote_arg`), `runtime/server/remote-functions.js:165`. — `junkyard/ephemeral/remote-oracle/inventory.md:L30`.
- **SOURCE-DERIVED.** Command arguments may include `File` (multipart or special encoding, CODEC-002, `shared.js:301`); Promise/RegExp are rejected. Not exercised. — `junkyard/ephemeral/remote-oracle/inventory.md:L33`.

## 4. Response envelopes

### 4a. query (GET) — OBSERVED
`{"type":"result","data":"<devalue.stringify(value)>"}` where `data` is a **string** containing the devalue JSON, decoded with `devalue.parse`. The runner computes `parseDevalue(JSON.parse(body).data)` for both query and secondary query. — `junkyard/oracle/proof/run.mjs:L927-L931`.

### 4b. command (POST) — OBSERVED
`{"type":"result","data":"<devalue.stringify({ _: result, r?: …, q?: {…} })>"}`. The command's return value is under `_`; refresh output rides in the same response.

- Minimal (no refresh matched): `{"type":"result","data":"[{\"_\":1},{\"name\":2,\"value\":3,\"version\":4},\"oracle\",\"node-saved\",2]"}` → `{_: {name:"oracle", value:"node-saved", version:2}}`. — `junkyard/ephemeral/remote-oracle/reviews/opus-01.md:L89`.
- With a requested refresh (happy path): the object has `_`, `r` and `q`; Node A and Go produced equal `_`/`q`/`r` envelopes. — `junkyard/ephemeral/remote-oracle/reviews/opus-01.md:L52-L54`.
- Refresh **failure is data, not an envelope error**: `type` stays `"result"`, `_` is preserved, and the failed key gets `q[key] = {"e": {"status": 400, "message": "…"}}`. Observed message: `Requested refresh was rejected because it exceeded requested(getDocument, 1) limit`. Raw: `[{\"_\":1,\"r\":5,\"q\":6},…,{\"e\":8},{\"status\":9,\"message\":10},400,\"Requested refresh was rejected …\"]`. — `junkyard/ephemeral/remote-oracle/reviews/opus-01.md:L92-L98,L103-L106`.
- An **unknown refresh key** in `refreshes[]` is ignored by kit (result still `type:"result"` with `_` only). — `junkyard/ephemeral/remote-oracle/reviews/opus-01.md:L80-L89`, `adjudication-01.md:L35-L39`.
- **TO VERIFY.** Exact meaning/shape of `r` versus `q` (which is keyed by refresh key → `{d}`/`{e}`?), and the role of `requested(query, limit)` selection. Read kit `runtime/server/remote-functions.js` `collect_remote_data` (~L361-430) and `runtime/app/server/remote/requested.js:104-135`. — `junkyard/ephemeral/remote-oracle/reviews/opus-01.md:L103-L112`.

### 4c. single-flight refresh — OBSERVED
Client `saveDocument(args).updates(getDocument(path))` sends the query's key inside `refreshes[]`; server-side `await requested(getDocument, 1).refreshAll()` executes the query in the command flight; the browser makes **exactly one** fetch and the query display updates with no follow-up GET. — `junkyard/oracle/src/routes/+page.svelte:L21-L22`, `junkyard/oracle/src/lib/document.remote.ts:L19`, `junkyard/oracle/proof/run.mjs:L473-L478,L551-L552` (`A02-single-flight`, `P02-single-flight`). `requested()` selects only the client-supplied keys for that query id, bounded by `limit` — the server, not the client list, decides what executes (kit `requested.js:104-135`). — `opus-01.md:L106-L112`.

### 4d. error envelope — PARTIAL
- **OBSERVED (Go's shape, accepted by the client):** `{"type":"error","error":{"message":"…"}}`. — `junkyard/ephemeral/remote-oracle/reviews/opus-01.md:L82,L100`. Kit's own error envelope for thrown/validation errors was **not** observed; HTTP-002 remains open. — `inventory.md:L45`.
- **OBSERVED (kit, adapter-node build):** cross-site request → body `{"message":"Cross-site remote requests are forbidden"}`, store unchanged. Status code not recorded (expect 403; verify `runtime/server/respond.js:101`, `runtime/server/csrf.js:63`). — `opus-01.md:L124-L127`.
- **ASSERTED.** Redirect envelope and unhandled-error envelope shapes exist (`runtime/server/remote-functions.js:322-345`, `runtime/server/errors.js:44`) — never exercised. — `inventory.md:L45`.

### 4e. headers — OBSERVED
- Native kit responses carry `x-sveltekit-version`; the runner requires it on every lane (`native-version-header`) and passes the observed value to the Go fixture via `--version` so Go emits the same. — `junkyard/oracle/proof/run.mjs:L641-L644`, `L339-L340`.
- **TO VERIFY.** `Cache-Control`/`Content-Type` on query responses, and SSE `Content-Type: text/event-stream` on live streams (HTTP-002). — `inventory.md:L45`.

## 5. `query.live` stream framing — OBSERVED

- Transport is **Server-Sent Events over GET**. Frames are `data: <JSON>\n\n`; each JSON frame has `type`, and result frames are `{"type":"result","result":"<devalue.stringify(value)>"}` (note the key is `result`, not `data`, inside live frames). The runner parses `line.startsWith("data: ")` → `JSON.parse` → filter `frame.type === "result"` → `devalue.parse(frame.result)`. — `junkyard/oracle/proof/run.mjs:L919-L924`.
- A complete frame ends with `\n\n`; the N03 control asserts the flushed wrong-value body `endsWith("\n\n")`. — `junkyard/oracle/proof/run.mjs:L647-L651`, `junkyard/ephemeral/remote-oracle/sprint-01.md:L79-L80`.
- The first frame arrives promptly after subscribe (initial value displayed at version 1); an external change produced a new frame with DOM change in 2–78 ms. — `validation/blackbox-01.md:L53-L58`, `validation/blackbox-02.md:L59,L75-L77`.
- Server-side the generator loop is cancelled via `getRequestEvent().request.signal` when the client disconnects (browser context close). — `junkyard/oracle/src/lib/document.remote.ts:L24-L37`.
- **SOURCE-DERIVED / unexercised:** kit suppresses duplicate yields itself and sends a real 30-second heartbeat on quiet connections (`runtime/server/remote-functions.js:27, 60, 108`); the fixture pre-filters duplicates so this was never tested. — `inventory.md:L41,L62-L64`. Other frame `type`s (error, completion, reconnect seed) exist per `remote-functions.js:394` / `query-live/instance.svelte.js:146-252` but were not observed. — `inventory.md:L39,L42-L43`.
- **TO VERIFY.** SSE event names (`event:` lines?), `id:`/`retry:` fields, heartbeat comment format, terminal/error frame shape, client retry behaviour. Files: `runtime/server/remote-functions.js` (`create_live_query_response`), `runtime/client/remote-functions/query-live/{iterator.js,instance.svelte.js}`.

## 6. Origin / CSRF boundary

- **OBSERVED (adapter-node build).** A POST whose `Origin` differs from the configured `paths.origin` is rejected with `Cross-site remote requests are forbidden` before the handler runs. — `opus-01.md:L119-L127`.
- **ASSERTED / SOURCE-DERIVED.** The origin check is skipped in dev, so only a production build can prove it (`runtime/server/respond.js:101`, `csrf.js:63`). — `overview.md:L55-L56`, `inventory.md:L44`.
- **OBSERVED config.** The app's origin is fixed at build time via `paths.origin` (set to the browser-facing scheme/host/port, no base path) and adapter-node also reads `ORIGIN`/`HOST`/`PORT` env. — `junkyard/oracle/vite.config.ts:L8,L22`, `junkyard/oracle/proof/run.mjs:L165-L187`, `sprint-02.md:L26-L27`.

## 7. Kit configuration needed for the client to emit this protocol (kit 3)

- No `svelte.config.js`; options are passed flat to `sveltekit({...})` in `vite.config.ts`: `adapter`, `appDir`, `paths:{base, origin}`, `compilerOptions:{experimental:{async:true}}`, `experimental:{remoteFunctions:true}`. — `junkyard/oracle/vite.config.ts:L17-L28`. Validation: `appDir` no leading/trailing slash; `paths.base` starts but does not end with `/` (`core/config/options.js:71,187`). — `sprint-02.md:L31-L34`.
- Client-only page (`export const ssr = false`) still drives the full protocol; SSR carriers are a separate path never exercised. — `junkyard/oracle/src/routes/+page.ts:L1`, `overview.md:L104-L110`.
- Import alias in kit 3 is `#lib/*` via `package.json` `imports` (the oracle added it; `$lib` gone). — `junkyard/oracle/package.json:L6-L8`, `junkyard/oracle/profiles/mounted/+page.svelte:L2-L3`.
- TypeScript 7 lacks `ts.sys`; kit needs TS 6.0.3. — `source-lock.json:L98-L100`.
- Kit and Vite must be the same class instances: a global `vp` fails kit's `RunnableDevEnvironment instanceof` guard — run the locked local `vp` under mise's Node. — `junkyard/ephemeral/worklog/20260904-remote-oracle-or1.md:L4`, `junkyard/oracle/README.md:L104-L105`.

## 8. Behavioural facts useful for a Go handler

- Query results are cached client-side by key; after an external change the ordinary query stays at its last value until a refresh or reload, while the live query updates. — `junkyard/oracle/README.md:L81-L83`, `run.mjs:L501-L502`.
- Refresh handling must not turn a persisted mutation into an envelope error: kit keeps `_` and reports per-key `e`. Go's first cut discarded `_` on any refresh failure — recorded defect (issue #1). — `opus-01.md:L68-L117`, `adjudication-01.md:L35-L42`.
- Go must own the **entire** remote prefix, including unknown IDs; never fall through to Node. — `proposal.md:L38-L39`, `run.mjs:L635-L640`.

---

## Reusable verdict for skgo

Take directly:
1. URL = `${base}/${appDir}/remote/${hash}/${export}`; hash vectors `2rsbgs`/`16ghqs9`/`1oadct1` for the three module paths above.
2. GET for query and live, POST for command; command body `{"payload","refreshes":[]}`; arg = base64-nopad(devalue.stringify(arg)); empty → `undefined`.
3. Query response `{"type":"result","data":"<devalue>"}`; command response `data` = devalue of `{_, r?, q?}` with per-key `{e:{status,message}}` on refresh failure; unknown refresh keys ignored.
4. Live = SSE `data: {"type":"result","result":"<devalue>"}\n\n`.
5. Emit `x-sveltekit-version`; enforce the cross-site Origin guard in production (`Cross-site remote requests are forbidden`).

Must still verify against `reference/kit/packages/kit/src/` before trusting: the GET query-string parameter name and base64 alphabet (`runtime/shared.js`, `runtime/client/remote-functions/query/index.js`); `r` vs `q` semantics and `requested()` selection (`runtime/server/remote-functions.js` `collect_remote_data`, `runtime/app/server/remote/requested.js`); live SSE heartbeat/error/completion frames (`runtime/server/remote-functions.js` `create_live_query_response`, `runtime/client/remote-functions/query-live/*`); native error/redirect envelopes and status codes (`remote-functions.js:322-345`, `errors.js:44`); origin-check status code (`respond.js:101`, `csrf.js:63`); `query.batch`, `form`, `prerender` (never in scope of the oracle — `overview.md:L74-L76`).

## Gotchas and traps
- Never recompute kit's hash by hand as the authority; scrape kit's generated client (`protocol-switchboard.ts:L6`). Use the hash port only after it reproduces the three vectors.
- Symlinked sources change IDs (`or2-plan-03.md:L66-L75`).
- Dev transform is lazy: the ID for a module doesn't exist until the browser imports it (`or2-plan-01.md:L96-L110`); HMR re-transforms, so append-only ID registries duplicate entries.
- Production-only origin check: a dev pass proves nothing about CSRF (`overview.md:L55-L56`).
- Distinguish protocol requests by `/remote/` segment, not `.remote.` filename (`or2-plan-03.md:L79-L85`).
- `x-sveltekit-version` header is asserted by the harness on every native response — the Go side had to copy the observed value (`run.mjs:L641-L644`).

## Recipes
- To see how the runner decodes each envelope: `junkyard/oracle/proof/run.mjs:L906-L939`.
- To see a raw command POST/response pair for both backends: `junkyard/ephemeral/remote-oracle/reviews/opus-01.md:L77-L101,L122-L135`.
- To see how IDs were harvested from kit's transform: `junkyard/oracle/vite/protocol-switchboard.ts:L16-L47`.
- To see the fixture's server-side API use (`query("unchecked", …)`, `command`, `query.live` generator, `requested(...).refreshAll()`, `getRequestEvent().request.signal`): `junkyard/oracle/src/lib/document.remote.ts:L1-L37`.
- To see the client-side API use (`.current`, `.updates(...)`): `junkyard/oracle/src/routes/+page.svelte:L1-L25`.
- Kit source entry points list (line numbers at pin `a593272`): `junkyard/ephemeral/remote-oracle/overview.md:L145-L159`, `junkyard/ephemeral/remote-oracle/inventory.md:L27-L48`.
