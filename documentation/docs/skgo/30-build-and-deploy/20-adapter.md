---
title: The adapter
summary: "@skgo/sveltekit-adapter: how it is installed and configured, what it writes, and what it refuses."
proof:
  - TestAManifestFromThisModulesAdapterIsRead
  - TestAManifestFromAnotherSkgosAdapterIsRefused
  - TestGeneratingAgainstAnotherSkgosAdapterIsRefused
---

Kit turns an app into a deployable form through an [adapter](https://svelte.dev/docs/kit/adapters). SKGo has one, `@skgo/sveltekit-adapter`, and its output is not a server. It writes the files a Go binary embeds and a manifest that tells Go what to answer.

## Using the adapter

`skgo new` installs the adapter in place of any `@sveltejs/adapter-*` package and configures it in `web/vite.config.ts`:

```ts
/// file: web/vite.config.ts
import { sveltekit } from "@sveltejs/kit/vite";
import { defineConfig } from "vite-plus";
import skgo from "@skgo/sveltekit-adapter";

export default defineConfig({
  plugins: [
    sveltekit({
      adapter: skgo(),
      experimental: { remoteFunctions: true },
      compilerOptions: { experimental: { async: true } },
    }),
  ],
});
```

The two experimental flags are not optional: SKGo's remote functions and its generated stubs depend on them. The scaffold does not set `paths.origin`; [Building your app](building-your-app#The-origin-is-compiled-in) says why you should.

## Installing the adapter

The adapter and the Go module are one contract in two languages, and a release publishes the package only when the adapter's files changed. The version to install is therefore the newest at or below your SKGo version, which is the `github.com/tylergannon/skgo` requirement in `go.mod` without its `v`:

```sh
vp add -D '@skgo/sveltekit-adapter@<=VERSION'
```

The package declares `@sveltejs/kit` %kit% as a peer dependency, and `@opentelemetry/api` as an optional one for [tracing](observability). If your package manager enforces a minimum release age, exclude this package from it; otherwise a release that changes the adapter can leave you on the previous one, and the Go side refuses that build at startup. `skgo generate` makes the same comparison earlier, against the adapter installed in `node_modules`, so a stale install fails before the frontend is built.

## Options

```ts
adapter: skgo({
  out: "build",
  precompress: true,
  prerenderPackage: "./buildservice",
});
```

### out

The directory the build is written to, relative to the Vite root. It defaults to `build`, which is the directory `web/dist.go` embeds with `//go:embed all:build`. Change both or neither.

### precompress

Writes `.br` and `.gz` copies of the files under `client/` and `prerendered/`. It defaults to `true`, and the Go server sends whichever copy the request's `Accept-Encoding` allows; see [Deploying](deploying#Static-files).

### prerenderPackage

The Go package that answers Kit's build-time requests, relative to your Go module root. It defaults to the entry `skgo generate` writes under `internal/skgo/prerender`. Set it when the build should share settings with your production renderer; see [Custom prerender service](prerendering#Custom-prerender-service).

## What the adapter writes

Under `out`, the adapter writes `client/` (Kit's client bundle), `prerendered/` (pages, data files and remote results the build produced), `ssr/bundle.js` (the server-rendering bundle the Go process runs), `index.html` (Kit's SPA fallback document), `app.html` and `error.html` (your templates), `env.json` (the static environment variables) and `skgo.manifest.json`. The manifest carries the routes, the pages' node table, the server loads, the remote function ids, the prerendered paths and the adapter's identity. `skgo.ReadManifest` reads it when the server starts; [Server composition](server-composition) shows how the pieces are wired.

Along the way the adapter checks that the two halves agree. The server loads and actions Kit compiled must be exactly the ones `skgo.remotes.json` declares, and every remote function the built client calls must be one `skgo generate` wrote. A hand-written `.remote.ts` fails the build for that reason; write it as a `.remote.go` file instead.

## Refused configurations

Two Kit options fail the build:

- `output.bundleStrategy: 'inline'`. SKGo assembles the boot script itself and only knows the split form.
- `router.resolution: 'server'`. Routing is Go's, and the client resolves its own routes.

## Other adapters

SKGo builds with this adapter only. The Node, Bun, static and platform adapters produce a JavaScript server or a static site, and every server load, action, endpoint and remote function in an SKGo app is Go, so there is nothing for them to run. Serverless platforms are [not currently planned](project-types#Serverless-app). A fully prerendered app is still deployed as the binary; see [Prerendering](prerendering#Static-only-output).
