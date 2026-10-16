# Adversarial review: dev → main readiness (round 01)

Date: 2026-10-15

## Target

Range `1e9ecaab4e7c80308e4dc43e154920b179caedff` (main) .. `3cb90f033e1698289dbe7cc5f93eec790697a55d` (dev): 36 commits, 292 files, +16474/−15735. Question under review: is dev ready to merge into main and be retired. Authoritative context taken from AGENTS.md/CLAUDE.md, `ephemeral/projects/dev-server/GOAL.md`, `DESIGN.md`, `CONTINUE.md`, README.md and the available proof.

Scope note: the launch prompt contained no narrowing of defects, files or subject matter and predicted no verdict; nothing was ignored. The checkout also carries unrelated uncommitted edits in `internal/kitpatch/` and `cmd/skgo/kit_patch_test.go` plus untracked `ephemeral/ci-repair-*.log`; those are outside the range and were not reviewed.

## Evidence inspected

- Full range diff by file; read in full: `internal/devserver/{session,router,frontend,config,process*}.go`, `internal/devserver/session_integration_test.go`, `internal/hotreload/host.go`, `dev_bridge.go`, `native_development.go`, `dev_kit.go`, `dev_fetch.go`, `dev_remote.go`, adapter `dev-host.js`, `dev.js`, `dev-modules.js`, `dev-bridge.js`, `dev-predicates.js`, newapp templates, example `server.go`/`cmd/main.go`.
- Production-path diffs (`internal/ssr/ssr.go`, `csp.go`, `document*.go`, `endpoint.go`, `remote.go`, `load.go`, `fetch.go`, `prerender_*.go`, `environment.go`, `middleware.go`, `static.go`): removal of the old `Dev` flag, dev Vite runner, dev style-CSP branch and `LoadDevEnvironment`; no production behaviour change observed beyond deletion of dev-only branches.
- CI/qualification/Justfile/package.json/go.mod diffs; docs (`docs/*.html`, `SVELTEKIT_COMPATIBILITY.md`, README).
- Worklogs under `ephemeral/worklog/2026101[45]-*.md` and the prior review `ephemeral/reviews/20261015-native-inputs-round-02.md`.
- Ran, read-only: `go vet ./... ./example/...` (exit 0); `go test -count=1 ./internal/gen ./internal/hotreload ./internal/devserver .` (all ok; gen 153.8s, hotreload 112.1s). The `skgo_dev_integration`-tagged two-session test was not run here (requires a live Vite child); its coverage was read instead.

## Findings

### 1. issue — incomplete requirement: shipped documentation contradicts the shipped `skgo dev`

The range adds `skgo dev` and a README "Development" section describing it as available, while the same tree still tells developers development is unavailable:

- `docs/features.html:105` — "Development server and hot updates … Unavailable … pending its separate replacement."
- `docs/index.html:131` — "Development mode is unavailable pending its separate replacement."
- `AGENTS.md:133` and `CLAUDE.md:133` — "development qualification is unavailable pending its separate replacement".
- `.github/workflows/qualification.yml:10` — same comment; dev matrix removed.
- `internal/newapp/gofiles/Justfile.tmpl:10` — every newly scaffolded app ships a Justfile saying "Development mode is unavailable pending its separate replacement" while its own `cmd/main.go` now contains the development entry point.

Impact: a merge to main publishes the docs site and scaffolder with the pre-dev-branch message. The project rule "never claim anything that hasn't been demonstrated running" is violated in the other direction too: `README.md:258` lists the feature as Limited with "process recovery", which the integration test does demonstrate, but the docs site says the feature does not exist. One of the two must change before dev is retired; the task "retire the dev branch" implies main becomes the single truth.

### 2. issue — incorrect implementation vs DESIGN: a structural edit that fails to compile takes the healthy application down

DESIGN.md: "failed edits retain the healthy service until repair." README (`README.md:155-156`): "Compile errors … keep the last working application."

`internal/devserver/session.go:373-378`: on a `CompleteBuild` (rebuild) path the loop calls `closeHost(active)` and only then `makeHost(nextBuild.Inputs, snapshot.Dist)`, where `hotreload.BuildHost` (`internal/hotreload/host.go:82`, compile, then `cmd.Start` at :287) performs the full `go build`. `Classify` (`internal/hotreload/classify.go:37`) returns `CompleteBuild` from declaration-shape comparison before any compilation, so a syntactically valid edit that changes a declaration and does not type-check (e.g. add `func helper() int { return "x" }` to a route package, or rename an exported symbol used by another package) reaches line 377 with the healthy host still serving and stops it. `makeHost` then fails; the error path at :409-421 reports the diagnostic and `continue`s, and the closed host trips `case <-active.Done()` (:212-233), which rebuilds from `goodInputs`. Net effect: every request returns 502/503 for the duration of a failed full build plus a recovery full build, and all in-memory Go state is lost, on an ordinary typo.

The integration test's "bad Go edit retains the running application" case (`session_integration_test.go`, the `"package orders\nthis is invalid Go\n"` write) is a parse failure that `declarations()` rejects before `CompleteBuild`, so it never exercises this path; a type-error structural edit is untested. Reproduction: run `go test -tags skgo_dev_integration ./internal/devserver` with the bad-edit body replaced by `sessionRoute("7407", 9) + "func helper() int { return \"x\" }\n"` and assert `lastPID()` is unchanged after the diagnostic appears. Fix shape: build the replacement into its own socket path (or build first, then close and start) so the compile result is known before the healthy host is stopped; the "same private pathname" comment at :375 is the only reason given for the current order.

### 3. nitpick — `just e2e dev` is accepted with nothing behind it

`Justfile` keeps `e2e mode="prod"` and `example/e2e/playwright.config.ts` keys output dirs on `SKGO_E2E_MODE`, but the range deletes the `dev` recipe, the dev qualification matrix and all dev scenarios. `just e2e dev` therefore runs the production feature set against whatever is listening and labels the report `dev`. Given AGENTS.md's "skips are failures" rule, the mode argument should be removed or rejected until a dev browser suite exists (CONTINUE.md:17 says that pass is not established).

### 4. nitpick — CI toolchain pin drift

`.github/workflows/ci.yml:31` and `:104` pin `go-version: 1.27.2` while `go.mod:3` and `example/go.mod:3` declare `go 1.27.1`, and the other jobs use `go-version-file: go.mod`. The native-replacement loader (goloader adaptation for Go 1.27 in `internal/hotreload`) is the component most sensitive to a patch-level toolchain mismatch; one of the two should be authoritative. Carried over from round-02, still open.

### 5. nitpick — hard-coded `/tmp` in a package with a Windows build file

`internal/devserver/session.go:49` uses `os.MkdirTemp("/tmp", "skgo-")` (and `router_test.go:20`), while `process_windows.go` exists in the same package. The choice is deliberate (short Unix socket paths), but it should either be documented at the call site or the Windows file removed so the package does not imply a platform it cannot start on.

## Not findings, recorded for the merge decision

- Round-02's flaky `TestRetainedAtomicThenPreservedMtimeCompilerInputs` passed in this run after commits 335cbd7/868c5db; one green run is not proof of a fixed race, but no failure was reproduced.
- CONTINUE.md's open items (draft preservation unqualified, intermittent native-lifetime assertion unexplained, SSR UDS subfetch cookie relay) are honestly disclosed in README; they are limitations, not merge blockers, provided finding 1 is resolved so main does not say two things.

## Outcome

material findings remain
