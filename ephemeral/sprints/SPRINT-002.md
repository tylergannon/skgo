# Sprint 002: Remote functions in Go — `query`, `query.live`, `command`

## Pyramid Index

- L0: Go answers SvelteKit remote-function requests for `query`, `query.live`, and
  `command` against a production build, proven by Playwright BDD scenarios.
- L1:
  - Port kit's wire protocol to Go: djb2/base36 id hash, devalue stringify/parse,
    remote-arg codec (`__skrao`/`__skram`/`__skras`, base64url), response envelopes.
  - Dispatch `/{base}/{appDir}/remote/<hash>/<name>`: GET query (`q` node carries the
    value), GET live (SSE), POST command (`_` + `refreshes` → `q` + `r:true`), plus
    error/redirect/404 envelopes and the prod Origin check.
  - Typed Go authoring API + registry, mounted ahead of the static handler and the dev
    proxy in `example/cmd/main.go`.
  - Example app: hand-written `.remote.ts` stubs whose bodies **throw**, so any rendered
    value proves Go answered.
  - Proof: Gherkin scenarios under `example/e2e/features/` against the production binary
    with Vite stopped; `go vet` and `go test` green.
- L2: `ephemeral/sprints/drafts/SPRINT-002-INTENT.md` for protocol detail and constraints.

## Scope

In: `query` (with and without an argument), `query.live` (SSE), `command` (with
single-flight refresh). Out: `query.batch`, `form`, `prerender`, `__data.json`, generated
bindings, polytype.

## Definition of Done

1. Production build (`vp build` + `go build`), Vite stopped: list renders from Go, a
   command mutates and refreshes in one round trip, an argument-taking query resolves on a
   deep link, and a live query updates the page as Go pushes values.
2. Gherkin scenarios for those four flows pass against the production binary.
   Dev (`vp dev` behind the Go server) answers remote calls from Go too.
3. `go vet ./... ./example/...` and `go test ./... ./example/...` pass.
4. Bar is working and not broken at ~90-95%. No gold plating.
