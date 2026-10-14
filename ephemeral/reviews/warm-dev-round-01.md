# Adversarial review: warm generation worker and shared dev supervisor (round 01)

Date: 2026-10-14. Reviewer: Claude Fable 5.1, read-only except this file.

## Review target

- Repository: skgo, worktree `/Users/tyler/.codex/worktrees/skgo-warm-dev`, branch `codex/warm-dev-supervisor`.
- Committed: `8af4493 feat(generate): retain dev analysis and omit unchanged projections` (one commit ahead of `origin/main` at `525bf42`). Adds `internal/gen/worker*.go`, the `generate.Worker` export, projection-key reuse, retained private source view, retained Polytype `goload.View`, SKGO-owned `types.ts` headers and narrowed pruning.
- Uncommitted: `internal/dev/{server,watch,inputs,reload}.go` and tests, `dev/dev.go` (new public package), `cmd/skgo/dev.go` (in-process `Worker`, owned Vite moved into the supervisor), adapter plugin `goSupervisorReload` in `internal/adapter/skgo-adapter/env.js`, e2e feature/steps for browser reload and overlay, 21 regenerated example `types.ts` headers, a `go.mod` `replace` of polytype to a local worktree, probe changes under `ephemeral/`.
- Companion sources: Polytype `/Users/tyler/.codex/worktrees/polytype-warm-analysis` (commits `da484b4`, `a1420d7` plus uncommitted `goload/view.go` clone fix); Gimbal `/Users/tyler/src/gimbal-worktrees/gimbal-warm-dev` (`internal/generate/appgen/dev.go`, `dev_inputs.go`, `Justfile` `dev` recipe, `go.mod` replaces to `/tmp/gimbal-dev-producers/*`).
- Specification: GitHub issue #311 and its three comments; CLAUDE.md rules; the caller's restated goal (correct low-latency edits in SKGO and Gimbal, omit unnecessary generation, keep diagnostics and browser recovery current).

Caller constraints honoured: read-only, artifact path. No narrowing of scope was requested; none was applied.

## Evidence inspected

- Full `git diff` of the worktree and `git show 8af4493`; the worker, cache, cache_source, prune, types, codecs, snapshot changes; all of `internal/dev`; `dev/dev.go`; the adapter plugin and its Node-driven test; e2e feature and steps.
- Gimbal: `appgen dev` command, `generatorFiles`, `go list -deps ./internal/generate/appgen` (main-module deps now `workflow`, `internal/builtin`, `internal/generate`, `appgen`; `internal/observation` import removed by `45fb79de`), `internal/builtin` embeds (`defaults.json` only), worklog.
- Polytype: `go doc ./goload View` (PrepareView/Update/Load/LoadWithFallback/Close), uncommitted clone fix, latest release `v1.8.0` (2026-10-14) which does **not** contain `PrepareView` (verified against `$GOMODCACHE/.../polytype@v1.8.0/goload`).
- Installed Vite (`@voidzero-dev/vite-plus-core@1.0.0`) declares the `vite:client:connect` event the plugin relies on.
- Test run (this worktree, `go build ./... && go vet ... && go test -count=1 ./internal/gen/ ./internal/dev/ ./cmd/skgo/ ./internal/adapter/ ./dev/ ./generate/`):

| Package | Result |
| --- | --- |
| `internal/dev` | ok (38.8s), all 12 tests including the four new ones |
| `internal/gen` | FAIL: 5 top-level tests + 6 `TestEvolvedApplication` subtests; all worker tests pass |
| `cmd/skgo` | FAIL: `TestNativePluginHelpApplicationAndProjectTool`, `TestKitPatchCLIConfiguresFilesAndDoesNotClaimInstallation` (pnpm 12.11.2 vs pinned 12.9.1, environmental, not attributed to this branch); the four `TestSkgoDev*` tests pass |
| `internal/adapter` | FAIL: 13 tests, every one at `go generate` of a sandbox app; `TestGoSupervisorUsesViteReloadAndErrorProtocol` passes |

- `example/e2e/test-results/warm-dev/.last-run.json` reports `passed`; which mode and which scenarios ran is not recorded there, so the browser-reload and overlay steps are treated as unverified by this review.
- `internal/gen/worker_test.go` changed on disk during the review (a new `TestWorkerDeclarationEditWithEmbedDirectoriesRetainsDiscovery` appended by a concurrent session). It was not part of the run above.

## Test load-bearing assessment

