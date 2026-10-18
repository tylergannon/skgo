---
title: Building your app
summary: Generate the bindings, build the frontend, and compile one Go binary that embeds it.
proof:
  - TestTheEmbeddedBuildProducesAWorkingServer
  - TestDriftBetweenTheBuildAndTheBinaryStopsTheServer
  - TestBuildReusesCurrentOutputsAndRebuildsChanges
  - TestBuildEmbedsImmutableAssets
---

Kit's [build](https://svelte.dev/docs/kit/building-your-app) has two stages: Vite builds the app, then the adapter turns it into a deployable form. SKGo adds a Go step on each side. The `Justfile` that `skgo new` writes runs all of it:

```text
/// file: Justfile
build:
    go generate ./...
    cd web && ORIGIN="{{origin}}" vp build
    mkdir -p bin
    go build -ldflags "-X main.origin={{origin}}" -o bin/myapp ./cmd
```

`go generate ./...` runs [`skgo generate`](/docs/cli/generate), which writes the TypeScript modules, the `+page.server.ts` stubs and `skgo.remotes.json` from your Go route files. `vp build` is Kit's ordinary build with [the SKGo adapter](adapter) writing to `web/build`. `go build` compiles your Go and embeds that directory:

```go
/// file: web/dist.go
/// include: example/web/dist.go
```

The result is one executable. It renders pages itself, so Node is not installed where it runs.

The order matters. The adapter reads `skgo.remotes.json` and compares it with what Kit compiled; when the file is missing it fails with `Run go generate ./... before building the frontend`, and when a Go load or remote function was added or removed since the last generation it fails naming the difference. A fresh checkout compiles before any frontend build has run, because `web/build/.gitkeep` is tracked; the binary it produces refuses to start until `vp build` has written a manifest.

## During the build

Kit's own build runs as it does upstream, and universal code can read [`building`](https://svelte.dev/docs/kit/$app-env#building) from `$app/env`. Your Go loads, endpoints and remote functions take part too: the adapter compiles and starts a Go [prerender service](prerendering#How-prerendering-runs), which answers Kit's build-time requests, so the machine that runs `vp build` needs the Go toolchain as well as Node.

A [static environment variable](environment-variables#Static-variables) is read from the build's environment and inlined; the adapter fails the build when one is missing or malformed. Every other variable is read when the server starts.

## The origin is compiled in

Set the public origin once, in the `ORIGIN` variable, and both halves of the build read it. Kit %kit% fixes `paths.origin` at build time and uses it while prerendering; the scaffold's `vite.config.ts` does not set it, so add it, as the example app does:

```ts
/// file: web/vite.config.ts
import { sveltekit } from "@sveltejs/kit/vite";
import { defineConfig } from "vite-plus";
import skgo from "@skgo/sveltekit-adapter";

export default defineConfig({
  plugins: [
    sveltekit({
      adapter: skgo(),
      paths: { origin: process.env.ORIGIN ?? "http://127.0.0.1:8080" },
      experimental: { remoteFunctions: true },
      compilerOptions: { experimental: { async: true } },
    }),
  ],
});
```

`go build -ldflags "-X main.origin=…"` sets the `origin` variable in `cmd/main.go`, which is the default of the binary's `-origin` flag and, through its host and port, of `-listen`. Go hands that origin to `NewHandler`, where it decides which form submissions and remote calls are same-origin, what URL a load sees, and whether cookies default to `Secure`. Changing the origin means rebuilding; [Deploying](deploying#The-origin) covers the flags.

## Incremental builds

[`skgo build`](/docs/cli/build) runs the frontend half only when something changed. It fingerprints the Go module and the source tree, installs frontend dependencies when `web/package.json` or the lockfile changed, runs `go generate ./...`, `vp fmt` and `vp build`, and records the result under `.skgo/`. An unchanged tree prints `Frontend is current` and stops. It never compiles the Go binary; `go build` follows it.

```sh
go tool skgo build
go build -o bin/myapp ./cmd
```

## Build checks

The adapter stamps every build with a fingerprint of its own files, and `skgo.ReadManifest` compares that stamp with the adapter your Go module embeds when the server starts. A build written by a different adapter is refused:

```text
skgo: the frontend build and this program come from different skgo versions.
```

The message names both sides, the version and adapter fingerprint that built the frontend and the ones in the program, and ends with the install command for the matching package. Install it, run `go generate ./...` and build the frontend again; see [The adapter](adapter#Installing-the-adapter).

## Preview your app

There is no `vite preview`. Kit's preview server runs the build in Node, where every generated server stub throws, so `skgo new` rewrites the `preview` script in `web/package.json` to `cd .. && just serve`. Either spelling runs the real binary:

```sh
just serve
```

`serve` depends on `build`, so it is always a build followed by `./bin/myapp` listening on the origin's host and port.
