# Adversarial review: native release preparation (round 03)

## Target

Branch `codex/native-release-prepare` at `b1422d5` (PR #320, base `e5e83bc`,
working tree clean apart from this review). Two commits:

- `e2784c9` feat(native): prepare release projects without rebuilding web or
  Debug — `cmd/skgo/native.go` flags `--prepare-only`, `--reuse-web-build`,
  `--jobs`; `internal/nativebuild/build.go` executor seam `build(o, run)`,
  `Options.PrepareOnly/ReuseWebBuild/Jobs`, manifest check in place of
  `vp build`, `-j<N>` for zig, `-jobs N` for xcodebuild, early return after
  xcodegen, negative `Jobs` rejected; README paragraph; worklog entries.
- `b1422d5` test(native): cover worker defaults and exact Zig arguments —
  `prepare_test.go` now compares the zig argv to a literal, adds a `Jobs: 0`
  default case and a negative-`Jobs` case.

Objective, unchanged from rounds 01 and 02: the Gimbal release critical path
(`.../gimbal-local-release-cache/gimbal/ephemeral/worklog/release-critical-path.md`),
which needs skgo to prepare native libraries and the Xcode project without
rebuilding the frontend or a Debug app, with Mac workers capped at four.
`scripts/release-macos.sh:210` invokes
`go tool skgo native build --platform macos --prepare-only --reuse-web-build --jobs "${GIMBAL_BUILD_JOBS:-4}"`
and line 215 passes the same `-jobs` to Gimbal's own Release xcodebuild.
Gimbal's `go.mod` pins skgo `v0.32.2` with no `go.work`; `native/` and
`internal/nativebuild/` are unchanged between `v0.32.2` and `main`, so the
pinned SDK the log copied is the one this branch builds against.

Caller narrowing: none that limits subject matter. "Check that the validation
is load-bearing" is an emphasis, honoured by mutation testing below. "Do not
edit any other files" and the artifact path are operating constraints and were
honoured; mutations ran in a scratchpad copy of the repository.

## Evidence inspected

- Full diff `main...HEAD`; the whole of `build.go`, `native.go`,
  `prepare_test.go`, `build_test.go`; `ci.yml` trigger logic.
- `gofmt -l` clean; `go vet` clean; `go build ./...` ok;
  `go run ./cmd/skgo native build -h` lists the three flags with the README's
  wording.
- `go test ./internal/nativebuild` ok (0.16s). Under vp's environment
  (`cd example/web && CI=true vp exec go -C ../.. test ./cmd/skgo
  ./internal/nativebuild`) both packages pass (44.9s, 0.15s). Round 01/02's
  ambient-pnpm failure in `kit_patch_test.go` does not reproduce under vp, which
  is the only supported way to run it.
- CI: the `pull_request` runs show SKIPPED, which is `ci.yml:20` skipping
  same-repo PRs by design; the `push` run on `b1422d5` (38582558945) completed
  with both jobs `success`. PR #320 is MERGEABLE.
- `/tmp/skgo-native-release-prepare.log` was regenerated since round 02
  (08:35, "generation cache hit"): one prepare run, no `vp build`, no
  xcodebuild, ends at "Prepared native project:". It still does not record its
  argv and cannot show skipped steps; that gap is now closed by the Go suite.
- `xcodebuild -help` documents `-jobs NUMBER`. Round 02 established that the
  pinned zig rejects `-j0`.
- Mutation testing of `build.go` against the current suite, one mutation at a
  time in a scratchpad copy:

  | # | Mutation | Caught? |
  |---|----------|---------|
  | M1 | `PrepareOnly` ignored | yes |
  | M2 | `ReuseWebBuild` ignored | yes |
  | M3 | zig `-j<N>` dropped | yes |
  | M4 | xcodebuild `-jobs N` dropped | yes |
  | M5 | zig `-j4` placed before `--` (mise would reject) | yes (was survivor in round 02) |
  | M6 | empty manifest accepted | yes |
  | M7 | negative `Jobs` accepted | yes (was survivor) |
  | M8 | xcodebuild flag misspelled `-j4` | yes |
  | M9 | manifest looked up under root, not web | yes |
  | M10 | prepare returns before xcodegen | yes |
  | M11 | reuse skips the `c-archive` build | yes |
  | M14 | `-j0`/`-jobs 0` emitted when `Jobs` unset | yes (was survivor) |
  | M15 | `-jobs` emitted twice | yes |
  | M18 | prepare skips `skgo generate` | yes |
  | M19 | zig `-Doptimize=ReleaseFast` dropped | yes |
  | M20 | prepare skips zig | yes |
  | M21 | reuse skips `skgo generate` | yes |
  | M22 | `Jobs` ignored when preparing | yes |
  | M23 | Debug configuration changed | yes |
  | M12 | `vp build` run in root instead of `web/` | **survives** |
  | M16 | `ORIGIN` env dropped from `vp build` | **survives** |
  | M17 | a directory named `skgo.manifest.json` accepted | **survives** |

  Round 02's findings 1 (no default-`Jobs` contract) and 2 (zig flag position
  unchecked) are resolved by M14, M7 and M5 now failing the suite. The three
  survivors are covered by the nitpicks below; none is on the new behaviour the
  objective asked for, and M12/M16 are behaviour the branch did not change.

## Findings

No material findings remain. The requested capability is implemented as the
objective specifies, the Go contracts fail under every mutation of the new
behaviour that was tried, the Gimbal prepare run and the CI gate are green,
and the README and usage line match what the code enforces.

### 1. Nitpick — the `IsRegular` branch of the manifest check is unexercised

`build.go:157` rejects a manifest that is not a regular file, but the only
negative cases in `TestReuseRequiresNonemptyFrontendManifest` are absent and
empty (`prepare_test.go:120-129`). Mutation M17 (drop `!IsRegular()`) survives.
A third loop case that replaces the file with a directory costs two lines.
Practical risk is near zero.

### 2. Nitpick — the fake executor still discards `dir` and `env`

`prepare_test.go:40` ignores the first two parameters, so M12 (`vp build` in
the wrong directory) and M16 (`ORIGIN` dropped) survive. Both are pre-existing
command lines that this branch did not alter, so this is not a regression and
was recorded as round 02 nitpick 3. Recording `{dir, env}` alongside the argv
would make the seam cover the whole command plan, including the `CC=...
-isysroot` line for the `c-archive` build.

### 3. Nitpick — the flag-to-`Options` wiring has no Go test

`native.go:22-24` is three `fs.*Var` lines with nothing in `cmd/skgo` tests
touching them. The Gimbal log proves `--prepare-only` and `--reuse-web-build`
reach `Options` (no `vp build`, ends at "Prepared"), but nothing observable
proves `--jobs` reaches `Options.Jobs`: zig is silent on success and the log
carries no argv. The code is plainly correct on reading; noted because it is
the one link in the chain that only a human eye has verified.

### 4. Nitpick — the xcodebuild `-jobs` path is still proven only by test and `-help`

Gimbal passes `--prepare-only`, so `build.go:206-208` was never executed by a
real xcodebuild. The exact-argument test and `xcodebuild -help` make it very
likely correct; a plain `skgo native build --jobs 4` on any native example would
close it. Acceptable to ship; mentioned so no one cites the Gimbal log as proof
of this path.

## Outcome

only nitpicks remain
