# Adversarial review: Worker source-capture optimization (round 02)

## Target

- Branch `codex/worker-source-capture` at `31fa0e5` "fix(generate): reject unsafe metadata-only source retention", PR #318 (head verified equal to local HEAD; working tree clean).
- Net production diff versus the branch base `d69891c` (= tag `v0.32.1`): one line in `internal/gen/cache_source.go:403`, reordering the ownership predicate so the cheap generated-header prefix test runs before the two `EvalSymlinks` calls inside `withinTree`. Plus one new test, `internal/gen/worker_capture_test.go` (`TestWorkerCapturePreservedMtimeEditsMatchAuthoritativeGeneration`).
- Research material: `ephemeral/worker-capture-probe/` (now its own Go module with `go.mod`/`go.sum`), six probe logs, `ephemeral/worklog/worker-source-capture.md` with a new rejection entry.
- Specification: issue #311 and repository `CLAUDE.md`, as in round 01.

Caller narrowing: none ignored.

## What changed since round 01

The retained-byte layer (`worker_capture.go`, the three platform files, the `Worker.capturedFiles` field, the two retention tests and the benchmark) was deleted outright in `31fa0e5`. The worklog records the rejection and the reason (round 01 finding 1), and the PR body states that no retained-byte or platform-specific metadata logic remains. That is accurate: `git diff d69891c HEAD -- internal/` shows only the predicate reorder and the new test. The probe moved into its own module (round 01 nitpick 4) so `./...` no longer compiles it.

## Evidence inspected and run

- Full net diff and the surrounding `cacheOwned`, `capture`, `captureWithRead` code; the remaining test and its helpers (`foreignFixture`, `cacheWrite`, `assertCacheFile`, `generatedTree`).
- Worklog and PR body claims cross-checked against the logs recomputed in round 01: the quoted predicate-only numbers (239.06 ms to 224.24 ms, 37.45 MB to 31.16 MB) match the logs.
- `go vet ./internal/gen` clean; `gofmt -l` clean on the touched files.
- macOS: `TestWorkerCapturePreservedMtimeEditsMatchAuthoritativeGeneration` with `-race`: pass. Full `go test ./internal/gen -count=1`: `ok` in 115 s.
- Linux (`golang:1.27-bookworm`, kernel `6.12.76-linuxkit`, repo mounted read-only, cross-compiled test binary): the remaining test passes 3 of 3 runs.
- PR #318 CI: "Build, tests, and Conventional Commit" and "Swift client and generated production HTTP calls" both pass on the current head.
- Load-bearing check by mutation, using `go test -overlay` against scratchpad copies of `cache_source.go` so no project file was touched:
  - Mutation B, disabling the whole web/src ownership clause: 17 tests fail (`TestGenerationCache*`, `TestWorker*`), so the predicate itself is covered.
  - Mutation A, dropping the `withinTree(cfg.Web/src)` term so any file carrying a generated-code header and a generated-module basename is treated as owned regardless of location: the entire `internal/gen` suite passes. See nitpick 1.
  - The remaining test is load-bearing for its stated claim: an in-place same-size edit with mtime restored, then an atomic replacement with mtime restored, each must produce a fresh projection and byte-identical output to an authoritative `Run`. Reintroducing any stat-only retention would fail it on Linux, which is exactly what it exists to pin.

## Findings

No material findings remain. The production change is a semantics-preserving reorder of two pure predicates, the unsafe layer is gone, and every claim in the PR body is backed by the logs or by tests that run on both platforms CI uses.

### Nitpick 1 — the `withinTree` half of the reordered predicate has no test

`internal/gen/cache_source.go:403`: with the location guard removed (mutation A), all 100-plus `internal/gen` tests still pass. The guard protects embedded or native resources outside `web/src` whose basename happens to match a generated module name and whose content starts with the generated-code header; today nothing exercises that. The reorder does not change behaviour, so this is a pre-existing coverage gap rather than a regression, but it is the only line the PR touches and half of it is unverified.

### Nitpick 2 — the probe module cannot reproduce the worklog measurements as committed

`ephemeral/worker-capture-probe/go.mod` requires `github.com/tylergannon/skgo v0.32.1` with no `replace`. Inside this repository the root `go.work` resolves skgo to the local tree, but the worklog says the probe runs from the cloned Gimbal application's root (outside this workspace), where it would measure the released `v0.32.1`, which predates the reorder. The logs stand as evidence of the earlier run; the committed probe does not reproduce them without a workspace or replace directive. Research material only; no production impact.

### Nitpick 3 — test file and helper names still describe the deleted feature

`worker_capture_test.go` and the failure message "retained capture differs from authoritative generation" refer to retention that no longer exists. The test's real claim is "timestamp-preserving edits are never missed by a Worker"; naming it that would stop a future reader from looking for a capture cache.

## Outcome

only nitpicks remain
