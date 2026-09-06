# Loads, page options, and routing: the contract skgo's page/layout server-load generator must expose

## Purpose

skgo lets page and layout **server** loads be written in Go. This file records the kit 3
`load` contract (inputs, outputs, error/redirect, streaming, rerun rules), the page options
that interact with a CSR-only app (`ssr = false`), and the route-matching rules Go must
implement or respect when it serves the built bundle and answers data requests. Universal
loads (`+page.ts`/`+layout.ts`) stay plain kit and run only in the browser under `ssr =
false`; they are noted only where they change the server contract.

Citation paths are relative to `/Users/tyler/src/skgo/ephemeral/inspiration/`.

## Key facts

### Which load runs where

- `+page.js`/`+layout.js` = **universal** (server during SSR, then browser); with
  `export const ssr = false` they **only run in the browser**. `+page.server.js`/
  `+layout.server.js` = **server** loads, always on the server. If both exist, the server
  load runs first and its return value is the universal load's `data` argument. DOCS
  `reference/kit/documentation/docs/20-core-concepts/20-load.md:L47`, `L174-L199`, `L213-L235`.
- A load is invoked at runtime unless the page is prerendered (then at build time). DOCS
  `20-load.md:L191`.
- All loads for a page run **concurrently**; during client navigation the results of multiple
  server loads are **grouped into a single response**. DOCS `20-load.md:L568-L570`.
- Types: `PageServerLoad`, `LayoutServerLoad` from `./$types`; page components take
  `PageProps` (`data`, `form`, `params`), layouts `LayoutProps` (`data`, `children`). DOCS
  `20-load.md:L9-L33`, `L62-L70`, `L87-L99`; `98-reference/54-types.md:L26-L99`;
  `10-routing.md:L57-L72`.

### Server load input (`ServerLoadEvent`)

- `params`, `route` (`{ id }` like `/a/[b]/[...c]`), `url` (`URL`; `url.hash` unavailable;
  origin may need adapter config), `fetch`, `setHeaders`, `parent`, `depends`, `untrack`,
  plus from `RequestEvent`: `clientAddress`, `cookies`, `locals`, `platform`, `request`.
  DOCS `20-load.md:L193-L199`, `L237-L270`.
- `params` derived from `url.pathname` + `route.id`: for `/a/[b]/[...c]` and `/a/x/y/z` →
  `{ b: 'x', c: 'y/z' }`. Param matchers can transform types (number/boolean/bigint). DOCS
  `20-load.md:L259-L270`; `30-advanced/10-advanced-routing.md:L96-L116`.
- `cookies.get/set/delete`; kit supplies `httpOnly`, `secure`, and (kit 3) `path: '/'`
  defaults. Only same-host or subdomain fetches forward cookies. DOCS `20-load.md:L293-L326`;
  `60-appendix/35-migrating-to-sveltekit-3.md:L358-L368`.
- `setHeaders({...})` sets response headers on the server only; a header may be set **once**
  across all loads; **cannot set `set-cookie`** (use `cookies.set`). DOCS `20-load.md:L328-L351`.
- `await parent()` in a server load returns merged data from parent `+layout.server.js`
  files; call it after independent work to avoid waterfalls; it also makes the load rerun
  when the parent reruns. DOCS `20-load.md:L353-L420`, `L618`.
- `locals` is populated in `handle` and is where trusted auth context lives. DOCS
  `30-advanced/20-hooks.md:L66-L103`; `20-load.md:L440-L451`.
- `getRequestEvent()` (from `$app/server`) returns the same event inside helpers called from a
  load (e.g. `requireLogin()` that throws `redirect(303, '/login?redirectTo=…')`). DOCS
  `20-load.md:L714-L780`.

### Server load output

- Must be **devalue-serializable**: JSON plus `BigInt`, `Date`, `Map`, `Set`, `RegExp`,
  repeated/cyclic refs, plus custom types via the universal `transport` hook. DOCS
  `20-load.md:L205`; `20-hooks.md:L361-L379`.
