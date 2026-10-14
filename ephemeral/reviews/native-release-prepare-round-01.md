# Adversarial review: native release preparation (round 01)

## Target

Uncommitted working tree on `codex/native-release-prepare` (base `e5e83bc`):

- `cmd/skgo/native.go` — new flags `--prepare-only`, `--reuse-web-build`, `--jobs`.
- `internal/nativebuild/build.go` — `Options.PrepareOnly`, `Options.ReuseWebBuild`,
  `Options.Jobs`; manifest check in place of `vp build`; `-j<N>` for zig; `-jobs N`
  for xcodebuild; early return after xcodegen.
- `ephemeral/worklog/native-release-prepare.md` — friction/decision entry.

Objective: the Gimbal release critical path
(`/Users/tyler/.codex/worktrees/gimbal-local-release-cache/gimbal/ephemeral/worklog/release-critical-path.md`),
which asks skgo for a minimal upstream change so Gimbal can prepare native
libraries and the Xcode project without rebuilding the frontend or a Debug app,
capping Mac workers at four.

Caller narrowing: none that limits subject matter. "Do not edit any other
files" and the artifact path are operating constraints and were honoured.

## Evidence inspected

- Full diff and surrounding `Build()` body; `build_test.go`; `nativeapp` config.
- `/tmp/skgo-native-release-prepare.log` (37 lines, one run). It shows generate,
  xcrun, xcodegen and the new "Prepared native project:" line. No `vp build`
  output and no xcodebuild output appear, consistent with both flags being honoured.
- `go list -m -json` inside that log resolves skgo to `v0.32.2` in the module
  cache. Gimbal's `go.mod` pins `v0.32.2` with no `replace` and no `go.work`, so
  the run was driven by a locally built binary while the SDK it copied and the
  `go tool skgo generate` it ran were the pinned release's. `git diff v0.32.2 HEAD
  -- native/ internal/nativebuild/ cmd/skgo/native.go` is empty, so that
  substitution does not change what was exercised.
- Gimbal's `scripts/release-macos.sh:205` invokes
  `go tool skgo native build --platform macos --prepare-only --reuse-web-build --jobs N`.
  `web/dist.go` embeds `all:build`, so the reused `web/build` is what lands in
  `skgo-host.a`.
- `zig build --help` confirms `-j<N>`; `xcodebuild -help` confirms `-jobs NUMBER`.
- `go build ./...` and `go vet` pass. `go test ./internal/nativebuild` passes.
  `go test ./cmd/skgo` fails in this shell on `kit_patch_test.go:43` because the
  ambient pnpm is 12.11.2 and the example declares 12.9.1; the Gimbal worklog
  records the same failure and a pass under vp's resolved environment. This
  predates the diff.

## Findings

### 1. Issue — the new behaviour has no load-bearing test

`build_test.go` only covers `projectSpec` and config validation. Nothing fails if
`ReuseWebBuild` is ignored and `vp build` runs anyway, if `PrepareOnly` is ignored
and xcodebuild runs, if `Jobs` is dropped from either command line, or if the
`-jobs` argument is spelled wrong for xcodebuild. The only validation is the
single `/tmp` log, which does not record the command line it was produced by and
by construction cannot show that two steps were *skipped* (zig and `go build` are
also silent on success, so the log is equally consistent with them not running).
CLAUDE.md: "A passing command is not evidence on its own" and "Contracts are Go
tests". `Build()` shells out inline, so the fix is to lift the command plan (the
ordered list of `{dir, env, argv}`) into a pure function and assert, with literal
fixtures, that prepare-only ends at xcodegen, reuse omits `vp build` and fails
on a missing or empty manifest, and `Jobs` appears exactly once per tool. That
test runs in milliseconds and would have caught every mutation above.

Evidence: `internal/nativebuild/build.go:151-167,197-204`; `build_test.go` (no
reference to any new field).

### 2. Issue — `-jobs` on the xcodebuild path is unexercised by the proof

The Gimbal release never reaches skgo's xcodebuild call (it passes
`--prepare-only` and runs Release itself), so `build.go:202-204` was not run by
the preparation log and has no test. `xcodebuild -help` documents the flag, so it
is very likely correct, but as shipped it is untested code on the default
(`skgo native build` without `--prepare-only`) path that every non-Gimbal user
hits when they pass `--jobs`. Covered by the same test as finding 1; listed
separately because it is a change to the default path, not the new one.

### 3. Nitpick — `--reuse-web-build` validates presence, not freshness

The check at `build.go:152-155` accepts any non-empty `skgo.manifest.json`. The
flag text promises "the frontend already built for this source revision", but
`Build()` runs `skgo generate` *after* the web build would have run and *before*
the archive is linked, and the log shows "generation cache miss" with projected
`types.ts` files rewritten under `web/src`. In a clean release checkout these
are byte-identical to what was committed, so the reuse is sound, and the worklog
explicitly assigns revision selection to the caller. A stale `web/build` from a
different revision would still pass this check and be embedded silently via
`web/dist.go`. Acceptable under the stated decision; the help text should not
claim more than the check enforces.

### 4. Nitpick — usage string and README not updated

`cmd/skgo/native.go:14` still prints
`usage: skgo native build [--root DIR] [--platform ...] [--preset NAME]`; the
three new flags appear only in `-h`. `README.md:483-485` lists the native build
commands without the new options. Minor, but the usage line is the first thing a
wrong invocation shows.

### 5. Nitpick — `--jobs` does not reach the Go steps

The Gimbal objective says "four Go/Zig/Xcode workers". `Jobs` is applied to zig
and xcodebuild only; `go tool skgo generate` and `go build -buildmode=c-archive`
run with Go's defaults. The flag help honestly says "Zig/Xcode", and Gimbal can
set `GOFLAGS=-p=4` since `run` inherits the environment, so this is a scope note
rather than a defect.

## Outcome

material findings remain
