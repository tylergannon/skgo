# CSR shell: what `ssr = false` pages and the SPA fallback receive

## Purpose

skgo serves kit's client bundle as a CSR app. This file pins down exactly what
kit's own server emits for an `ssr = false` page (the "shell"), what the
build-time `fallback` HTML is and how it is produced, which `__sveltekit_*`
globals the client bundle reads at boot, and what the client does next
(the `enter` navigation that triggers `__data.json`). Go must either serve the
prebuilt fallback verbatim or reproduce this HTML.

## Key facts

### When the shell is produced

- `render_page`: if `ssr === false` and not prerendering server data, kit
  returns `render_response` with `branch` = the page's nodes with `data: null`
  (no loads run — "so that the styles and fonts are linked in the head before
  CSR takes over"), `page_config: { ssr: false, csr }`, `status` = 200 or the
  action result status. `reference/kit/packages/kit/src/runtime/server/page/index.js:L108-L156`
- `ssr`/`csr` are the last non-undefined `universal ?? server` option on the
  root→leaf node chain; defaults `true`. `reference/kit/packages/kit/src/utils/page_nodes.js:L48-L62`
- Fallback (SPA) / hash-routing: `resolve()` in respond.js bypasses routing
  entirely when `__SVELTEKIT_HASH_ROUTING__ || state.prerendering?.fallback`,
  rendering `branch: [root layout node only]`, status 200, `ssr: false, csr: true`.
  `reference/kit/packages/kit/src/runtime/server/respond.js:L613-L633`
- Build-time fallback generation (`adapter-static` SPA mode, kit's
  `builder.generateFallback`): boots the built `Server`, requests
  `origin + '/[fallback]'` with `prerendering: { fallback: true, ... }`, and
  writes the response text. `reference/kit/packages/kit/src/core/postbuild/fallback.js:L18-L52`
- For an error on a route without SSR (root layout `ssr = false`),
  `respond_with_error` also produces a shell with `branch: []` and the error
  status. `reference/kit/packages/kit/src/runtime/server/page/respond_with_error.js:L31-L96`

### What the shell HTML contains

`reference/kit/packages/kit/src/runtime/server/page/render.js:L51-L663`

- `rendered = { head: '', body: '', hashes: { script: [] } }` for non-SSR. `L259-L261`
- Head accumulates: `modulepreload` links for `client.imports` + each branch
  node's `imports` (subject to `resolve_opts.preload`, default js+css),
  stylesheet `<link rel="stylesheet">` for `client.stylesheets` + node
  stylesheets, font preloads, inline `<style>` for below-threshold CSS.
  `L75-L81, L263-L350, L381-L389`
- Body (`page_config.csr && client`): one `<script>` block:
  ```js
  {
      __sveltekit_<hash> = { base: <expr>, version: "<name>"[, assets: "..."][, env: ...] };
      const element = document.currentScript.parentElement;
      import("<base>/_app/immutable/entry/start.<hash>.js").then((app) => { app.start(element) });
  }
  ```
  For `bundleStrategy: 'split'` the boot is
  `import(start).then(async kit => { kit.init(__sveltekit_x); const app = await import(app.js); kit.start(app, element) })`;
  for `'inline'` the script is inlined. `L415-L586`
- `element` argument only; the second `{ node_ids, data, form, error, ... }`
  hydration object is added ONLY when `page_config.ssr`. `L477-L520`
- `base` expression: `s(paths.base)` normally; when `paths.relative` (default
  true) and not fallback, a relative `'.'`/`'../..'` computed from the
  request path depth (`base_expression = new URL("<rel>", location).pathname.slice(0, -1)`).
  For the fallback page (`state.prerendering.fallback`) the relative branch is
  skipped so `base` stays the absolute configured value. `L112-L140`
- Global name: `__sveltekit_dev` in dev, else `__sveltekit_${hash(version_name)}`.
  `reference/kit/packages/kit/src/core/utils.js:L20-L25`
- `defer`/`resolve` helpers are only added when `chunks` (SSR streamed
  promises) exist — never for the shell. `L427-L470`
- Remote-function prelude data (`__sveltekit_x.data = ...`) is added only if
  `collect_remote_data` found any. `L522-L527`
- Service-worker registration snippet if configured. `L554-L574`
- Headers: `x-sveltekit-page: true`, `content-type: text/html`;
  CSP header(s) when configured; `link` preload header when
  `output.linkHeaderPreload`; `etag: "<hash(html)>"` when not streamed. `L588-L637`
- The template is `options.templates.app({ head, body, assets, nonce, env })` —
  `src/app.html` with `%sveltekit.head%`, `%sveltekit.body%`,
  `%sveltekit.assets%`, `%sveltekit.nonce%`, `%sveltekit.env.X%` substituted.
  `L620-L626`, `reference/kit/packages/kit/src/types/internal.d.ts:L498-L507`
- Prerendered/fallback mode adds `<meta http-equiv>` for CSP and cache-control
  instead of headers. `L593-L604`

### What the client does on boot without hydration data

- `app.start(element)` → `_start` → `init_transport`, `hooks.init`,
  `parse_routes` (client routing) then, since `data` is undefined,
  `navigate({ type: 'enter', url: location.href, replace_state: true })`.
  `reference/kit/packages/kit/src/runtime/client/client.js:L494-L600`
- That navigation matches the route client-side (`dictionary` + regexes) and
  builds the invalidation bitmask with every server-load node invalid (no
  previous branch), so the first `__data.json` fetch has all `1`s for nodes
  with server loads (`0` for nodes without). `L1440-L1468`
- No route match → client 404 (`server_fallback` → root error page, and
  `load_data(url, [true])` if the root layout has a server load). `L2035-L2058, L1696-L1732`
- `payload.base ?? __SVELTEKIT_PATHS_BASE__` / `payload.assets` are read from
  the global at module init; the client bundle's `$app/paths` depends on it.
  `reference/kit/packages/kit/src/runtime/app/paths/internal/client.js:L8-L11`,
  `reference/kit/packages/kit/src/runtime/client/payload.js:L7-L12`
- `version` from the global is compared against `x-sveltekit-version` on
  every `__data.json`/action response (`notify_version`).
  `reference/kit/packages/kit/src/runtime/app/state/client.svelte.js:L196-L200`
- `SvelteKitPayload { version; base; assets?; env?; data?; defer? ... }`.
  `reference/kit/packages/kit/src/types/internal.d.ts:L752-L764`

### Static asset paths the client expects

- `/${app_dir}/immutable/...` (default `_app`) for JS/CSS chunks; served with
  long cache by adapters. Kit's own runtime returns 404 with
  `cache-control: public, max-age=0, must-revalidate` for unknown `/_app/*`.
  `reference/kit/packages/kit/src/runtime/server/respond.js:L356-L365`
- `/${app_dir}/env.js` is served dynamically by kit when
  `$env/dynamic/public` is used (`get_public_env`). `L356-L358`
- `/${app_dir}/version.json` is polled by the client when `version.pollInterval`
  is set (adapter-static output; not in the server runtime path read here).

## Citations

- `reference/kit/packages/kit/src/runtime/server/page/index.js:L82-L156`
- `reference/kit/packages/kit/src/runtime/server/page/render.js:L51-L140, L259-L350, L367-L586, L588-L663`
- `reference/kit/packages/kit/src/runtime/server/page/respond_with_error.js:L22-L107`
- `reference/kit/packages/kit/src/runtime/server/respond.js:L356-L365, L613-L633`
- `reference/kit/packages/kit/src/core/postbuild/fallback.js:L18-L52`
- `reference/kit/packages/kit/src/core/utils.js:L20-L25`
- `reference/kit/packages/kit/src/utils/page_nodes.js:L48-L62`
- `reference/kit/packages/kit/src/runtime/client/client.js:L494-L600, L1440-L1468, L1696-L1732, L2035-L2058`
- `reference/kit/packages/kit/src/runtime/client/payload.js:L7-L12`, `reference/kit/packages/kit/src/runtime/app/paths/internal/client.js:L8-L11`
- `reference/kit/packages/kit/src/runtime/app/state/client.svelte.js:L196-L200`
- `reference/kit/packages/kit/src/types/internal.d.ts:L498-L507, L752-L764`

## Go implementation notes

- Simplest correct path: have the adapter (kit build) emit the fallback HTML
  once (`builder.generateFallback`) and let Go serve that file for every
  page route that is not prerendered, with `content-type: text/html` and
  `x-sveltekit-page: true`. The fallback's `base` is absolute (fallback mode
  skips the relative computation), so it is safe at any depth. Status codes:
  200 for matched page routes; 404 for unmatched paths (the client will still
  boot and show its own 404). For form-action POSTs without JS, respond with
  the same shell using the action's status (or a real redirect).