- Same key from multiple loads: **last (deepest) wins** — layout `{a,b}` + page `{b,c}` →
  `{a, b(page), c}`. DOCS `20-load.md:L153`.
- **Streaming with promises**: top-level promise values in a *server* load are streamed as
  they resolve; the component uses `{#await data.comments}`. Attach a noop `.catch` to any
  promise that may reject before render starts or the server can crash on an unhandled
  rejection. Headers/status can't change once streaming begins (no `setHeaders`/`redirect`
  inside a streamed promise). Streaming only works with JS enabled; proxies must not buffer.
  Top-level promises are streamed (kit 2+ behaviour, not awaited). DOCS `20-load.md:L493-L566`.
- Parent layouts can read page data via `page.data` from `$app/state` (typed by
  `App.PageData`). DOCS `20-load.md:L155-L172`.

### Errors and redirects from a load

- `error(status, message, extras?)` from `@sveltejs/kit` throws; nearest `+error.svelte`
  renders (`page.status`, `page.error.message`); for a layout load the boundary is the
  `+error.svelte` *above* that layout; root layout errors fall to `src/error.html`. Every
  error reaches `handleError`. DOCS `20-load.md:L422-L458`; `10-routing.md:L149-L170`;
  `25-errors.md:L130-L169`.
- `redirect(3xx, location)` throws; don't wrap in `try`; external targets need
  `{ external: true }`. DOCS `20-load.md:L460-L491`; `35-migrating-to-sveltekit-3.md:L569-L577`.

### When loads rerun (client-side dependency tracking)

- A load reruns when: a referenced `params.x` changes; a referenced `url` property changes
  (`request.url` is not tracked); `url.searchParams.get/getAll/has(x)` and `x` changes
  (search params tracked independently); it called `await parent()` and the parent reran (or
  a child reran and the parent is a server load); it declared a dependency with `fetch(url)`
  (universal only) or `depends('app:x')` and `invalidate('app:x')` was called; `refreshAll()`.
  Tracking stops once the function returns (don't read `params` inside a nested promise).
  `untrack(() => …)` opts out. DOCS `20-load.md:L572-L698`.
- `invalidate(url | 'a-z:…' | predicate)`, `refreshAll()` (does not reset `page.state`;
  reruns every load and all active queries). `invalidateAll` is deprecated. DOCS
  `20-load.md:L639-L682`; `35-migrating-to-sveltekit-3.md:L130-L134`.
- Server loads **never** auto-depend on a fetched URL (no secret leakage). DOCS
  `20-load.md:L641`.
- Rerunning updates `data` in place; component state is preserved unless keyed. DOCS
  `20-load.md:L698`.
- Auth implication: layout loads don't run on every navigation, and page loads run
  concurrently with layout loads unless `await parent()` — guard in `handle` or per-page
  server load, not in a layout load alone. DOCS `20-load.md:L700-L712`.

### Page options

- Exported from `+page.js`, `+page.server.js`, `+layout.js`, `+layout.server.js`; child
  overrides parent; root layout sets the app default. `prerender = true | false | 'auto'`,
  `ssr`, `csr`, `trailingSlash = 'never' | 'always' | 'ignore'`, `config` (adapter-specific,
  merged one level deep), `entries()` for dynamic prerender routes. DOCS
  `20-core-concepts/40-page-options.md:L5-L222`.
- **Only literal boolean/string page options are evaluated statically**; otherwise kit
  imports the module on the server at build and runtime to evaluate them (browser-only code
  must not run at module load). DOCS `40-page-options.md:L132`.
- `ssr = false` in root `+layout.js` → SPA: empty shell HTML, universal loads only in the
  browser. `csr = false` too → nothing renders. DOCS `40-page-options.md:L118-L132`.
- `prerender` also applies to `+server.js` files (inherited from pages that fetch them);
  `url.searchParams` is forbidden during prerender; pages with actions cannot be
  prerendered; a route dir and a file can't share a name in prerender output (use file
  extensions like `foo.json/+server.js`). DOCS `40-page-options.md:L40-L75`.
