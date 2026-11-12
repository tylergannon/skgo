# Adversarial review: native release automation (round 01)

Outcome: **material findings remain**

## Target

- Commit `3b3c7c94 feat: automate qualified native-shell release candidates` on
  worktree `native-release-public` (working tree clean apart from this review
  and the session log).
- Files: `.github/workflows/native-plugin.yml`, `.github/workflows/release.yml`,
  `cmd/skgo-native-release/{main.go,main_test.go,notes.go}`,
  `ephemeral/worklog/native-release-automation.md`.
- Requirements: the caller's restated list (automatic candidate on official
  core Release, manual dispatch with optional next-patch allocation, generic
  `NATIVE_RELEASE_DISPATCH_REPOSITORY/TOKEN` delivery that fails visibly when
  absent, qualification-gated promotion of original bytes, immutable reruns and
  pairings, authoritative run-metadata validation, release notes with real
  links, host, prerequisites and copyable pinned consumer instructions) plus
  `CLAUDE.md` and the agent protocol.
- No caller instruction narrowed scope; "do not edit other files" is an
  operating constraint and was honoured.

## Evidence inspected

- Full diff of both workflows against `c86dda6f`; full source and tests of
  `cmd/skgo-native-release` (840/489/88 lines). `go test`, `go vet`, `gofmt -l`
  on the package: pass/clean (12 tests).
- Surrounding code the notes recipe depends on: `cmd/skgo/update.go`,
  `internal/update/{update,complete}.go`, `cmd/skgo/plugin.go`
  (`--prebuilt/--sha256/--select` exist), `plugins/native-shell/plugin.go`
  (`build --platform`), `plugins/native-shell/nativebuild/build.go`
  (default macOS deployment target `14.0`, nested `mise -C <sdk> exec`),
  `plugins/native-shell/example/README.md`, `internal/toolchain/pins.go`,
  `example/mise.toml`, `plugins/native-shell/sdk/mise.toml` (all pin regexes
  match the current files), `internal/pluginbuild/build.go` (plugin build uses
  `GOTOOLCHAIN=<host Go>`, so `setup-go stable` in CI is harmless).
- Live repository state: tags (`plugins/native-shell/v0.1.0` at `45e471a8`),
  releases (`plugins/native-shell/v0.1.0` for v0.46.0, no `candidate.json`
  asset), recent `Release` runs (6 of the last 8 were `workflow_dispatch`),
  native runs 43138337596 / 43138974838.
- Real-API check of the immutability chain: the `digest` field of artifact
  13908013772 equals `sha256` of the `/actions/artifacts/{id}/zip` download
  (`1f3803a6…898b`), so `downloadArtifact`'s digest comparison is sound.
- Local check of the recipe's mise usage: `mise trust` on a project directory
  also trusts nested config files (`mise trust --show` lists `sub: trusted`),
  so the copied SDK's `mise.toml` under `native/build/macos/.sdk` does not need
  `MISE_TRUSTED_CONFIG_PATHS` as the v0.1.0 notes required.
- Worklogs: `ephemeral/worklog/native-release-automation.md`,
  `ephemeral/worklog/202611021538-published-v0460-consumer.md`.

## Findings

### 1. issue — Generated consumer recipe is unproven and differs from the only verified path

`cmd/skgo-native-release/notes.go:44-83` emits a new fresh-consumer recipe
(copy `plugins/native-shell/example` out of the source tarball, write a
`mise.toml`, `skgo update --complete-version`, `skgo plugin install --prebuilt`,
`skgo plugin native-shell build`, `open …/TinyNative.app`). The recipe that was
actually published and walked for v0.1.0 (`gh release view
plugins/native-shell/v0.1.0`) is different (`skgo new` + `skgo add native-app`).
Nothing in the commit, the worklog or `ephemeral/` shows the generated
`notes.md` executed end to end on a clean machine; the only test
(`TestFreezeToolsUsesSelectedCoreDeclarations`, `main_test.go:341-383`) asserts
substrings of the template.

Concrete hazards the recipe carries that a run would have surfaced:

- `skgo update --complete-version` (`internal/update/complete.go:41-53`) runs
  `vp upgrade <VitePlus>` when the first `vp` on PATH reports another version.
  Under `mise exec` with `"npm:vite-plus" = "@VP@"` that should be exact, but
  the consumer journal (`202611021538-published-v0460-consumer.md`) records
  this very step needing a PATH shim/`--vp` last time; it has not been shown to
  work from the recipe's mise-managed `vp`.
- The example the recipe copies is documented (same journal, last line) as
  deliberately *not* advanced to the current core; the recipe relies on
  `update --complete-version` to carry it from v0.41.0-era pins to the selected
  core in one step. Plausible, unproven.
- `notes.go:65` hardcodes the extracted directory as `skgo-@SOURCE@` while every
  URL is derived from `GITHUB_REPOSITORY`; a rename or fork breaks the copy
  step silently.

