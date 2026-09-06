# Remote functions: the authored surface skgo's Go side must mirror

## Purpose

Kit 3 remote functions are the primary thing skgo reimplements in Go. This file records the
*authored* TypeScript surface kind by kind — what a Svelte developer writes and what the
client-side object exposes — plus the server-side semantics (caching, validation, refresh,
redirects, `getRequestEvent`) that the Go handler generator must reproduce. The wire protocol
(`/_app/remote/<hash>/<fn>` argument encoding, devalue envelopes, live-stream framing, the
form binary encoding) is **not** in the authored docs; it lives in kit source and is
another segment's job. Where a fact is source-level, it's flagged.

Citation paths are relative to `/Users/tyler/src/skgo/ephemeral/inspiration/`. The
canonical type file is `reference/kit/packages/kit/src/runtime/app/server/public.d.ts`
(kit `3.0.0-next.25`, `reference/kit/packages/kit/package.json:L3`).

## Key facts

### Module and file rules

- A remote module is any file with a `remote` filename segment (`data.remote.ts`,
  `remote.ts`, `x.remote.test.ts`); it can live anywhere in `src` except inside a `server`
  directory, and third-party packages with `@sveltejs/kit` in `peerDependencies` can ship
  them. On the client every export becomes a `fetch` wrapper hitting a generated endpoint.
  DOCS `reference/kit/documentation/docs/20-core-concepts/60-remote-functions.md:L38`;
  `60-appendix/35-migrating-to-sveltekit-3.md:L530-L534`.
- Four kinds: `query` (with `.batch` and `.live` variants), `form`, `command`, `prerender`.
  DOCS `60-remote-functions.md:L38`.
- Both flags required (`experimental.remoteFunctions`, `compilerOptions.experimental.async`).
  VERIFIED-LIVE `junkyard/.agents/skills/sveltekit-current/SKILL.md:L78-L82`.
- You **cannot export a schema from a `.remote.ts` file** — schemas shared with `preflight`
  must come from another module or a `<script module>`. DOCS `60-remote-functions.md:L664`.

### Validation argument (all kinds)

- First positional arg is the validator; forms: no validator (no-argument function),
  `'unchecked'` (TS type only, no runtime check), or a Standard Schema v1 object (valibot,
  zod, arktype…). VERIFIED-LIVE `SKILL.md:L91-L92`; DOCS `60-remote-functions.md:L137-L163`,
  `L1256-L1266`.
- Argument and return are serialized with **devalue** (Date, Map, Set, BigInt, RegExp,
  cycles) plus any `transport` codecs. DOCS `60-remote-functions.md:L165`.
- **Cache-key normalization**: for `query` and `prerender` *arguments* (not return values),
  objects/maps/sets are sorted so `{limit:10, offset:10}` and `{offset:10, limit:10}` share a
  key; arrays preserve order. DOCS `60-remote-functions.md:L167`.
- Validation failure → generic `400 Bad Request`; passes through server `handleError` with
  `kind: 'validation'`, `error: { status: 400, message: 'Bad Request' }`, `issues`. Issues are
  not exposed unless the hook returns them. DOCS `60-remote-functions.md:L1232-L1254`;
  `30-advanced/20-hooks.md:L165-L168`.
- Type note: `RemoteQueryFunction<Input, Output, _Validated = Input>` — `Validated` is the
  post-schema (possibly transformed) argument type the implementation receives; it's also what
  `requested()` yields. `public.d.ts:L425-L450`.

### `query`

- `query(fn)`, `query(schema, fn)`, `query('unchecked', fn)`. Returns
  `RemoteQueryFunction<Input, Output>`; calling it yields `RemoteQuery<Output>` which is a
  `Promise<Output>` **and** a reactive resource: `error: App.Error | undefined`, `loading`,
  `current` (with `ready` discriminant), plus `set(value)`, `refresh(): Promise<void>`,
  `withOverride(fn): RemoteQueryOverride`. `public.d.ts:L351-L404`; DOCS
  `60-remote-functions.md:L40-L117`, `L193-L203`.
- **Deduplication**: the serialized argument is the cache key. Server: request-scoped cache,
  so N identical calls in one request run once. Client: identical calls share one instance
  (`getPosts() === getPosts()`); cache lives while anything uses it. DOCS
  `60-remote-functions.md:L169-L191`, `L203`.
