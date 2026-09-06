# Sprint 003: Server logic written in Go

## Mission

A developer writes a remote function in Go, colocated with the routes that use
it, and the SvelteKit app calls it with end-to-end types — no hand-written glue,
no TypeScript stub kept in sync by hand. Authentication works: a handler can read
and write cookies under the same rules kit enforces on its own remote functions.

Kit is the specification. `ephemeral/inspiration/reference/kit/` is the pinned
source; where kit has already answered a question, match its answer, including
its constraints and not merely its ergonomics. `ephemeral/brief/` and
`ephemeral/plans/` describe the authoring model this sprint implements.

## Acceptance

Scenarios, in the example app, proven against a production build with Vite
stopped and again under `vp dev`:

- A Go remote function is reachable from the app with no hand-written
  `.remote.ts` in the repository.
- Changing a Go type changes what the app sees: a mismatch between Go and the
  component using it fails the build rather than reaching a browser.
- A visitor signs in and the session survives across requests. Signing out ends
  it.
- A signed-in visitor sees data a signed-out visitor cannot.
- One command mutates a todo and the open page shows the new value without a
  second request to the server.
- Everything that passed before this sprint still passes.

`go vet` and `go test` green. The binary refuses to start rather than serving a
silently broken app when generated output and the built frontend disagree.

## Ownership

One builder owns this whole mission. Sprint 004 (server loads) is a separate
mission and does not overlap.

## Constraints

Go only — JavaScript exists solely where kit requires it. CSR only; there is no
sidecar and no SSR. Never reimplement kit. polytype (`@v1.0.0-rc.9`, pinned) is a
code generator to be run and consumed, never called as a library; if it cannot do
what is needed, say so rather than working around it in skgo.