Requirement missed: "copyable fresh-consumer instructions". `CLAUDE.md`
Testing: "No recipe means 'done': someone looks at the running app." Impact:
the release notes are the artifact consumers copy; a broken step ships as part
of an immutable release and cannot be corrected without editing a published
release body by hand. Accepted proof: one run of the generated `notes.md`
block, verbatim, on a clean directory against the real v0.46.0 CLI, ending
with the TinyNative window open.

### 2. issue — A failed build strands an explicit plugin version; blank versions burn a patch per failure

`main.go:295-326` (`reserve`) creates `refs/tags/plugins/native-shell/<v>` at
`prepare` time, before the build; `main.go:441-467` only reuses a candidate
when an artifact exists. After any failed build (compiler error, runner loss,
`gh release download` hiccup) there is no artifact, so a rerun of the same run
goes back through `reserve`:

- `workflow_dispatch` with an explicit `plugin_version`: `POST git/refs`
  returns 422 and `prepare` fails with `reserve immutable plugin version vX:
  GitHub API 422` (`TestReservationNeverOverwritesExistingTag` proves the
  path). The run can never be rerun; the operator must dispatch again with a
  *different* version, and the requested version is a permanent dangling tag
  with no release.
- `workflow_run` or blank version: `nextVersion` (`main.go:276-294`) skips the
  dangling tag and reserves the next patch, so each failed attempt consumes a
  version and leaves a tag that `publishedPair` (which only reads releases)
  never explains.

The worklog accepts "gaps are safe", but the explicit-version dead end is not
a gap, it is an unrecoverable rerun, which conflicts with "Reruns … must
preserve immutable release bytes" in spirit: a rerun that produced no bytes
should be able to produce them. Fix direction: when the reserved tag already
exists and resolves to the current `GITHUB_SHA` (`tagSHA`) and no release
exists for it, treat the reservation as owned by this run instead of failing.

### 3. issue — Every successful `publish=false` Release dry run produces a failed native run on the macOS runner

`native-plugin.yml` `candidate.if` admits any successful `Release` run;
`main.go:395-415` then fails with "successful core run has no published
identity artifact", and `TestNonPublishingCoreRunCannotScheduleCandidate`
(`main_test.go:447-466`) enshrines the red run as the intended outcome.
`release.yml:13-16` documents `publish=false` as the way to "qualify runner
changes" and the run history shows `workflow_dispatch` is the common case.
Impact: a red "Native plugin release" run after every runner qualification,
each one first claiming `skgo-managed-macos` for checkout + Go setup. Those
expected failures look identical to the failure this design relies on being
noticed ("absent config fails visibly"), so the signal the requirement asks
for is diluted. Fix direction: make the dry run distinguishable before the
macOS job starts (e.g. a cheap Linux gate job that inspects the upstream run's
`publish` job conclusion, or an always-uploaded identity artifact carrying
`published: false` that `prepare` treats as a clean no-op skip in the `if:`).

### 4. nitpick — Cross-repository dispatch token is exported to every step of every job

`native-plugin.yml` workflow-level `env:` maps
`NATIVE_RELEASE_DISPATCH_TOKEN` into all jobs, including the macOS `candidate`
job's `go test -tags skgo_qualification ./plugins/...` and the plugin build,
none of which need it (`prepare` only checks presence/format, `main.go:499-504`).
Only `notify` and `promote` send it. Scope it to those steps.

### 5. nitpick — Promotion is gated on the run's *latest* attempt, not the authoritative attempt

`main.go:539-545` requires `GET actions/runs/{run}` (latest attempt) to be
`success` even though every other check is against `r.Attempt`. A later
rerun of an already fully successful run (e.g. attempt 2 re-sending the
candidate notification while the qualification repository is down) flips the
run to `failure` and blocks promotion of the unchanged attempt-1 artifact until
yet another rerun succeeds. The attempt-level job check at `main.go:546-563`
already establishes the build succeeded; the latest-attempt check adds only
this failure mode.

## Verified as sound (not findings)

- Core identity: `release.yml:413-428` uploads the identity only from the
  publishing job; `prepare` verifies run path/event/conclusion, run id, the
  published non-draft release and that the tag resolves to the recorded SHA.
- Delivery: both notifications go only to the configured repository with the
  configured token (`main.go:506-513`); missing/invalid config fails `prepare`,
  `notify` and `promote` before any mutation.
- Promotion: `authorize` validates attempt metadata, head SHA, workflow path,
  the `candidate` job conclusion, the artifact id/name/digest, the manifest
  equality, file digests, the reserved tag's SHA and the core identity; the
  release is created from the downloaded original bytes with `--verify-tag
  --latest=false`; reruns re-download and compare, mismatches fail
  (`TestPromotionPublishesOriginalBytesAndRecoversNotification`).
- Pins in notes come from the selected core source via the contents API and
  from the native checkout at `GITHUB_SHA`; supported host and deployment
  target (`macOS 14`) match `nativebuild/build.go:90`.
- Legacy `plugins/native-shell/v0.1.0` lacks `candidate.json` and is invisible
  to `publishedPair`; its source (`45e471a8`) differs from any future default
  branch head, so no false pairing or duplicate can arise from it.
