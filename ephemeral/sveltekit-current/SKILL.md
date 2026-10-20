---
name: sveltekit-current
description: Map SvelteKit 3 behavior from skgo's pinned Kit source before changing the adapter, generated app, or example frontend.
---

# Current SvelteKit

Treat Kit as the specification. Before changing a mirrored feature, read its
implementation in the installed `example/web/node_modules/@sveltejs/kit`
package, verified as version `3.1.0` against the frozen lockfile. Each worktree
installs its own pinned package; do not depend on another checkout's ignored
reference directory. Use the official GitHub release for the target tag to
identify migrations since the prior pin.

Kit 3 facts that overturn older SvelteKit assumptions:

- Configuration is passed to `sveltekit(...)` in `vite.config.ts`; there is no
  app-owned `svelte.config.js` in this repository.
- Kit requires Node 22.17+, Vite 8.0.12+, Svelte 5.57.1+, and TypeScript 6.
  TypeScript 7 is not a compatible substitute for the `typescript` package.
- `kit.paths.origin`, when set, is compiled into the build and takes
  precedence over the request's origin; unset (the default), Kit derives the
  origin from `request.url`. The example sets it from `ORIGIN`, so its build
  fixes the origin.
- The application alias is `#lib`, declared through package imports; `$lib`
  was removed.
- `browser`, `dev`, `building` and `version` come from `$app/env`;
  `$app/environment` is a deprecated re-export. Environment variables are
  declared in `src/env.ts` with `defineEnvVars` and imported from
  `$app/env/public` and `$app/env/private`.
- The public `$app/paths` exports only `asset`, `resolve` and `match`. Adapter
  code that needs the base path or app directory reads Kit's build defines,
  as `internal/adapter/skgo-adapter/reroute.js` does.
- Sync writes the generated `$app/types` and `$app/env` declarations to
  `node_modules/$app/types/`, not `.svelte-kit`. A fresh install has none
  until sync runs, and a fixture that shares another app's `node_modules`
  overwrites that app's route types: give each fixture app its own install
  and run sync before `svelte-check`.
- `transport` is a universal hook. A key Go sends must match the Go codec's
  tag and payload exactly; a key only the browser uses needs no Go half, since
  Kit also passes snapshots and history state through the hook.
- Remote functions remain experimental and require
  `experimental.remoteFunctions`.
- Adapter Vite plugins are split into `pre` and `post`. Kit 3.0.0-next.27 also
  passes the Svelte config to adapter plugin factories and replaces
  `builder.generateManifest` with `builder.generateServerInstance` plus
  `builder.manifest`.
- `$app/manifest` exports build assets and route records, including `page` and
  `endpoint` booleans.
- Endpoint exports include the HTTP `QUERY` method. It is unrelated to a
  SvelteKit remote query.
- Remote form fields expose `dirty()` and `touched()` state.
- A command or form must fulfill every client-requested single-flight update or
  explicitly ignore it. The server returns ignored remote keys as `i`, and the
  next.27 client rejects a response with requested keys left unhandled.
- After internal data-request parameters are removed, Kit 3.0.0 rejects any
  remaining query parameter beginning with `x-sveltekit-` with HTTP 400 before
  it runs hooks or resolves a route.
- `builder.generateServerInstance` now writes a `server` object made by
  `create_server`. `Server` is deprecated; the generated instance still exposes
  `init` and `respond`.

When the dependency pin advances, update the verified version and this list
only for facts verified in that source or the official release notes.

Verified 3.1.0 changes: endpoint server tracing creates `sveltekit.endpoint`;
CSP quotes `report-sha256`, `report-sha384`, and `report-sha512`. Prerender MIME
extensions come from generated files. Kit sync compares physical generated-file
contents before rewriting; the browser inherits iframe focus and service-worker
deployment recovery fixes. The exact declared-Inputs queue correction remains
required and is qualified against 3.1.0.
