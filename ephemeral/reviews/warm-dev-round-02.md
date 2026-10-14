# Adversarial review: warm generation worker + shared dev supervisor, round 02

Date: 2026-10-14 (local). Reviewer: Claude (Fable 5.1), read-only except this file.

## Review target

- skgo worktree `/Users/tyler/.codex/worktrees/skgo-warm-dev`, branch `codex/warm-dev-supervisor`.
  At the start of the round the DWARF/`--debug` work was uncommitted; during the round it was
  committed as `b9cfabe feat(dev): shorten linking while preserving explicit debug builds`
  (14 files). The working tree was clean when this review was written. Branch is 8 commits
  ahead of `origin/main` (525bf42 = tag v0.31.0): 80 files, +3381/-404.
- Authoritative sources: issue tylergannon/skgo#311 (outcomes, learnings, evidence required),
  CLAUDE.md, the round-01 review (`ephemeral/reviews/warm-dev-round-01.md`), and the caller's
  restated requirement: development builds omit DWARF by default for faster linking, retain Go
  symbols and native plugin compatibility, and `--debug` restores default debug data while
  respecting explicit caller flags.
- Polytype: released v1.9.0 (gh release 2026-10-14T08:04Z); module cache copy at
  `$GOMODCACHE/github.com/tylergannon/polytype@v1.9.0/goload/view.go` defines `PrepareView`.
- Gimbal integration: `/Users/tyler/src/gimbal-worktrees/gimbal-warm-dev` at `64be8583`
  (`feat: preserve live generation and expose debugger builds`), with uncommitted `go.mod`
  replaces and regenerated TypeScript.

No scope-narrowing instruction was given; none was ignored.

## Evidence inspected

Code read in full or in the relevant regions: `internal/dev/{server.go,build.go,build_test.go,
reload.go,reload_test.go,digest.go,digest_darwin.go,digest_other.go,inputs.go,watch.go,
server_test.go}`, `dev/dev.go`, `cmd/skgo/{dev.go,dev_test.go}`, `internal/adapter/skgo-adapter/env.js`
(`goSupervisorReload`, lines ~1278-1312), `internal/adapter/dev_watcher_test.go`,
`internal/gen/worker*.go` (round 01; diffed against the Gimbal producer copy this round),
`example/{buildfresh_test.go,generate_test.go,typedrift_test.go}`, Gimbal
`internal/generate/appgen/{dev.go,dev_inputs.go}`, `registry.go`, `Justfile`, `go.mod`.

Checks run with the repository tool environment:

| Check | Result |
| --- | --- |
| `CI=true just test` (background, full recipe) | 34 packages ok; `example` FAILED (2 tests), see finding 5 |
| `vp exec go test -count=1 -run 'TestTheEmbeddedBuild…|TestNothingGenerated…' ./example` rerun serially | ok, 17.3s |
| `internal/dev` incl. `TestDevBuildRetainsSymbolsAndOffersDWARF`, `TestDevCanTransferAnExclusiveApplicationResource`, `TestDevExclusiveReplacementClosesEventStreams`, `TestFingerprintReusesBytesAndDetectsPreservedMtimeEdits` | ok, 49.4s |
| `internal/gen` (worker tests), `internal/adapter`, `cmd/skgo` (`--debug` launch test) | ok, 136.9s / 45.1s / 85.9s |
| Gimbal `go build ./internal/generate/appgen` | builds, via `/tmp/gimbal-dev-producers/*` replaces |
| Scratchpad experiment: `go build -buildmode=plugin` + host built with `-ldflags=-w`, `plugin.Open` + `Lookup("Register")` | plugin loads and runs; host 4,252,162 B → 3,036,658 B |

Load-bearing assessment of the proof the branch offers:

- `TestDevBuildRetainsSymbolsAndOffersDWARF` reads the produced `tmp/app-1` with `debug/macho`
  (or elf/pe) and checks for `__DWARF`/`.debug_*` presence per case (fast default absent; `--debug`,
  caller `-ldflags=-w=false` in BuildFlags, caller GOFLAGS, present; explicit `-ldflags=-w` under
  Debug absent) and asserts `go tool nm` lists ` main.main`. Each assertion fails if `buildFlags`
  is removed. Load-bearing.
- `TestSkgoDevAdoptsAuthoredEndpointsAndActionsInOneLaunch` passes `--debug` and inspects the
  fixture's own executable path for DWARF. Load-bearing for the flag; the CLI default path is
  proven only through `internal/dev`.
- `TestDevCanTransferAnExclusiveApplicationResource`: a fixture holds a TCP lease, replacement
  under `StopBeforeStart` must take it, a fake Vite must receive exactly one empty-diagnostic
  reload within 3s and none at first readiness. Round-01 finding 2 (no reload after exclusive
  replacement) is closed by this test.
- `TestDevExclusiveReplacementClosesEventStreams`: real `text/event-stream` closed at retire.
  Round-01 finding 5 (30s drain on critical path) is closed for streams.
- `TestFingerprintReusesBytesAndDetectsPreservedMtimeEdits`: digest reuse and ctime detection.
  Round-01 finding 3 (hashing cost) is reduced; see finding 2 for what remains.
- Round-01 finding 1 (unreleased Polytype API) is closed: `go.mod` and `example/go.mod` pin
  v1.9.0, the local replace is gone, and the sandbox-based tests that failed in round 01 pass.

## Findings (most severe first)