- Queries cannot be used on fully prerendered pages. DOCS `60-remote-functions.md:L44`.
- Inside `query` (and `.batch`/`.live`) accessing `event.route`/`params`/`url` **throws** —
  pass page values as arguments. `handle` for a query request sees the remote endpoint's URL.
  DOCS `60-remote-functions.md:L1310`; `35-migrating-to-sveltekit-3.md:L536-L538`.
- Cannot set headers inside any remote function except cookies, and cookies only inside
  `form`/`command`. DOCS `60-remote-functions.md:L1309`.
- `redirect()` allowed inside `query`. DOCS `60-remote-functions.md:L1313-L1315`.

### `query.batch`

- `query.batch(schema, async (inputs: Input[]) => (input: Input, index: number) => Output)`.
  Batches calls in the same macrotask; server callback receives the **array** of arguments and
  must return a resolver function kit invokes per input. DOCS `60-remote-functions.md:L205-L256`.
  ```js
  export const getWeather = query.batch(v.string(), async (cityIds) => {
    const lookup = new Map(...);
    return (cityId) => lookup.get(cityId);
  });
  ```

### `query.live`

- `query.live(async function* () { yield ...; })` — callback returns an `AsyncIterable`.
  Client type `RemoteLiveQuery<T>` = resource + `AsyncIterable<T>` + `connected`, `done`,
  `reconnect(): Promise<void>`. **No `refresh()`** (self-updating). `public.d.ts:L406-L414`;
  DOCS `60-remote-functions.md:L258-L317`.
- During SSR, `await getTime()` returns the first yield then closes the iterator (irrelevant
  for CSR-only). On the client the stream stays connected while used; multiple consumers share
  one connection; when unused it disconnects and **server iteration is stopped**. Passive
  reconnect with exponential backoff; active on `navigator.onLine` false→true. DOCS
  `60-remote-functions.md:L273-L291`.
