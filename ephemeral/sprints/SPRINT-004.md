# Sprint 004: Server loads, and one place to make a trust decision

## Mission

A developer writes server loads in Go — a layout's load and its pages' loads
composing as one navigation, streaming slow data as it resolves — and authorizes
requests in **one** place rather than in every handler.

Loads are not optional under CSR. With `ssr = false`, on first paint, kit's own
client issues `GET /<route>/__data.json?x-sveltekit-invalidated=…` unprompted;
it is the only endpoint kit calls on its own, once per navigation. skgo answers
today with the SPA document at HTTP 200, which kit's client then tries to parse
as JSON.

Authorization is today per-function: each remote calls `signedIn(ctx)` itself.
Two functions have already forgotten — `watchCount` disclosed a private row's
existence, `renameTodo` mutated and returned one. That is a missing shape, not
two missing checks, and kit has already solved it: `handle` in
`hooks.server.js`, plus layout loads guarding their subtree.

Kit is the specification; `ephemeral/inspiration/reference/kit/` is pinned
source. Under CSR there is exactly one wire format — NDJSON
`{"type":"data","nodes":[…]}`, errors and redirects at HTTP 200,
`{"type":"chunk"}` per settled promise — and no `devalue.uneval` HTML path.

## Acceptance

Scenarios in the example app, against a production build with vite stopped and
again under `vp dev`:

- A visitor is redirected out of a protected section before any page in it
  renders, and the rule that redirects them is written once — adding a second
  page to that section requires no new guard.
- A layout's data and its page's data arrive together on a cold load, and the
  page can read what its parent loaded.
- A page shows its fast content before its slow content has arrived, and the
  slow content appears without a second request.
- Navigating between two pages under one layout does not re-run the layout's
  load; invalidating it does.
- A load that fails puts the visitor on an error page instead of a blank one.
- Everything that passed before this sprint still passes.

No assertion may derive what it expects from the thing under test. Every
scenario leaves a screenshot of the page in the state it asserts, and whoever
runs them looks at all of them.

## Ownership

One builder. It owns the loads implementation and the new routes that exercise
it. It does not touch the todos, auth or colocation routes, the remote-function
runtime, or the generator's type projection — another effort owns those.

## Constraints

Go only — JavaScript exists solely where kit requires it. CSR only; no sidecar,
no SSR. Never reimplement kit. `form`, `prerender` and `+server.ts` are out.
