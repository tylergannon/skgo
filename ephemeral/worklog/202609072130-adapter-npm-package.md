# @skgo/adapter as an npm package (#48)

## A linked package cannot see the app's node_modules, and that is not a corner case

`internal/adapter` is now the npm package root, and `example/web` depends on it
as `link:../../internal/adapter`. Node resolves a symlinked package by its
realpath, so every bare specifier inside the adapter resolves beside
`internal/adapter/` — where nothing is installed. Measured, not assumed:

    Cannot find package 'vite' imported from .../internal/adapter/probe.mjs

Two places had to change, and both are improvements rather than workarounds:

- `env.js` imported `vite/rolldown` and `vite/rolldown/experimental` statically.
  It now resolves them from `process.cwd()` (the vite root) with
  `createRequire` + `await import`. This is the same rule `mise.toml` already
  documents for `vp`: kit checks its dev SSR environment with an `instanceof`
  against *the project's* resolved vite, so a second copy of vite in a second
  module realm is wrong even where it exists. The module now has top-level
  await; vite's config loader externalises the package, so it loads fine.
- `entry.js` and `app-server.js` import `svelte/server` and
  `@sveltejs/kit/internal/server` — bare specifiers naming packages the *app*
  depends on. The goja environment's `resolveId` now resolves a bare specifier
  from one of the package's own files through vite's own resolver with the app
  root as importer.

Both would have been needed for a registry install too the moment pnpm's strict
layout put the adapter in its own virtual store directory.

## The version stamp is gone; the adapter names itself

`Source()` used to regex-replace `const SKGO = ...` on its way into the vite
root. A published package cannot be stamped on the way out, so
`skgo-adapter/identity.js` computes the fingerprint at build time by hashing the
package's own files — the same bytes in the same order Go hashes its embedded
copy. Verified across languages: JS wrote `900e9faf6a76` into the manifest and
`adapter.Fingerprint()` returned `900e9faf6a76`.

This makes byte-identity structural rather than asserted. A package missing a
runtime file, or carrying an edited one, produces a different fingerprint and is
refused at startup naming both — which is what makes the `files` list in
package.json load-bearing instead of decorative.

## The `go generate` fast-fail fires inside the build gesture, before vite

Measured with the mutation (`skgo-adapter/polyfill.js` removed from
package.json's `files`, repacked, reinstalled, `mise run build`):

    [build] $ go generate ./...
    skgo: the @skgo/adapter installed in this app is not the one this skgo publishes.
      installed in .../web/node_modules/@skgo/adapter: skgo 0.0.0-dev (adapter 4b90c39a6f01)
      this program:  skgo devel (adapter 900e9faf6a76)

`mise run build` installs before it generates, so a mismatched install never
reaches `vp build`. The check is skipped entirely when nothing is installed —
`go generate` before an install is the wrong order, not the wrong adapter, and
refusing there would be a new reason a tree cannot be generated.

## CI does not run `just generate`

`ci.yml` goes install → build → vet → test → e2e. The generated tree is
committed, so nothing regenerates on a runner and the fast-fail above is a
local and scaffold-test path only. The startup gate is what CI exercises.

## Release needs the owner first

`@skgo/adapter` is unclaimed and the `skgo` npm org does not exist. Nothing here
can publish; `.github/workflows/release.yml` waits on a `v*` tag and an
`NPM_TOKEN` secret. Note for whoever cuts the first release: pnpm 11 enforces a
minimum release age by default, so a freshly published `@skgo/adapter` may be
refused by `pnpm install` for a day unless the app lists it under
`minimumReleaseAgeExclude`. The example app and the scaffold test both install
from a local path, so neither sees this.