### 1. issue — Gimbal still integrates against an unreleased, already-drifting copy of skgo

Gimbal `go.mod` (uncommitted) ends with
`replace github.com/tylergannon/skgo => /tmp/gimbal-dev-producers/skgo` and the same for
polytype. `/tmp/gimbal-dev-producers/skgo` is not a git checkout and already differs from this
branch: `internal/dev/build_test.go` is absent there and `internal/dev/{reload_test.go,
server_test.go}`, `internal/gen/{worker.go,worker_projection.go,worker_test.go}` differ
(`diff -rq`). The released skgo Gimbal declares, v0.31.0 = `origin/main` 525bf42, has no
`dev` package field `Debug`, `StopBeforeStart` or `WatchInputs`, so Gimbal's `appgen dev`
(`internal/generate/appgen/dev.go:137-142`) does not compile against anything published.
Impact: the #311 requirement "the same technology for SKGO and Gimbal" is demonstrated
only against a snapshot that is neither this branch nor a release, so a Gimbal result does not
certify this branch and cannot be reproduced after `/tmp` is cleared. Resolution is sequencing
(merge + tag skgo, then bump Gimbal), but until then the Gimbal evidence should be labelled
as taken against a stale copy.

### 2. issue — every proxied request still pays a serialized full stat scan

`internal/dev/server.go:605-609`: `ServeHTTP` forwards only `isViteClient` paths and
upgrades straight to Vite; every other request, including each Vite module and asset request
in dev, calls `noticeEdit()` → `s.fingerprint()` (`inputs.go:74`) → `fileDigests.scan()`
(`digest.go:35`), which takes `c.mu` for the whole walk and releases it in `close()`
(`digest.go:42-46`). The scan stats every watched file (768 files in the worklog's Gimbal
measurement, 1.97ms per scan after digest reuse) and all concurrent requests queue on that
mutex, as does the 150ms poll. A cold Gimbal page issuing a few hundred module requests
therefore serializes a few hundred full scans behind one lock, on the order of hundreds of
milliseconds of added latency per cold load, scaling with repository size. Round-01 finding 3
reduced the per-scan cost; it did not remove the scan from the request hot path. A cheap fix
exists without losing the "hold a request that races the poll" guarantee: debounce
`noticeEdit` to at most one scan per poll interval, or scan only when the last scan is older
than a few milliseconds.

### 3. issue — Gimbal workflow type edits still demand a restart instead of regeneration

`internal/generate/appgen/dev_inputs.go:15-45` globs `<dir>/*.go` for every main-module
dependency of `appgen`; `go list -deps` includes `github.com/tylergannon/gimbal/workflow`
because `internal/generate/{analysis.go,command.go}` import it. Those globs become
`RestartFiles`, so editing any Go file under `workflow/` (the authored input of the workflow
`codegen.Worker` and the #311 "API edit" case for Gimbal) yields the "Generator implementation
changed. Restart the development command" diagnostic rather than a warm regeneration. Round-01
finding 4 persists. The in-generation `go tool polytype` rerun (the #313 workaround) has been
reduced to a single startup run (`dev.go:51`), which is an improvement; the remaining coupling
is the import, and the fix belongs in Gimbal (separate the generator's compiled model from the
package it generates from, or exclude authored generation roots from `RestartFiles`).

### 4. nitpick — `buildFlags` shells out on every build; CLI default path untested at the CLI

`internal/dev/build.go:22-27` runs `go env GOFLAGS` per build (a fresh `go` process on each
edit) although GOFLAGS cannot change for the lifetime of the supervisor process. Resolve it
once in `New`. `cmd/skgo/dev_test.go` exercises only `--debug`; the fast default is proven in
`internal/dev` and by my own plugin experiment, so this is coverage shape, not a gap. Native
plugin compatibility (`plugin.Open` into a `-ldflags=-w` host) is correct by construction
(`-w` is link-time only; the plugin runtime compares compile-time package hashes) and was
demonstrated in the scratchpad experiment above, but no repository test states it.

### 5. nitpick — `just test` is not isolated from a dev session on the same checkout (pre-existing)

The full recipe failed in `example`: `TestTheEmbeddedBuildIsNewerThanTheFrontendSource`
reported `web/src/routes/go-dev/go-dev.remote.ts` changed at 02:11:34 after the build at
02:10:11, and `TestNothingGeneratedWasWrittenByHand` saw `web/build/*` differ. The real
example tree's `go-dev.remote.go` was rewritten at 02:11:33, `web/build` at 02:13 and
`.svelte-kit/skgo-dev-supervisor.json` at 02:15:35, i.e. a concurrent dev/e2e session was
driving the same checkout during the run (git shows no content change). The serial rerun
passed. Not attributable to this branch, but the recipe's exit code was wrong for an
environmental reason the recipe cannot detect, which CLAUDE.md treats as a failure to report.

## Requirement check: DWARF / `--debug`

Implemented in `internal/dev/build.go` and plumbed through `cmd/skgo/dev.go` (`--debug`) and
Gimbal `appgen dev --debug` (`dev.go:31,138`). Default prepends `-ldflags=-w`; it is skipped
when `Debug` is set, when any `-ldflags`/`--ldflags` form appears in `BuildFlags`, or when
`go env GOFLAGS` contains `-ldflags`, which matches Go's rule that an explicit `-ldflags`
replaces rather than merges, so caller flags are respected. Symbols retained (nm shows
`main.main`), plugin loading verified. Requirement met.

## Outcome

material findings remain
