# Stage 2 RequestEvent review — page and layout event types (Opus 5.5)

## Target

Uncommitted stage-2 work on `codex/request-event-plan` over `afd2fc5`, in
`/Users/tyler/.codex/worktrees/bad1/skgo`. Stage-2 contract (plan table):
page and layout handlers receive their correct parameter domains, including
descendant optionality, matcher alternatives, route groups and layout resets;
colocated page/layout handlers compile with distinct aliases; accessors retain
per-load tracking. Kit pin: `example/web/node_modules/@sveltejs/kit` reports
`"version": "3.0.0"`.

The launch prompt contains no scope narrowing beyond operating constraints.

## Evidence inspected

- `AGENTS.md`/`CLAUDE.md`, `ephemeral/plans/20261006-request-event.md` (all of
  it, with §3 and the stage table as the stage-2 contract), the worklog diff.
- Full diffs of `loadevent.go`, `request_event.go`, `load.go`,
  `internal/gen/{emit,scan,load_params}.go`, new `internal/gen/layout_params.go`
  and `layout_params_test.go`, test/testdata/example signature updates, and the
  generated example `example/web/src/routes/skgo_gen.go`.
- Kit `src/core/sync/create_manifest_data/index.js` (file walk, duplicate-file
  detection, `child_pages` construction at ~L430–460).
- Runtime construction of load events (`loadevent.go:48`) and every site that
  sets `Event.hook` (`middleware.go:222` only).

## Checks run

| Check | Result |
| --- | --- |
| `just test` at the current candidate (no stage-2 broad pass was recorded) | **exit 1.** `internal/gen` ok (370s), `internal/newapp` ok, `example/...` ok, `internal/dev` ok. Failures: `internal/adapter` (4 tests, stage-2 caused — Findings 1–2); `cmd/skgo` and `internal/kitpatch` (pnpm environment — see Environment) |
| `go test -count=1 -run TestStage2 ./internal/gen` in an isolated copy (`/tmp/opus-s2mut`) | ok, 37.9s |
| Mutant A: `LoadRouteIDValue` records a route dependency | **killed** — `node 0 unexpected uses … Route:1` |
| Mutant B: replace Kit `child_pages` with directory-prefix membership | **killed** — `TestStage2DomainShapes` plus 4 reset/named-layout rejection subtests fail |
| Mutant C: revert `Event.RouteID` to hook-before-load ordering | **survived** — see nonblocking N1 |
| e2e fixture `example/e2e/fixtures/go-dev-added` copied into the example (in the copy) and `go generate ./internal/skgo/` | **fails**: `page.server.go:12:21: undefined: RequestEvent` |

What the stage-2 suite proves well: Kit's real `create_manifest_data` decides
membership (groups, `+layout@`, `+page@`, `+page@(group)` all distinguished and
compile-rejected where excluded); descendant-only keys become optional; one
key with several Go matcher types becomes a sealed union scoped to that
layout's participating pages; false/""/nil-pointer/nil-interface presence is
distinct from absence; construction records nothing, accessors record only the
reader, `Untrack` suppresses; colocated aliases have distinct identity; root
fallback construction is representable; wrong page/layout/shared/locals
domains are rejected by both `Run` and `Check`. Expected values are literals
in the fixture, not read back from generated metadata.

## Blocking findings

### 1. Issue — generation now fails when stale stubs of the other module extension exist (verifiable bug; `just test` red)

`internal/gen/load_params.go` (the `kitLoadParams` readdir overlay) injects a
pending `+page.server.<ext>` only when the *same basename* is absent. When the
directory already holds the generated stub in the other language, Kit sees
both and throws `route_duplicate_files`. Before stage 2, Kit's manifest builder
was not invoked here and stale stubs were reconciled later by generation.

Reproduction (from `just test`):

```
--- FAIL: TestDeclaredPrerenderInputsBuildAndProduceNativeArtifacts (55.15s)
    prerender_inputs_build_test.go:462: JavaScript-mode go generate: exit status 1
        skgo: reading Kit route params: … create_manifest_data/index.js:291
        SvelteKit error: route_duplicate_files
        Multiple server page module files found in `src/routes/(marketing)/pricing` : `+page.server.js` and `+page.server.ts`
```

Impact: regenerating an app in the other frontend language (or over any
leftover generated stub of the other extension) hard-fails before generation
can clean up its own output. The overlay must treat SKGo-owned stubs of either
extension as the same pending module (or hide stale owned stubs from Kit).

### 2. Issue — in-tree consumers still use the removed `RequestEvent` load alias (incomplete requirement)

Plan §3/§5: "Replace the existing ambiguous route-local `RequestEvent` alias …
Update every in-tree consumer to the new configuration and signatures." Two
consumers were missed:

- `internal/adapter/prerender_inputs_lifecycle_test.go:274` matches the literal
  `func pageLoad(event RequestEvent)` in the copied example's `about` page, which
  now reads `PageRequestEvent`. Three tests fail before exercising anything:
  `TestPrerenderInputsBuildAppWaitsForUnrelatedCrawlerFailureDrain`,
  `TestPrerenderInputsVPOwnerSignalDrainsBlockedProducer`,
  `TestPrerenderInputsOuterVPCLISignalDrainsBlockedProducer` —
  `about load signature changed; lifecycle injection was not installed`.
- `example/e2e/fixtures/go-dev-added/page.server.go.txt:12` still declares
  `func pageLoad(event RequestEvent)`. Installed into the example and generated,
  it fails with `undefined: RequestEvent`, so the dev-mode scenario in
  `example/e2e/features/zz-source-update.feature:46–49` ("answered 200 in dev",
  load reading "New Go route revision one") cannot pass in `just e2e`.

Both are mechanical fixes, but they leave `just test` red and break a dev-mode
browser scenario on the branch.

## Nonblocking bugs (for the orchestrator to file)

- **N1. Worklog trap is false; the RouteID reorder is not load-bearing.** The
  worklog says hook-first `Event.RouteID` "bypassed load dependency tracking"
  and that the served fixture fails if the fix is removed. Mutant C reverted the
  ordering and the full stage-2 handler test still passed. Load events are
  built fresh at `loadevent.go:48` with no `hook`; the only `hook:` assignment
  is `middleware.go:222`, and no code copies a hook event and adds `load`. The
  reorder is harmless, but the worklog's stage-3 note ("URL(), SearchParam(),
  and raw Param() still have the same hook-first ordering … need explicit
  served-boundary tracking assertions") is based on a path that does not
  currently exist and should not drive stage-3 work as written. Correct the
  worklog entry.

## Nitpicks

- `internal/gen/scan.go:527` still reports `load must be func(RequestEvent) (Data, error)`; the domain-specific message follows it.
- Generated union variant types (`IDParam_Number`, `Key_ID`) share the route package namespace with authored code; a same-named authored type fails compilation with a generic error.

## Environment (not attributed to stage 2)

`cmd/skgo` `TestKitPatchCLIConfiguresFilesAndDoesNotClaimInstallation` and 10
`internal/kitpatch` tests fail with `native pnpm version = "12.10.1", <nil>;
want 12.9.1`; `pnpm --version` on this machine is 12.10.1. Neither package is
touched by the diff. Under the repository's rule these are still failures, not
passes: `just test` cannot be green on this machine until the pinned pnpm is
the one on PATH (or the pin moves).

## Outcome

material findings remain