- `TestWorkerBodyEditsSkipGenerationAndPublishFreshSource`: load-bearing. Asserts `OnPackageLoad` count is 0 and projection parses are exactly 1 for a body edit, checks the fresh body reached the executable copy, checks a foreign Polytype `types.ts` survives, compares to a full `Run`. A removed memo or a widened `projectionKey` fails it.
- `TestWorkerProjectionInputsRemainExact`: load-bearing on the key; each listed edit (build tag, comment, tag, const, var, field type, signature) must change the key.
- `TestWorkerFieldEditOmitsUnchangedTypeProducer`: asserts exactly one TypeScript run and one hit across two producers, then compares with full generation.
- `TestDevWatchesCapturedReplacementSourcesAndNewEmbedMatches`: edits an out-of-root replacement module's embedded text with mtime preserved and adds a new embed member; literal expected bodies. Would fail on the old size/mtime fingerprint or if `WatchInputs` were ignored.
- `TestDevCanTransferAnExclusiveApplicationResource`: the fixture holds a TCP lease; the replacement cannot listen unless the predecessor exited first. Load-bearing for ordering, but it runs without Vite and so does not observe the reload defect in finding 2.
- `TestFirstReadyDoesNotQueueABufferedBrowserReload` and `TestViteReceivesLatestBuildAfterStartingLate`: exercise `settle` and the retry loop directly with literal payloads; neither covers the `StopBeforeStart` path.
- `TestGoSupervisorUsesViteReloadAndErrorProtocol`: drives the plugin with a fake Vite server object; asserts token/403, 400s, exact `error` and `full-reload` payloads, late-joiner replay, and no stale replay after repair. Load-bearing for the plugin, not for real Vite wiring.
- `TestSkgoDevListenFailureDoesNotStartViteChild` replaced a stronger cleanup proof with an ordering proof; acceptable since Vite now starts inside `Run`, after listen.

## Findings

### 1. Critical — the committed worker only compiles against an unreleased Polytype reachable through an uncommitted local `replace`

`internal/gen/worker.go:23` uses `goload.View`; `internal/gen/cache_source.go:694` calls `cfg.Session.PrepareView`. Neither exists in Polytype `v1.8.0`, the version pinned in `go.mod:9` and `example/go.mod:11`. The only thing making the tree build is the uncommitted `go.mod:30` `replace github.com/tylergannon/polytype => /Users/tyler/.codex/worktrees/polytype-warm-analysis`.

Reproduction: any `go generate` run from an application module resolves `github.com/tylergannon/skgo/internal/gen` with released Polytype and fails:

```
internal/gen/worker.go:23:26: undefined: goload.View
internal/gen/cache_source.go:694:34: cfg.Session.PrepareView undefined
```

That is the single cause of all 11 `internal/gen` failures, 13 `internal/adapter` failures and `TestNativePluginHelpApplicationAndProjectTool` in `cmd/skgo`. Gimbal's `go.mod` likewise depends on `replace` lines to `/tmp/gimbal-dev-producers/{skgo,polytype}` that must not land. Issue #311 says "land the bounded, independently verified ... gains first" and CLAUDE.md says a passing command is not evidence; here the suite is red, and the commit as it stands would break every consumer on `main`. Required ordering: release Polytype with `View`/`PrepareView`, bump both `go.mod` pins, drop the replaces, then rerun the full suite. Polytype's uncommitted `goload/view.go` clone fix also needs to be in that release.

### 2. Critical — with `StopBeforeStart`, a successful rebuild never tells the browser to reload

`internal/dev/server.go:462-470` (`build`): when `StopBeforeStart` is set, the supervisor takes `s.current`, sets `s.current = nil`, and retires the predecessor before starting the replacement. `settle` at `server.go:361-372` then reads `previous := s.current`, which is now nil, and only calls `s.reload` when `previous != nil || next.diagnostic != ""`. For a successful build both are false, so no `full-reload` is sent.

Causal chain for Gimbal, which sets `StopBeforeStart: true` in `appgen/dev.go`: edit a Go file → generate → compile → old process drained and stopped → new process starts → `settle(next)` with `previous == nil` → silence. The browser keeps the pre-edit page until the developer navigates by hand. Diagnostics still arrive (the `next.diagnostic != ""` branch), and a repair after a failure also arrives, so the gap is exactly the common case: a correct edit. This defeats the caller's "keeping browser recovery current" for Gimbal and the e2e scenario "shows ... without manual navigation" only runs against the SKGO example, which does not set the flag. `TestDevCanTransferAnExclusiveApplicationResource` cannot see it because it configures no Vite. Fix shape: decide the broadcast on whether this is the first settle (e.g. a `settled-once` flag or `s.builds > 1`) rather than on `s.current` being non-nil.

### 3. Issue — every HTTP request and every 150 ms poll hashes the complete Go tree's contents