- Consumer semantics: `for await` over the instance; first value is the most-recent cached
  one; **only the latest pending value is kept** if the consumer is slow ("live streams are
  not event logs"). Server-side `for await` joins a per-request shared iteration. DOCS
  `60-remote-functions.md:L295-L315`.
- Responses carry `Cache-Control: no-store`; service workers must not cache them. DOCS
  `60-remote-functions.md:L317`.
- Mutations can `.reconnect()` a live query in the same flight (`getNotifications(userId).reconnect()`
  inside a `form`/`command` handler); `requested(liveQuery, n).reconnectAll()`. DOCS
  `60-remote-functions.md:L1030-L1051`; `public.d.ts:L493-L509`.

### `command`

- `command(schema, fn)`; callable anywhere **except during render**. Client type
  `RemoteCommand<Input, Output>`: `(arg) => Promise<Output> & { updates(...RemoteQueryUpdate[]) }`
  plus `pending: number`. `public.d.ts:L333-L342`; DOCS `60-remote-functions.md:L909-L974`.
- By default a command invalidates **nothing**; the handler must refresh explicitly
  (server-driven `void getPosts().refresh()` / `getPost(id).set(result)`) or the client must
  request updates (`await addLike(id).updates(getLikes(id))`). DOCS
  `60-remote-functions.md:L976-L1028`, `L1053-L1077`.
- **`redirect()` is NOT allowed inside `command`**; return `{ redirect: location }` and handle
  on the client. DOCS `60-remote-functions.md:L1313-L1315`.
- Cookies may be written inside `command`. DOCS `60-remote-functions.md:L1309`.

### `form`

- `form(schema, async (data, issue) => ...)` where `data` is built from `FormData` and
  coerced by field-name prefixes (`b:` boolean, `n:` number) before validation. Nested names
  use JS object notation (`nested.array[0].value`); quoted keys unsupported. DOCS
  `60-remote-functions.md:L319-L376`, `L421`.
- The returned `RemoteForm<Input, Output>` is spread onto `<form {...createPost}>`: it has
  `method: 'POST'`, `action: string`, and a symbol-keyed **attachment** that progressively
  enhances (submits without reload when JS is available); without JS the browser POSTs to
  `action` and the page reloads. `public.d.ts:L276-L331`; DOCS `60-remote-functions.md:L376-L395`.
- Other instance members: `element`, `submit(): Promise<boolean> & { updates(...) }`,
  `enhance(cb)`, `for(id)`, `preflight(schema)`, `validate({ all?, preflightOnly? })`,
  `result: Output | undefined` (ephemeral), `pending: number`, `submitted: boolean`,
  `fields`. `public.d.ts:L276-L331`.
- **Fields API**: `fields.<path>.as(type, value?)` returns `{ name, type, 'aria-invalid',
  value/checked/files getters+setters, defaultValue/defaultChecked }`; `value()`, `set()`,
  `touched()`, `dirty()`, `issues(): { message, path }[] | undefined`; containers add
  `allIssues()`. `radio`, `submit`, `hidden` always need the second `value` arg; `checkbox`
  needs it when one of an array field; `file` can't be prefilled. Field values may be
  `string | string[] | number | boolean | File | File[]`, nested in objects/arrays.
  `public.d.ts:L10-L212`; DOCS `60-remote-functions.md:L397-L558`.
- Unchecked checkboxes and empty multi-selects are **absent** from `FormData` → make them
  optional in the schema. DOCS `60-remote-functions.md:L480`, `L558`.
- Server behavior: if the schema fails, the handler does not run; `fields.x.issues()` and
  `aria-invalid` populate. `invalid(issue.qty('msg'), 'form-level string')` from
  `@sveltejs/kit` throws like `error`/`redirect` to raise issues programmatically. DOCS
  `60-remote-functions.md:L560-L632`.
- Sensitive values: fields whose name begins with `_` are not echoed back on a failed no-JS
  submission. DOCS `60-remote-functions.md:L711-L733`.
- Returns and redirects: handler may `redirect(303, ...)` or return data → `form.result`;
  an error renders the nearest `+error.svelte`. **After a successful `form` submission kit
  refreshes ALL queries and loads by default** unless the handler does single-flight work.
  DOCS `60-remote-functions.md:L735-L799`, `L978`.
- Multiple instances: `form.for(id)` (id typed from schema `id` field);
  `as('submit', value)` for multiple buttons. DOCS `60-remote-functions.md:L835-L907`.
- `enhance(async (form) => { if (await form.submit()) form.element.reset(); })` — enhanced
  forms are not auto-reset. DOCS `60-remote-functions.md:L801-L833`.
- Manual `name=` attributes cause the submission to be **rejected**. DOCS
  `35-migrating-to-sveltekit-3.md:L544-L552`.

### Single-flight mutations

- Inside a `form`/`command` handler: `void getPosts().refresh()`,
  `getPost(id).set(result)`, `liveQuery(arg).reconnect()` — the framework awaits them and
  ships the refreshed data with the mutation response. Keys are argument-based, so
  `getPosts().refresh()` only hits the no-argument client instance. DOCS
  `60-remote-functions.md:L976-L1028`; VERIFIED-LIVE `SKILL.md:L97-L99` (one programmatic
  request observed).
- Client-requested refreshes: `submit().updates(getPosts, getPosts({filter}),
  getPosts({filter}).withOverride(fn))` — the server must **accept** them via
  `requested(getPosts, limit)` (iterable of `{ arg, query }`; `.refreshAll()` shorthand).
  `limit` is mandatory as a DoS bound; a malformed requested arg errors that entry, not the
  command. Unrequested refreshes are silently not sent (bundle-size + DoS rationale). DOCS
  `60-remote-functions.md:L1053-L1126`; `public.d.ts:L452-L513`.
- `RemoteQueryUpdate` = query instance | live instance | query function | live function |
  override. `public.d.ts:L344-L349`.

### `prerender`

- `prerender(fn)`, `prerender(schema, fn, { inputs?: () => Input[], dynamic?: boolean })`.
  Invoked at **build time**; client caches results in the `Cache` API, cleared on a new
  deployment. Crawler-found calls are saved automatically; `inputs` lists extra arguments;
  `dynamic: true` keeps the function in the server bundle so un-prerendered arguments work at
  runtime. `RemotePrerenderFunction<Input, Output> = (arg) => RemoteResource<Output>`.
  DOCS `60-remote-functions.md:L1128-L1230`; `public.d.ts:L418-L423`.
- Runtime trap: during SSR kit **fetches** `<origin>/_app/remote/<hash>/<fn>` over HTTP from
  its own origin; if that fails the stub throws. VERIFIED-LIVE `SKILL.md:L65-L70`.
- `redirect()` allowed in `prerender`. DOCS `60-remote-functions.md:L1313`.

### `getRequestEvent`

- Available in `query`, `form`, `command` (and server loads/actions). Returns the
  `RequestEvent`; typical use is a request-deduped `getUser` query reading
  `cookies.get('session_id')`. DOCS `60-remote-functions.md:L1268-L1305`;
  `20-core-concepts/20-load.md:L714-L780`.
- Property caveats: no header setting (cookies only in `form`/`command`); `route`/`params`/
  `url` throw in queries; in `form`/`command` they describe the **calling page** and are
  attacker-controlled — never authorize on them. DOCS `60-remote-functions.md:L1307-L1311`.

## Citations

- `reference/kit/documentation/docs/20-core-concepts/60-remote-functions.md:L1-L1315`
- `reference/kit/packages/kit/src/runtime/app/server/public.d.ts:L1-L513`
- `junkyard/.agents/skills/sveltekit-current/SKILL.md:L65-L70`, `L78-L99`
- `reference/kit/documentation/docs/60-appendix/35-migrating-to-sveltekit-3.md:L505-L552`
- `reference/kit/documentation/docs/30-advanced/20-hooks.md:L36`, `L165-L168`
- `reference/kit/documentation/docs/20-core-concepts/10-routing.md:L57-L72`
- `junkyard/README.md:L114-L131` (generator shape that existed: `remote.Query`/`Command`/
  `Live` in `*.remote.go`, `.remote.ts` shims generated, kit build run to capture real
  addresses)

## skgo implications

### Which kinds are CSR-viable (no SSR, no Node in production)

| Kind | CSR-only viable | Why / what Go must do |
|---|---|---|
| `query` | yes | Client always fetches; Go answers the GET, request-scoped dedupe in Go (memoize per request+key). |
| `query.batch` | yes | Client sends one batched request per macrotask; Go handler receives `[]Input`, returns per-input results in order. |
| `query.live` | yes | Client-only stream; Go streams values (channel/iterator), stops iteration when the client disconnects, sets `Cache-Control: no-store`. SSR first-yield semantics don't apply. |
| `command` | yes | Go POST handler; no redirects; must support `updates` (client-requested refresh list) and server-driven refresh/set in the same response. |
| `form` (enhanced) | yes | Go handles the multipart/urlencoded POST with the `b:`/`n:` prefix coercion, returns issues/result plus single-flight refreshes. |
| `form` (no-JS fallback) | **effectively no** | With `ssr = false` the browser's plain POST would need the server to re-render the page with `result`/issues; there is no renderer. Go can still accept the POST and redirect (303) so nothing breaks, but no-JS display of issues is out. Decide explicitly. |
| `prerender` | partial | Needs a build-time executor. Either Go runs the function at `skgo build` and writes `/_app/remote/<hash>/<fn>[…]` files that Go serves as static, or skgo declares `prerender` unsupported for Go units. `dynamic: true` means Go also answers at runtime. |

### Generator contract (Go → TS)

- For each Go unit emit into the sibling `.remote.ts`: the kind, the export name, the
  validation shape, and a body that **throws** ("Go remote function executed in JS"). The
  client transform only needs kind + name to build the fetch wrapper, so the throw is safe.
  Kit's build assigns the endpoint hash/name; skgo must **read the built server manifest
  to learn each function's address** (the junkyard's `skgo remotes build` ran the kit
  production build "to capture Kit's real addresses", `junkyard/README.md:L119-L125`).
