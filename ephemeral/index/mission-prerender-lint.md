# Mission: refuse a Go server load above a prerendered page, with a message that says why (#83)

Capability when done: a developer who puts a Go server load in a branch that
also has a prerendered page is told so by `skgo generate`, naming the route,
the load and the rule, before `vp build` ever runs; and if the build is run
anyway, the throwing stub's error names the same three things instead of
`skgo: implemented in Go`. Closes #83. (#81 is the later fix that makes it
work; this mission only refuses clearly.)

## Why it fails today

Kit runs a branch's server loads while it prerenders (build time, Node).
skgo's `+page.server.ts` / `+layout.server.ts` are generated stubs that throw.
So any prerendered page whose branch has a Go load fails inside kit's
prerenderer with the stub's stack. Seen in PR #79, where a root
`layout.server.go` broke `/about`.

## Where things are

- Stubs: `internal/gen/emit.go`:107-110 writes `unimplemented` and
  `throw new Error('skgo: implemented in Go')`; `internal/gen/endpoints.go`:102-106
  is the same for `+server.ts`. The load stub wraps `load(event)`, so it has
  the event's URL; kit's `building` flag from `$app/environment` is true
  during prerender, which is how the stub can say "while kit prerendered
  <path>" only when that is what happened.
- Route and load scan: `internal/gen/loads.go` pairs a Go load with its
  `+page.server.ts`/`+layout.server.ts`; `internal/gen/scan.go` and
  `links.go` walk the route tree; `gen.go` is the entrypoint that reports
  errors. Tests with fixture trees to copy: `ownership_test.go`,
  `links_test.go`, `ownershiprule_test.go`.
- The adapter already parses a page option out of module source at build
  time: `internal/adapter/skgo-adapter.js` `option()`:423 (`export const ssr`,
  `csr`). The same reading of `export const prerender = true | 'auto' |
  false` in `+page.ts`, `+page.server.ts`, `+layout.ts`, `+layout.server.ts`
  is what generate needs; a Go implementation, not a call into the adapter.
  `readPrerendered()`:285 is post-build knowledge and too late.

## Kit as the specification

`core/postbuild/analyse.js` resolves each route's page options from its
nodes (`nodes.prerender()`:224 inherits down the branch with a leaf
overriding; :116-118 puts a route in the prerender list only when the value
is `true`; :101 forbids a prerendered `+page` beside a `+server`);
`core/postbuild/prerender/prerender.js` is the crawl that runs the loads, and
it also prerenders `'auto'` pages it reaches by link, so map from source
whether `'auto'` above a Go load must be refused or only warned; `core/config/options.js` and
`core/sync/write_types` for which files may declare the option. Mirror kit's
inheritance rule exactly; a lint that is stricter than kit refuses apps kit
would build.

## Proof

Go tests in `internal/gen`: a fixture tree with a prerendered leaf under a Go
layout load fails with the exact message (route, load file, rule, `#81`); a
Go load beside a non-prerendered sibling passes; a prerendered leaf whose own
branch has no Go load passes even when another branch has one. Then the real
thing: on the example app as it stands on main (`/about` prerendered, no root
Go load) `skgo generate` still passes; add a root `layout.server.go` (the
shape PR #79 used, see that PR's diff) and generate must refuse with the
message, before any `vp build`. Revert the experiment. Run the stub message
once too: with the lint bypassed or the fixture built, `vp build` must show
the new message, not the old one; a Go test on the emitted stub text covers
the wording.

## Collisions

PR #80 (adapter) touches `internal/gen/adapter.go` only; you own
`internal/gen` otherwise. PR #79 removes `/about`'s prerender in the example;
whichever lands first, your lint must accept the example as it is on main
when you open the PR.