- If Go renders per-route shells instead (to include per-node modulepreloads
  and stylesheets like kit does), it needs the client build manifest
  (`client.imports`, per-node `imports`/`stylesheets`/`fonts`) — this is an
  optimization, not a contract; the client works from the root-only fallback.
- Do NOT emit the hydration second argument to `start()`; with `ssr = false`
  kit never does, and the client would try to hydrate.
- The `__sveltekit_<hash>` global name must match what the client bundle was
  compiled with (it is derived from `kit.version.name`); using the prebuilt
  fallback guarantees this. `version` must equal the value sent in
  `x-sveltekit-version` from `__data.json`/actions or the client will think a
  new deployment happened.
- Ignore: CSP nonce/hash plumbing, `transformPageChunk`, `link` preload
  headers, inline styles, `etag`, service worker — none are needed for
  correctness (but etag/CSP are cheap to add).

## Gotchas

- `x-sveltekit-page: true` is set by kit on every HTML page; adapters (e.g.
  adapter-node) don't require it, but it's harmless to keep.
- The fallback is produced by requesting `/[fallback]`, a path that matches
  nothing; kit's `resolve()` short-circuit on `state.prerendering.fallback`
  is what makes it a root-layout-only shell — do not attempt to reproduce it
  by hitting a real route.
