# Adversarial review: native release preparation (round 02)

## Target

Commit `e2784c9` "feat(native): prepare release projects without rebuilding web
or Debug" on `codex/native-release-prepare` (base `e5e83bc`, working tree clean
apart from this review):

- `cmd/skgo/native.go` — flags `--prepare-only`, `--reuse-web-build`, `--jobs`;
  usage line updated.
- `internal/nativebuild/build.go` — `Build()` now wraps an injectable executor
  around `build(o, run)`; `Options.PrepareOnly/ReuseWebBuild/Jobs`; manifest
  check instead of `vp build`; `-j<N>` for zig; `-jobs N` for xcodebuild; early
  return after xcodegen; rejects negative `Jobs`.
- `internal/nativebuild/prepare_test.go` — three tests over a fake executor.
- `README.md` — paragraph documenting the three flags.
- `ephemeral/worklog/native-release-prepare.md` — decision and friction entries.

Objective, as in round 01: the Gimbal release critical path
(`.../gimbal-local-release-cache/gimbal/ephemeral/worklog/release-critical-path.md`),
which needs skgo to prepare native libraries and the Xcode project without
rebuilding the frontend or a Debug app, with Mac workers capped at four;
`scripts/release-macos.sh:205` invokes
`go tool skgo native build --platform macos --prepare-only --reuse-web-build --jobs N`.

Caller narrowing: none that limits subject matter. "Check that the validation is
load-bearing" is a scope emphasis, not a restriction; "do not edit any other
files" and the artifact path are operating constraints and were honoured. The
only files touched were a scratchpad copy of the repo used for mutations.

## Evidence inspected

- Full diff against `e5e83bc`; the whole of `build.go`, `native.go`,
  `prepare_test.go`, `build_test.go`, and `internal/templates.Project` (what the
  fixture has to satisfy).
- `gofmt -l` clean; `go vet` clean; `go test ./internal/nativebuild` passes in
  0.14s. `go test ./cmd/skgo` still fails in this shell on ambient pnpm 12.11.2
  versus the example's declared 12.9.1 (`kit_patch_test.go`), which predates the
  branch and is recorded in both worklogs as passing under vp's environment.
- `/tmp/skgo-native-release-prepare.log` (unchanged since round 01, 08:30): one
  prepare run ending at "Prepared native project:", no `vp build` and no
  xcodebuild output. The worklog's claim that the subsequent unsigned Release
  build and the byte-identical bundled CLI also passed is Gimbal-side and was
  not re-verified here.
- `zig build --help` under the pinned mise toolchain in `native/core`: `-j<N>`
  "Limit concurrent jobs". Running `zig build library -j0 ...` there prints
  `error: number of jobs must be at least 1` (no artifacts were produced).
- Mutation testing of the new suite in a scratchpad copy of the repo (the
  worktree itself was not modified). Each mutation was applied to `build.go`
  alone and the package tests re-run:

  | # | Mutation | Caught? |
  |---|----------|---------|
  | M1 | `PrepareOnly` ignored (xcodebuild still runs) | yes, command list |
  | M2 | `ReuseWebBuild` ignored (`vp build` always runs) | yes, two tests |
  | M3 | zig `-j<N>` dropped | yes |
  | M4 | xcodebuild `-jobs N` dropped | yes |
  | M8 | xcodebuild flag misspelled `-j4` | yes |
  | M13 | zig flag split as `-j 4` | yes |
  | M6 | manifest check accepts an empty file | yes |
  | M9 | manifest looked up in the wrong directory | yes |
  | M10 | prepare-only returns before xcodegen | yes |
  | M11 | reuse also skips the `c-archive` build | yes |
  | M5 | `-j4` prepended to the `mise` argv, before `--` | **survives** |
  | M7 | negative `Jobs` accepted | **survives** |
  | M12 | `vp build` run in `p.Root` instead of `p.Web` | **survives** |
  | M14 | `-j0` / `-jobs 0` emitted when `Jobs` is unset | **survives** |

  Round 01's finding 1 asked for exactly M1–M4; all four now fail the suite, so
  that finding and finding 2 (xcodebuild `-jobs` untested) are resolved. Finding
  4 (usage line, README) is resolved. Finding 3 (presence, not freshness) stands
  as documented behaviour; the flag help and README now say the caller owns the
  revision, which matches what the check enforces. Finding 5 (`--jobs` not
  reaching Go) is now stated in the README.

## Findings

### 1. Issue — the default command line is never asserted, and a regression there breaks every plain `skgo native build`

All three tests construct `Options{Jobs: 4}` (`prepare_test.go:51`). No test
runs with `Jobs` at its zero value, which is what every invocation without
`--jobs` uses, including all three README examples. Mutation M14 (drop both
`if o.Jobs > 0` guards) keeps the suite green, and the resulting `-j0` is a hard
error in the pinned zig ("number of jobs must be at least 1"), so the whole
build would fail at the zig step for everyone while `go test` reports ok. M7
(negative `Jobs` accepted) survives for the same reason: the guard at
`build.go:54` has no test. A fourth case with `Jobs: 0` asserting that neither
`-j` nor `-jobs` appears, plus one for a negative value returning the
"nonnegative" error, closes this in milliseconds. CLAUDE.md: a contract must
fail if the feature is broken; today the default path is the one path with no
contract.

Evidence: `prepare_test.go:51`; `build.go:54,165-167,205-207`.

### 2. Issue — the zig worker assertion checks presence, not where the flag lands

`prepare_test.go:67` joins the whole `mise` argv and counts `-j4` in it. The
flag is only meaningful after `--`, where mise hands the rest to zig; M5 moves it
to the front of the `mise` argv (mise would reject it as an unknown flag) and the
test still passes. The same join-and-count approach is used for xcodebuild
(`prepare_test.go:88`), where position matters less because xcodebuild accepts
options anywhere. The test should compare the argv after `--` to a literal
`[]string{"zig","build","library","-Dtarget=aarch64-macos","-Doptimize=ReleaseFast","-j4"}`
rather than searching a string.

Evidence: `prepare_test.go:67`; `build.go:163-168`.

### 3. Nitpick — the fake executor discards `dir` and `env`

`nativeFixture`'s `run` ignores its first two parameters (`prepare_test.go:40`),
so the recorded commands cannot show that `vp build` runs in `web/` (M12
survives), that `ORIGIN` is set for it, or that the `c-archive` build receives
`CGO_ENABLED`, `GOOS` and the `CC=... -target ... -isysroot ...` line. None of
that changed in this branch, so it is not a regression, but the executor seam is
new and recording `{dir, env, name, args}` costs nothing. Worth doing when
finding 1 is addressed.

### 4. Nitpick — the only end-to-end proof is still a single undated log without its command line

`/tmp/skgo-native-release-prepare.log` has not changed since round 01. It shows
the prepare path once and cannot show the Release build that followed, and it
does not record the argv that produced it; the worklog's statement that the
Release app built and bundled a byte-identical CLI lives only in Gimbal's
worklog. With the Go suite now covering the command plan, this is acceptable as
the human "opened the box" step, but it should not be cited as proof of the
`-jobs` xcodebuild path, which Gimbal never reaches.

## Outcome

material findings remain