- `trailingSlash: 'never'` (default) redirects `/about/` → `/about`; `'always'` yields
  `about/index.html` on prerender, else `about.html`. Also exportable from `+server.js`.
  DOCS `40-page-options.md:L161-L174`.
- `csr = false` strips all `<script>` tags, disables enhancement/HMR — not usable here. DOCS
  `40-page-options.md:L134-L159`.

### Routing

- `src/routes` is the root; directories become paths; files with a `+` prefix are route
  files; `+layout`/`+error` apply to subdirectories. "All files run on the client except
  `+server` files." DOCS `10-routing.md:L5-L21`.
- Route files: `+page.svelte`, `+page.js`, `+page.server.js` (load, page options, `actions`),
  `+error.svelte`, `+layout.svelte` (must `{@render children()}`), `+layout.js`,
  `+layout.server.js`, `+server.js` (exports `GET/POST/PATCH/PUT/DELETE/OPTIONS/HEAD/QUERY`
  and `fallback`, returning `Response`). Other files in a route directory (tests, stories,
  components) are ignored. DOCS `10-routing.md:L23-L445`.
- `+server.js`: `+layout` files have no effect on it; `HEAD` uses `GET`'s content-length;
  `fallback` catches unhandled methods; errors return JSON or `error.html` by `Accept`
  (`+error.svelte` not used); Vite injects CORS headers on `OPTIONS` in dev only. DOCS
  `10-routing.md:L294-L394`.
- **Content negotiation when `+page` and `+server` share a directory**: `PUT/PATCH/DELETE/
  OPTIONS/QUERY` always go to `+server.js`; `GET/POST/HEAD` go to the page if `Accept`
  prioritises `text/html`, else `+server.js`; `GET` responses get `Vary: Accept`. DOCS
  `10-routing.md:L396-L402`.