`internal/dev/watch.go:99-113` (`stat`) now reads and SHA-256s every file instead of recording size/mtime (a correct response to the worklog's "equal-size timestamp-preserving writes" trap). `fingerprint()` is invoked from the poll loop (`server.go:327`, default `Poll` 150 ms), from `awaitChange`'s quiet check, and synchronously from `noticeEdit` (`server.go:579`) on **every** proxied request before it is forwarded.

Measured on the Gimbal worktree, which the Gimbal supervisor roots at the repository: 761 Go files, 6.4 MB, about 80-90 ms wall per full read-and-hash from a warm page cache (`find | xargs cat | shasum`, as an approximation of the Go walk). On the SKGO example the tree is 228 KB and the cost is negligible. For Gimbal this adds roughly 80 ms to each request's latency in dev and keeps the poll loop busy more than half the time, which contradicts the issue's primary aim ("edit-to-browser loop" no worse than SvelteKit) and the worklog's own note that "the per-request polling cost needs measuring before claiming fast browser latency". That measurement has not been done in a Go test. Options that keep the exact-contents guarantee: stat first and hash only files whose size/mtime changed, cache per-file digests keyed by (size, mtime, inode) across polls, or make `noticeEdit` consult the last poll's result instead of re-walking. Whichever is chosen needs a test that asserts the preserved-mtime edit is still noticed, which already exists, plus one bounding the per-request work.

### 4. Issue — Gimbal's `RestartFiles` turn authored generation inputs into "restart required"

`appgen/dev_inputs.go` builds `RestartFiles` from `go list -deps ./internal/generate/appgen`, adding `<dir>/*.go` for every main-module package. After `45fb79de` those are `workflow`, `internal/builtin`, `internal/generate` and `appgen`. `workflow` is also a generation input: the `project` callback runs `go tool polytype --typescript ../web/src/lib/workflow` from that directory. So an edit to any `workflow/*.go` type definition makes `internal/dev/server.go:412-416` return "Generator implementation changed. Restart the development command" instead of regenerating. That is precisely a "Go API/type edit" the issue lists as a required fixture, and for Gimbal the warm loop does not cover it. Either `workflow` stops being compiled into appgen (the issue's third comment already names this coupling and the `internal/generate` metadata contract) or the restart set is narrowed to true generator implementation files. The restart diagnostic itself is correct behaviour for real generator edits and `TestRestartInputsIncludeMembershipAndPreservedTimestamp` proves it.

Related, lower weight: `appgen/dev.go` still reruns the workflow Polytype projection after SKGO "until skgo#313 isolates ownership". The committed `pruneStaleArtifacts` change (`internal/gen/prune.go:87-90`) now preserves foreign Polytype headers outside `src/lib/skgo`, and `TestPrunePreservesForeignPolytypeOutput` proves it, so the Gimbal workaround is now dead cost in every edit and its comment is stale.

### 5. Issue — `StopBeforeStart` moves the 30 s drain onto the critical path while all requests are held

`server.go:466-468` calls `s.retire(ctx, previous)` inside `build` with `building == true` and `s.current == nil`, so every new request blocks in `awaitSettled`. `retire` (`server.go:384-399`) waits for `previous.requests.Wait()` or 30 s. Previously the drain ran in a background goroutine after `settle`, so a long-lived response only delayed the old process's exit, never the new generation. Gimbal serves `text/event-stream` responses (`internal/observation/http.go`, `internal/pi/providers/openai/stream.go`); a browser tab holding one such stream now stalls every Go edit for 30 s with the page frozen, and the stream itself is cut rather than drained. The exclusive-lock constraint is real (worklog), but the drain budget for the stop-before-start case should be short or should actively close idle streams, and a test should hold a streaming response during a `StopBeforeStart` rebuild and bound the replacement time. `TestDevDrainsAnAssignedResponseBeforeRetiringItsApplication` covers only the overlapping path.

## Nitpicks (not counted above)

- `viteClientPath()` (`internal/dev/reload.go:118-124`) reads and parses the supervisor JSON on every request; cache it after first success.
- `cmd/skgo/dev.go` creates `gen.Worker` as a local variable captured by two closures; it works because the methods take a pointer, but a `*gen.Worker` would make the "must not be copied after first use" rule visible.
- The uncommitted probe changes under `ephemeral/warm-worker-probe` hard-code the Polytype worktree path as a default; fine for ephemeral material, but it should not be the fallback once the release exists.
- A concurrent session is appending tests to `internal/gen/worker_test.go` during this review; coordinate before committing (`gh pr list`, `git worktree list`).

## Outcome

`material findings remain`
