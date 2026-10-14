# Adversarial review: dev-server adoption plan, round 01

## Target

`ephemeral/projects/dev-server/PLAN.md` (capabilities 1–13, coverage table, unresolved leads, delivery section), reviewed against `CLAUDE.md`, the user's stated adoption direction, and the paired upstream sources in `/Users/tyler/src/skgo-token-cache`. Plan review only; nothing was implemented or edited.

## Caller narrowing

The launch prompt's calibration instructions (report only concrete feature breakage or operational harm; no new requirements, hardening, proof machinery or expanded testing; do not manufacture product decisions from legacy tests; do not reintroduce a native loader project) are user-authored scope and match `CLAUDE.md` ("Build the software", "no command means done"). I treated them as scope, not as a verdict request. No ignored narrowing: the review still covered the whole plan and the cited sources broadly.

## Evidence inspected

- `CLAUDE.md` rules: mirroring, Go-only SKGO implementation, no application I/O in JS, no Node at request time, no gate runners.
- Cited upstream anchors, opened and compared with what the plan says they do:
  - gopls v0.52.0 `filewatcher/fsnotify_watcher.go` (drain, `walkDirWithRetry`, rename-as-delete, `skipDir`/`skipFile`, joined `WaitGroup`), `cache/session.go` glob patterns, `cache/view.go`, `cache/snapshot.go`, `filecache/filecache.go` `Set`, `mcp/mcp.go` `StartStdIO`.
  - x/tools `internal/gocommand/invoke.go` (`inFlight`/`serialized`, load-concurrency serial retry, `PWD`, `WaitDelay`, `runCmdContext` interrupt-then-kill).
  - Vite 8.3.4 `optimizer/index.ts` (`runOptimizeDeps` processing dir, cancel/commit rename sequence), `ssr/fetchModule.ts` (`inlineSourceMap`, `startOffset`), `server/hmr.ts` (environment-snapshot staleness guard), `server/index.ts`, `server/environment.ts`.
  - Kit pinned `runtime/client/client.js` `refreshAll`.
  - goja `parser/statement.go` `parseSourceMap` / `extractSourceMapLine`.
  - MCP go-sdk v1.8.0 `go.mod` and `server.go`.
- Baseline `skgo-adoption-baseline-53179fc`: `internal/adapter/skgo-adapter/env.js` (`inlineSourceMap: false`, `/__skgo_dev/changed` cursor), `internal/vite/runner.go` (`compile` wrapper), `internal/dev/server.go` (`noticeEdit`), `cmd/skgo/mcp.go` (handwritten JSON-RPC), `internal/ssr/ssr.go`, `go.mod`.

## Verification of the plan's load-bearing claims

- **R6 mapped errors works as described.** `runner.go` wraps module code as `"(async function …{\n\"use strict\";\n" + code + "\n})"`. Vite appends the inline map as the last line, and goja's `extractSourceMapLine` skips trailing empty lines and a lone `})`, then decodes `data:application/json` URLs. A two-line `startOffset` is exactly what the wrapper prefix needs. The independent small delivery is sound.
- **Capability 1 source and caveats are accurate.** The gopls watcher does what the plan says. It also filters to `.go/.mod/.sum/.work/.s` and skips dot-, underscore- and `testdata` directories. The plan already requires substituting SKGO's input policy and excluding generated-output loops. It states the missed-event and removed-root limits instead of overclaiming.
- **Capability 2** matches `gocommand`: the serial retry is specifically the concurrent go.mod-update load error, not a general retry.
- **Capability 10**: Kit's `refreshAll` exists in the pinned client and preserves `page.state`, as the plan says.
- **Capability 11**: go-sdk v1.8.0 requires Go 1.25; the baseline is 1.27.1. The baseline's MCP is handwritten JSON-RPC, so the replacement is real.
- **Capabilities 12/13**: Vite's commit renames the old directory aside and then deletes it. The plan already flags that a literal port would be wrong for `sv` link targets and says to publish only immutable new targets. The gopls `Set` content-addressed-then-index ordering supports that.
- **Capability 3–7 ownership**: goload lives in polytype, not SKGO. The plan routes loader changes through a polytype dependency update and forbids a private second loader, so this is consistent with its own boundary.

## Findings

No critical or issue-level findings. I did not find a mapping that the cited source contradicts, a capability whose ordering would break a working dev path, or a step that would produce concrete feature breakage or operational harm if followed as written.

### Nitpicks

1. **Several line anchors drift from the symbol the prose names.** `gocommand/invoke.go:295` lands on a blank line (`runCmdContext` is at ~298); `cache/snapshot.go:489` lands at the end of a function (`buildOverlays`, the "historical overlays" target, is at ~490); `vite/server/environment.ts:137` is the module-graph construction, not the hot channel (`hot: NormalizedHotChannel` is declared at ~109); `vite/server/index.ts:930` is `reloadOnTsconfigChange`, while the watch-change → `onHMRUpdate` ordering is at ~919–990. The builder is told to map upstream independently, so this costs a few minutes, not correctness.
2. **Capability 1 names Vite's watcher ordering at `index.ts:930` but says both watchers must not become competing Go invalidation paths.** It does not say which side owns a file both observe (e.g. a route `+page.svelte`). The sentence "Avoid competing Go invalidation paths through both Go and Vite watchers" is the whole answer; an implementer will have to pick. This is a decision point for the builder, not a defect in the plan.

## Outcome

only nitpicks remain