- Advanced routing: `[param]`, `[...rest]` (matches zero segments too; validate with a
  matcher; use `[...path]` + `error(404)` for nested 404 pages, else 404s arrive in
  `handleError` as `kind: 'framework'`), `[[optional]]` (can't follow a rest param),
  `[name=matcher]`, `(group)` dirs (no URL effect), `+page@segment.svelte` /
  `+layout@.svelte` to reset the layout chain, `[x+nn]`/`[u+nnnn]` encodings (e.g.
  `[x+2e]well-known`). DOCS `30-advanced/10-advanced-routing.md:L5-L70`, `L159-L283`.
- **Sort order** (Go's router must match): more specific first; matcher params beat plain
  params; `[[optional]]` and `[...rest]` are ignored unless final (then lowest priority);
  ties alphabetically. Example order: `foo-abc`, `foo-[c]`, `[[a=x]]`, `[b]`, `[...catchall]`.
  DOCS `10-advanced-routing.md:L130-L157`.
- Matchers run on server **and** client. DOCS `10-advanced-routing.md:L126`.
- `$app/types`: `RouteId` (union of all IDs, start with `/`), `PageRouteId`,
  `EndpointRouteId`, `Path`/`ResolvedPathname` (no leading slash), `RouteParams<'/blog/[slug]'>`,
  `LayoutParams<'/x'>`; `resolve('/blog/[id=number]', { id: 1 })` from `$app/paths`. DOCS
  `98-reference/22-$app-types.md:L11-L119`; `10-advanced-routing.md:L118-L124`.

### Form actions (stay TS — recorded for completeness)

- `+page.server.js` exports `actions` (`default` or named, invoked via `?/name`); each gets a
  `RequestEvent`, reads `request.formData()`, may set cookies, returns data (page `form`
  prop / `page.form`) or `fail(status, data)` (enhanced responses now carry that status; `204`
  when returning nothing); `redirect()` works; after an action the page's loads rerun but
  `handle` does not, so update `event.locals` yourself after changing a cookie. `use:enhance`
  requires `method="POST"` and a `+page.server.js` action; cross-page enhanced submissions now
  **navigate** to the action page. `method="GET"` forms behave like links. DOCS
  `20-core-concepts/30-form-actions.md:L105-L135`, `L166-L170`, `L276-L300`, `L340-L367`,
  `L545-L560`; `35-migrating-to-sveltekit-3.md:L99-L101`, `L400-L402`.
- `+page.server.js` actions are the one server surface skgo does **not** move to Go
  (junkyard interview 0005, recommendation: actions stay TS). In a CSR-only app with no Node
  in production there is no runtime for them at all — the example app must avoid classic
  actions entirely and use remote `form` instead.

## Citations

- `reference/kit/documentation/docs/20-core-concepts/20-load.md:L1-L786`
- `reference/kit/documentation/docs/20-core-concepts/40-page-options.md:L1-L226`
- `reference/kit/documentation/docs/20-core-concepts/10-routing.md:L1-L453`
- `reference/kit/documentation/docs/30-advanced/10-advanced-routing.md:L1-L320`
- `reference/kit/documentation/docs/30-advanced/20-hooks.md:L15-L103`, `L361-L379`
- `reference/kit/documentation/docs/30-advanced/25-errors.md:L13-L169`
- `reference/kit/documentation/docs/20-core-concepts/30-form-actions.md:L105-L135`,
  `L166-L170`, `L276-L300`, `L340-L367`, `L519-L560`
- `reference/kit/documentation/docs/98-reference/22-$app-types.md:L11-L119`
- `reference/kit/documentation/docs/98-reference/54-types.md:L26-L124`
- `reference/kit/documentation/docs/60-appendix/35-migrating-to-sveltekit-3.md:L99-L101`,
  `L130-L134`, `L358-L368`, `L400-L402`, `L569-L581`
- `junkyard/ephemeral/projects/skgo/interview/0005.md:L1-L20`
- `junkyard/ephemeral/projects/skgo/chapters/08-go-loads/CHAPTER.md:L13-L23`

## skgo implications

### What the `page.server.go` / `layout.server.go` generator must expose

- **Event struct** mirroring `ServerLoadEvent`: typed `Params` (from the route ID, honouring
  matcher output types), `Route.ID`, `URL` (with the build-time origin, since Go owns it),
  `Cookies` (get/set/delete with kit 3 defaults: `path` `/`, `httpOnly`, `secure`),
  `Locals` (populated by Go middleware — the junkyard planned a `Project(func(*http.Request)
  Locals)` seam, `chapters/06-host-api/CHAPTER.md:L15-L19`), `ClientAddress`, `Request`,
  `SetHeaders` (once per header; reject `set-cookie`), `Parent()` (typed merged parent
  layout-server data), `Depends(id)`. `Fetch`/`untrack` are optional in Go.
- **Return type** is a Go struct → devalue-encodable JSON with `Date`/`Map`/`Set`/`BigInt`
  support and `transport` codec hooks; the generator emits a `.d.ts`/`$types`-compatible
  declaration so `PageProps['data']` is typed as if the load were TS (junkyard interview
  0004 option 1: Go is the source). Merge semantics: deepest wins per key.
- **Streaming**: a Go field typed as a promise-equivalent (channel/future) should be streamed
  the way kit streams top-level promises in the data response; if not supported in v1,
  document that Go loads are non-streaming. Rejections must not crash the process.
- **Errors/redirects**: Go `error(status, msg, extras)` and `redirect(status, location,
  external?)` values that the data-response encoder turns into kit's error/redirect shapes;
  all errors funnel through a Go `handleError` equivalent with `kind`.
- **Rerun rules are client-side** — Go doesn't decide; but Go must return the dependency
  metadata kit's client expects for `depends()`/`params`/`url` tracking, which is part of
  the unpublished data-request format (`__data.json`; not in any authored doc — source
  segment). Go must also support the "group multiple server loads into one response"
  contract: one data request resolves all server loads (layout chain + page) for the route,
  including `Parent()` ordering.
- **Universal loads still exist** in TS and run only in the browser; the server-load result
  is their `data` input. Go should not try to run them.

### Serving the built bundle (CSR-only)

- Every page request that isn't a static asset or a prerendered file must return the SPA
  shell (the `ssr = false` output) — the client router then matches routes. Go still needs
  the route table for: data requests (which loads to run for which route ID), content
  negotiation between `+page` and `+server` in the same directory, `trailingSlash` redirects
  (308), 404 for unknown routes (`kind: 'framework'`), and `+server.js` endpoints that are
  authored in TS — which have **no runtime in production**; the example app must not use
  `+server.js` (or skgo must offer a Go equivalent: "your own routes" per the framing).
- Route sort order in Go must reproduce kit's (specificity, matchers, optional/rest last,
  alphabetical) and param matchers must be re-implemented in Go for any route Go answers
  natively — matchers run on the client too, so the client will already have rejected
  non-matching URLs, but Go must not trust that.
- Prerender: with `ssr = false`, prerendered pages are shells; `prerender` remote results
  still need serving. `entries()` and the prerender crawler need a renderer at build time —
  which means `skgo build` must run kit's build with Node present (build-time only).
- Page options must be **literals** in the example app so kit evaluates them statically and
  never needs to import `+page.ts`/`+layout.ts` on a server that doesn't exist.

## Gotchas

- `setHeaders` in a server load applies to the *data response* during client navigation,
  not to the page shell; caching headers meant for HTML don't do what kit-2 SSR habits
  expect under `ssr = false`.
- `await parent()` in a layout load only sees parent **server** layouts; a `+layout.ts`
  between them is invisible to the server side.
- A layout load throwing does not stop the page load from running (they're concurrent) —
  Go must run them concurrently or at least not assume ordering, and must not return page
  data when the layout errored.
- `url.hash` is never available in a load. `request.url` is untracked.
- `+server.js` in the same directory as `+page.svelte` steals non-HTML `GET`s; `Vary: Accept`
  is required on `GET` responses if Go implements that negotiation.
- `[...rest]` matches the empty path; `/a/[...rest]/z` matches `/a/z`. Validate.
- `error()`'s second argument must be a **string** in kit 3 (object form removed).
- `10-getting-started/30-project-structure.md:L12-L13,L43` still lists `src/params/`;
  kit 3 uses a single `src/params.ts`.
- `$types` files are emitted by `svelte-kit sync` under `.svelte-kit/types` and reached via
  `rootDirs`; skgo's generated declarations for Go loads must slot into the same
  `./$types` resolution or ship as sibling `.d.ts` that `$types` re-exports — otherwise
  `PageProps` will be typed `any`.

## Recipes

- **Server load with cookie, parent, error, redirect (the shape Go mirrors)**
  ```ts
  import { error, redirect } from '@sveltejs/kit';
  export const load: PageServerLoad = async ({ params, cookies, locals, parent }) => {
    if (!locals.user) redirect(303, '/login');
    const { posts } = await parent();
    const post = await db.getPost(params.slug);
    if (!post) error(404, 'Not found');
    cookies.set('seen', params.slug);          // path defaults to '/'
    return { post, comments: loadComments(params.slug) }; // promise → streamed
  };
  ```
- **CSR-only app skeleton**: `src/routes/+layout.ts` → `export const ssr = false;` (literal);
  `src/routes/+layout.server.ts` → Go-generated layout load; pages use
  `let { data, params } = $props()`.
- **Param matcher**: `src/params.ts`
  ```ts
  import { defineParams } from '@sveltejs/kit/params';
  import * as v from 'valibot';
  export const params = defineParams({ integer: v.pipe(v.string(), v.toNumber()) });
  ```
  then `src/routes/items/[id=integer]/+page.svelte` sees `params.id: number`; Go's matcher
  for the same route must accept the same strings and produce the same typed value.
- **Manual invalidation from a component**: `depends('app:random')` in the load;
  `invalidate('app:random')` or `refreshAll()` from `$app/navigation` in the page.
- **Nested 404**: `src/routes/section/[...path]/+page.ts` → `error(404, 'Not Found')`.
