# Junkyard guestbook — the app

**Purpose.** A deliberately small Kit 3 "guestbook" whose every route exists to exercise one server-facing feature (loads, form actions, remote functions in all four flavours, transport codecs, streaming, errors, redirects, platform locals). It is the fixture the Playwright suite runs against and the natural fixture for skgo's Go backend to reimplement.

## Key concepts

### Shell and hooks
- **`app.html`** is stock kit: `%sveltekit.head%` / `%sveltekit.body%` inside `<div style="display: contents">` — `junkyard/app/src/app.html:L1-L12`. Favicon at `/favicon.svg` from `static/`.
- **`App.Locals` = `{ user: string | null; requestId: string }`, `App.Error` = `{ message; code? }`** — `junkyard/app/src/app.d.ts:L1-L14`. `code` is the app-defined error discriminator (`TEAPOT`, `NOT_FOUND`, `VALIDATION`, `INTERNAL`).
- **`hooks.server.ts`** — `junkyard/app/src/hooks.server.ts:L1-L21`:
  - `handle` reads cookie `session` into `locals.user`, mints `locals.requestId = crypto.randomUUID()`, and stamps response header `x-guestbook-request-id` (`L3-L10`).
  - `handleError` is the kit-3 shape with a `kind` discriminator: `kind === 'validation'` returns `issues[0].message` with `code: 'VALIDATION'`; `kind === 'unknown'` logs and returns the generic `"Something went wrong on our side."` with `code: 'INTERNAL'` (`L12-L21`). Types come from `@sveltejs/kit/hooks`, not `@sveltejs/kit`.