- Validation must also exist on the TS side for `form.preflight` and for `command`/`query`
  type inference: emit a Standard Schema (valibot is what the docs and junkyard use) or
  `'unchecked'` plus a TS type. Go is the source of truth for the type (junkyard interview
  0004, recommendation option 1); the junkyard generated schemas from Go types via
  `go-gen-jsonschema` registrations (`junkyard/README.md:L116-L120`).
- Types to emit/import: `RemoteQueryFunction<I,O>`, `RemoteQueryFunction` for batch,
  `RemoteLiveQueryFunction<I,O>`, `RemoteCommand<I,O>`, `RemoteForm<I,O>`,
  `RemotePrerenderFunction<I,O>` — all from `$app/server`.
- Go-side registration must know each function's **cache-key normalization** rules (sorted
  object keys for query/prerender args) so server-driven `refresh()`/`set()` target the same
  key the client computed.
- Go must implement `requested(fn, limit)` semantics: parse the client's refresh list, bound
  it, validate each argument independently, and include refreshed values in the response.
- Go must implement `invalid()` (issues with `{ message, path }`) and `redirect()` (303 for
  forms; forbidden for commands) and `error()` (App.Error with status) with kit-compatible
  encodings.
- `getRequestEvent()` maps to a Go request context carrying `cookies` (read + write for
  form/command only), `locals`, `request`, `clientAddress`; expose **no** `route`/`params`/
  `url` for queries (panic or compile-time absence), and for form/command expose them
  labeled untrusted.
