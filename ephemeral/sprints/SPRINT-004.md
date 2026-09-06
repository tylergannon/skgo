# Sprint 004: Server loads, and one place to make a trust decision

## Mission

A developer writes server loads in Go — a layout's load and its pages' loads
composing as one navigation, streaming slow data as it resolves — and authorizes
requests in **one** place rather than in every handler.

Today authorization is per-function: each remote function calls `signedIn(ctx)`
itself, and the one that forgot (`watchCount`) discloses a private todo's
existence to a signed-out visitor while eleven scenarios stayed green. That is a
missing shape, not a missing check. Kit has already solved this — `handle` in
`hooks.server.js`, and layout loads guarding their subtree — so the shape is not
yours to invent. Map it.

Kit is the specification. `ephemeral/inspiration/reference/kit/` is the pinned
source. Loads compose and stream; both are the feature, and a page-only,
non-streaming implementation is a shrink fallback, not the target.

## Acceptance

Scenarios, in the example app, against a production build with vite stopped and
again under `vp dev`:

- A visitor is redirected out of a protected section before any page in it
  renders, and the rule that redirects them is written once — adding a second
  page to that section requires no new guard.
- A signed-out visitor's todo count matches what that visitor can actually see.
- A layout's data and its page's data arrive together on a cold load, and the
  page can read what its parent loaded.
- A page shows its fast content before its slow content has arrived, and the
  slow content appears without a second request.
- A load that fails puts the visitor on an error page instead of a blank one.
- Everything that passed before this sprint still passes.

Every scenario leaves a screenshot of the page in the state it asserts, and
whoever runs them looks at all of them. `go vet` and `go test` green, with no
check able to skip and still report success.

## Ownership

One builder owns this mission. A separate effort ports the junkyard's guestbook
onto skgo as a second app; it does not touch this one's files, and its job is to
find walls, not to pass scenarios.

## Constraints

Go only — JavaScript exists solely where kit requires it. CSR only; no sidecar,
no SSR. Never reimplement kit. `form` and `prerender` are out of scope.