- **`hooks.ts` (universal) hosts `transport`** — `junkyard/app/src/hooks.ts:L1-L10`. `Money` encodes as `[cents, currency]` and decodes back to a class instance; `encode` returns `false` for non-Money values (kit's "not mine" signal). This is what makes `data.price instanceof Money` true on the client.
- **`lib/money.ts`** — plain class with `format()` → `"USD 19.99"` (`junkyard/app/src/lib/money.ts:L1-L10`).
- **`lib/server/store.ts`** — in-memory message store seeded with one message `{id:1, author:'kit', text:'welcome to the guestbook'}` (`junkyard/app/src/lib/server/store.ts:L9-L11`); `list()` returns newest-first (`L17-L19`), `add()` bumps a `version` and wakes watchers (`L23-L35`), `changed()` returns a one-shot promise resolved on next add (`L43-L51`) — the primitive behind the live query.

### Routes (what each one exercises)
| Route | Server piece | Client piece | Exercises |
|---|---|---|---|
| `/` (layout) | `+layout.server.ts` reads/sets `visits` cookie (httpOnly, lax, path=/), returns `{user, visits}` — `junkyard/app/src/routes/+layout.server.ts:L3-L8` | `+layout.svelte` nav + `data-testid` `user`/`visits`/`hydrated`; `hydrated` flips to `yes` in `$effect` from `browser` — `junkyard/app/src/routes/+layout.svelte:L2-L26` | layout load composition, cookie set from a load, hydration sentinel |
| `/` | `+page.server.ts` → `{greeting, messageCount: store.count()}` — `routes/+page.server.ts:L4-L7` | `greeting`, `message-count`, `<img src="/logo.svg">` — `routes/+page.svelte:L5-L7` | PageServerLoad, static asset |
| `/about` | `+page.ts` universal load, `export const prerender = true`, `builtAt: 'build time'` — `routes/about/+page.ts:L3-L5` | `built-at` — `routes/about/+page.svelte:L6` | prerendered page + trailing-slash 308 |
| `/messages` | `messages.remote.ts` (see below) | top-level `await` on `getBanner()`, `getMessageCount()` (live), `getMessages()`; `postMessage` via `onsubmit`; `attachFile` spread onto a multipart `<form>` with `.fields.x.as('text'|'file')` and `.result` — `routes/messages/+page.svelte:L2-L54` | query, query.live, command, form (with file), prerender remote |
| `/messages/[id]` | `+page.server.ts` → `store.get(Number(params.id))`, else `error(404, {message:'no message #N', code:'NOT_FOUND'})` — `routes/messages/[id]/+page.server.ts:L6-L10` | `message-title`, `message-author`, `message-text` — `routes/messages/[id]/+page.svelte:L5-L8` | dynamic param, expected 404 from load |
| `/prices` | `+page.server.ts` → `{price: new Money(1999,'USD')}` — `routes/prices/+page.server.ts:L4-L6`; `prices.remote.ts` `getSalePrice = query(() => new Money(1499,'USD'))` — `routes/prices/prices.remote.ts:L5` | `price`, `price-is-money`, `sale-price`, `sale-price-is-money` (`instanceof Money`) — `routes/prices/+page.svelte:L10-L17` | transport codec through a load AND a remote query |
| `/slow` | `+page.server.ts` returns `{fast, slow: Promise resolving after 1500ms}` — `routes/slow/+page.server.ts:L3-L6` | `{#await data.slow}` → `loading…` then value — `routes/slow/+page.svelte:L7-L11` | streamed (promise-valued) load data |
| `/whoami` | `+page.server.ts` returns `url.href`, `getClientAddress()`, `locals.requestId`, `locals.user`, `request.headers.get('host')` — `routes/whoami/+page.server.ts:L3-L9` | `url`, `client-address`, `request-id`, `whoami-user`, `host` — `routes/whoami/+page.svelte:L6-L17` | platform surface a Go server must expose to a load |
| `/login` | `+page.server.ts` load `{user, loggedOut: url.searchParams.has('bye')}`; actions `login` (`fail(400,{name,error:'name is required'})` on empty, else set `session` cookie + `redirect(303,'/')`) and `logout` (delete cookie + `redirect(303,'/login?bye')`) — `routes/login/+page.server.ts:L4-L23` | `use:enhance` forms with `action="?/login"` / `?/logout`; shows `form.error`, `data.loggedOut` — `routes/login/+page.svelte:L9-L30` | classic named form actions, `fail`, cookie auth, redirects |
| `/redirect` | `redirect(307, '/messages')` from load — `routes/redirect/+page.server.ts:L4-L6` | none | redirect thrown in a load |
| `/error/expected` | `error(418, {message:'expected teapot', code:'TEAPOT'})` — `routes/error/expected/+page.server.ts:L4-L6` | `+error.svelte` | expected error → status + message |
| `/error/unexpected` | `throw new Error('SECRET-INTERNAL-DETAIL: database password is hunter2')` — `routes/error/unexpected/+page.server.ts:L3-L5` | `+error.svelte` | unexpected error → 500, message scrubbed by `handleError` |
| `/api/echo` | `+server.ts` `POST` returns `{bytes: body.byteLength}` — `routes/api/echo/+server.ts:L4-L7` | none | endpoint + body-size limit (413 over ~512 KiB) |
| `+error.svelte` | — | `error-status` = `page.status`, `error-message` = `page.error?.message` from `$app/state` — `routes/+error.svelte:L1-L6` | error page contract |

### Remote functions (`messages.remote.ts`)
`junkyard/app/src/routes/messages/messages.remote.ts:L1-L40`, imports from `$app/server` (`command, form, getRequestEvent, prerender, query`) and `valibot`:
- `getMessages = query(() => store.list())` (`L6`).
- `getMessageCount = query.live(async function* () { while (true) { yield store.count(); await Promise.race([store.changed(), timeout 2000]) } })` (`L8-L14`) — a server-push generator; the timeout is there to release dropped clients.
- `postMessage = command(v.pipe(v.string(), v.nonEmpty()), async (text) => { locals via getRequestEvent(); store.add(...); void getMessages().refresh(); return message.id })` (`L16-L22`) — the `refresh()` inside the command is the **single-flight** mechanism: the refreshed query result rides back in the command response, so the client makes exactly one request.
- `attachFile = form(v.object({text: nonEmpty, attachment: v.file()}), async ({text, attachment}) => {...; void getMessages().refresh(); return {id, name, size}})` (`L24-L38`) — multipart remote form with a `File` field, also single-flight.
- `getBanner = prerender(() => ({text: 'guestbook — prerendered banner'}))` (`L40`) — a build-time remote function; its response is a static file, not a runtime call.

## Citations
- `junkyard/app/src/hooks.server.ts:L3-L10` (locals + response header), `:L12-L21` (handleError kinds)
- `junkyard/app/src/hooks.ts:L5-L10` (transport)
- `junkyard/app/src/lib/server/store.ts:L16-L52`
- `junkyard/app/src/routes/messages/messages.remote.ts:L6-L40`
- `junkyard/app/src/routes/messages/+page.svelte:L21-L22`, `:L45-L54`
- `junkyard/app/src/routes/login/+page.server.ts:L9-L23`
- `junkyard/app/src/routes/whoami/+page.server.ts:L3-L9`
- `junkyard/app/src/routes/+layout.server.ts:L3-L8`, `junkyard/app/src/routes/+layout.svelte:L7-L9`

## Reusable verdicts
- Route tree + `.svelte` pages + `data-testid` contract — **REUSE AS-IS**. They are the UI the acceptance suite targets; nothing in the markup is SSR-specific except that `{#await}` blocks will start in the pending state under CSR.
- `messages.remote.ts` (query / query.live / command / form / prerender) — **REUSE WITH CHANGES**: the TS becomes a generated throwing stub; the bodies (store ops, valibot schemas, `refresh()` single-flight, live generator) become the Go spec. The **valibot schema is the shape the Go validator must reproduce**, including the `'text is required'` message.
- `+page.server.ts` / `+layout.server.ts` loads — **REUSE WITH CHANGES**: same — bodies move to `page.server.go` / `layout.server.go`; the TS becomes a stub. `whoami` is the spec for what `RequestEvent` a Go load must provide (`url.href`, client address, host header, locals).
- `hooks.server.ts` — **REUSE WITH CHANGES**: `handle` (cookie → locals.user, request id, response header) and `handleError` (kind-based scrubbing) must be re-expressed in Go middleware; the TS file itself has no role when there is no Node server.
- `hooks.ts` `transport` — **REUSE AS-IS on the client side**; the Go side must emit the same `[cents, currency]` encoding under the `Money` key in devalue/transport output, and decode it from command/form arguments.
- `lib/server/store.ts` — **REUSE WITH CHANGES**: port to Go (in-memory slice + version + watcher channel). The `changed()` one-shot promise maps naturally onto a Go channel/`sync.Cond`.
- `login/+page.server.ts` classic actions + `use:enhance` — **REUSE WITH CHANGES**: Go must implement `?/login` and `?/logout` action POSTs with kit's action response envelope (fail/redirect), and the `enhance` client still handles them.
- `about/+page.ts` universal load — **REUSE AS-IS**: universal loads run in the browser under CSR; `prerender = true` may or may not be honoured by the skgo adapter (see csr-portability.md).
- `api/echo/+server.ts` — **REUSE WITH CHANGES**: becomes a Go handler; the 413 limit must be Go's own body limit.

## Gotchas
- **`#lib` not `$lib`** everywhere (`hooks.ts:L3`, `+page.server.ts:L1`, `messages.remote.ts:L4`). Backed by `package.json` `imports`.
- **`transport` lives in universal `hooks.ts`**, not `hooks.server.ts` (`junkyard/app/src/hooks.ts:L1-L5`); the type is imported from `@sveltejs/kit/hooks`.
- **`handleError` receives `{ kind, error, issues }`** in kit 3 (`hooks.server.ts:L12`) — `kind` is `'validation' | 'unknown' | ...`; `issues` is present for validation failures of remote-function schemas. Older shapes (`{error, event, status, message}`) are wrong.
- **`$app/state` not `$app/stores`** in `+error.svelte` (`routes/+error.svelte:L2`).
- **`$effect` + `browser` hydration sentinel** (`+layout.svelte:L5-L9`): tests wait on `hydrated === 'yes'` before counting requests; under CSR this flips almost immediately, which weakens some assertions (see e2e-suite.md).
- **Top-level `await` in markup** (`messages/+page.svelte:L21,L30`) requires `compilerOptions.experimental.async` in `vite.config.ts:L12-L14`.
- **`getRequestEvent()` inside remote functions** (`messages.remote.ts:L17,L30`) is how remote functions see `locals`/cookies; the Go equivalent needs the same per-request context available to remote handlers.
- **Form remote functions spread onto `<form>`** (`{...attachFile}`) and fields via `.fields.name.as('file')` (`messages/+page.svelte:L45-L47`); `attachFile.result` is the last response. The no-JS fallback requires the server to answer a plain multipart POST with a full page (SSR) — impossible without SSR.
- **`query.live` is a server-push stream** (`messages.remote.ts:L8-L14`); the wire protocol is kit's (long-lived response). Go must speak the same framing, or the live test must be replaced.

## Recipes
- To define the Go fixture app's behaviour, read the route table above, then each server file it points at (start at `junkyard/app/src/routes/messages/messages.remote.ts:L6` and `junkyard/app/src/routes/login/+page.server.ts:L9`).
- To reproduce cookie semantics: `+layout.server.ts:L4-L5` (visits), `login/+page.server.ts:L16,L20` (session set/delete; `path:'/'`, `httpOnly`, `sameSite:'lax'`).
- To reproduce the error scrubbing contract: `hooks.server.ts:L12-L21` + `error/unexpected/+page.server.ts:L4` + `+error.svelte:L5-L6`.
- To reproduce the transport wire format: `hooks.ts:L6-L9` and the consumer `prices/+page.svelte:L10-L17`.
- To reproduce the request-id header and locals: `hooks.server.ts:L4-L8` and `whoami/+page.server.ts:L3-L9`.