- Because loads do not run for the shell, cookies/headers set in Go loads
  only appear on the `__data.json` response, not on the HTML.
- A native (non-enhanced) form POST to a CSR page returns the shell with the
  action's status; the browser then boots the app at `?/action` URL — kit's
  client strips nothing here, so the resulting `enter` navigation URL still
  contains `?/action` in its search. Prefer redirects from Go actions.
- Hash routing (`router.type = 'hash'`) 404s every path except `base + '/'`
  and `/[fallback]` at the server. `reference/kit/packages/kit/src/runtime/server/respond.js:L137-L139`

## Recipes

Minimal shell that boots the kit client (values from the build):

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <link rel="modulepreload" href="/_app/immutable/entry/start.abc123.js">
    <link rel="modulepreload" href="/_app/immutable/chunks/xyz.js">
    <link rel="modulepreload" href="/_app/immutable/entry/app.def456.js">
    <link rel="modulepreload" href="/_app/immutable/nodes/0.js">
  </head>
  <body data-sveltekit-preload-data="hover">
    <div style="display: contents">
      <script>
        {
          __sveltekit_1a2b3c = { base: "" , version: "1712345678" };
          const element = document.currentScript.parentElement;
          import("/_app/immutable/entry/start.abc123.js").then((app) => { app.start(element) });
        }
      </script>
    </div>
  </body>
</html>
```

Go route table for a CSR app: static `/_app/immutable/*` (immutable cache) →
prerendered pages/files → `__data.json` (Go loads) → `?/action` POST (Go
actions) → `+server` endpoints (Go) → everything else on a page route → shell.