- Server-side dedupe for `query` is per request; the generated Go handler should memoize by
  (function, normalized arg) within one request scope so a Go `getUser` helper called from
  several units runs once.

### Wire protocol (not in this segment)

- Endpoint path shape `/_app/remote/<hash>/<fn>` (VERIFIED-LIVE `SKILL.md:L66`); argument
  encoding, response envelopes (result + refreshes), live-stream framing, batch request/
  response, the binary form encoding, and `__data.json` are unpublished — junkyard rule
  `junkyard/AGENTS.md:L33-L36`. Read kit source under `reference/kit/packages/kit/src/runtime`
  (`app/server/remote/*.js`, `client/remote-functions/*`) and match the pinned version
  exactly; re-validate on every kit bump.

## Gotchas

- `form` refreshes **everything** after success by default; `command` refreshes **nothing**.
  A Go `command` that mutates and forgets to refresh leaves stale UI; a Go `form` that
  doesn't do single-flight costs a second round-trip.
- `redirect()` in a `command` is a bug on kit's side (not allowed); the generator should
  reject `command` units that return redirects or map them to `{ redirect }` data.
- `query` handlers that read `url`/`params`/`route` throw at runtime — a Go query must not be
  handed the page URL; make the generator refuse such a signature.
- `event.route/params/url` inside `form`/`command` are the **calling page's** and are
  user-controlled; Go must never derive authorization from them.
- Docs say `validate({ includeUntouched: true })` (`60-remote-functions.md:L642`); the type
  says `validate({ all?: boolean; preflightOnly?: boolean })` (`public.d.ts:L312-L322`).
  Trust the type when writing the example app.
- `checkbox`/`select multiple` absent from FormData when empty → optional with default in
  the schema, and the Go struct field must tolerate absence.
- Prerendered remote results are **served files**; if Go serves the bundle it must serve
  them at the exact path kit's client computes, including argument-hashed variants.
- Live query iteration continues server-side until the client disconnects; Go must tie the
  generator/channel lifetime to the request context and cancel on disconnect.
- `query.live` values that arrive faster than consumed are dropped on the client (latest
  wins). Don't build event logs on it.

## Recipes

- **Query with validation (TS reference shape the generator must emit)**
  ```ts
  import * as v from 'valibot';
  import { query } from '$app/server';
  export const getPost = query(v.string(), async (slug) => { throw new Error('go-only'); });
  ```
- **Command with single-flight refresh (Go handler must reproduce)**
  ```ts
  export const addLike = command(v.string(), async (id) => {
    await db.like(id);
    void getLikes(id).refresh();   // framework awaits it; ships in same response
  });
  ```
  Client: `await addLike(item.id)` — one request, `getLikes(item.id)` updates in place.
- **Client-requested refresh** (Go must honour `requested` with a limit)
  ```ts
  await createPost.submit().updates(getPosts({ filter }));
  // server: for (const { query } of requested(getPosts, 1)) void query.refresh();
  ```
- **Form in a CSR-only page**
  ```svelte
  <form {...createPost.enhance(async (f) => { if (await f.submit()) f.element.reset(); })}>
    {#each createPost.fields.title.issues() ?? [] as i}<p>{i.message}</p>{/each}
    <input {...createPost.fields.title.as('text')} />
    <button>Publish</button>
  </form>
  ```
- **Live query consumption**: `const t = getTime(); <p>{await t}</p> <p>{t.connected}</p>`.
- **Reading cookies in a Go-backed query**: model as `getRequestEvent().cookies.get(name)`;
  in Go, a `Cookies` accessor on the request context, read-only for queries.
