---
name: sveltekit-current
description: Map SvelteKit 3 behavior from skgo's pinned Kit source before changing the adapter, generated app, or example frontend.
---

# Current SvelteKit

Treat Kit as the specification. Before changing a mirrored feature, read its
implementation in the pinned package under
`/Users/tyler/src/skgo/ephemeral/inspiration/reference/kit@3.0.0-next.27` and
refresh that package when skgo's `@sveltejs/kit` pin advances. Use the official
GitHub release for the target tag to identify migrations since the prior pin.

Kit 3 facts that overturn older SvelteKit assumptions:

- Configuration is passed to `sveltekit(...)` in `vite.config.ts`; there is no
  app-owned `svelte.config.js` in this repository.
- Kit requires Node 22.17+, Vite 8.0.12+, Svelte 5.56.4+, and TypeScript 6.
  TypeScript 7 is not a compatible substitute for the `typescript` package.
- `kit.paths.origin` fixes the application origin at build time.
- The application alias is `#lib`, declared through package imports; `$lib`
  was removed.
- `transport` is a universal hook. Its tags and payloads must match the Go
  codec exactly.
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

When the dependency pin advances, update the pinned-source path and this list
only for facts verified in that source or the official release notes.
