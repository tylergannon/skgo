# Adversarial review: dev server plan, round 05

Date: 2026-10-14 (local). Reviewer: Claude Fable 5.1, read-only.

## Target

`ephemeral/projects/dev-server/PLAN.md` as committed at `d5b777d` on
`codex/dev-server-replacement-plan` (229 lines; the diff from `d8261bb` is
confined to the plan). The working tree is clean. The authoritative request is
unchanged: a complete, executable plan for rebuilding the dev server after the
deletion that issue #321 requested and PR #322 merged to `dev` at `b2f2c00`,
built from Vite, gopls, the selected `pkujhd/goloader` and the indexed upstream
implementations, with no implementation foundation or compatibility obligation
to the old server or to the failed branch at `360b874`. The conversation-only
requirement restated in earlier rounds still applies: the product is the dev
server and GoLoader code replacement is its mechanism, not a separately named
mode.

Caller constraints honored: read-only except this artifact; artifact path as
supplied; no edits to any other file, so no worklog entry or checkpoint commit
was made for this round. "The same authoritative request, repository
instructions, and source evidence" was read as the evidence set, not as a
limit on subject matter. No caller instruction predicted findings or requested
a verdict.

## Evidence inspected

- The plan in full; rounds 01 to 04; the planning worklog
  `ephemeral/worklog/20261014-dev-server-replacement-plan.md` and the deletion
  worklog.
- Issue #321 (open, no comments) and PR #322 (merged 2026-10-14T20:47Z, base
  `dev`). `origin/dev` is at `b2f2c00`; `origin/main` is at `53179fc`, tagged
  `v0.33.0`.
- Repository process and tooling: `CLAUDE.md`, the `agent-protocol` skill,
  `Justfile` (`test`, `e2e mode="prod"` refusing any other mode, `worktree`
  hardcoding `main` at line 85), `.github/workflows/ci.yml` (`on: push` and
  `pull_request` with no branch filter, so `dev` PRs get CI; `ubuntu-latest`
  root suite; `macos-15` Swift/client job), `qualification.yml` (`on:
  workflow_call` and `workflow_dispatch` only; asserts `x-skgo-mode: prod`),
  `release.yml` (`on: push: branches: [main]` and `workflow_dispatch`).
- `internal/releaseversion/releaseversion.go:47-87`: `!` or a `BREAKING
  CHANGE:`/`BREAKING-CHANGE:` footer maps to `Major`; `Major` produces
  `major+1.0.0` with no `v0` exception; `feat` produces `Minor`. The plan's
  `v0.34.0` statement at line 221 is correct against this code.
- `go.mod` and `example/go.mod` declare `go 1.27.1`; every workflow uses
  `go-version-file: go.mod`. Local Go is `go1.27.2 darwin/arm64`.
- `cmd/skgo/templates.go:122-125,189` and `internal/newapp/newapp.go:58-77,
  168-173,339,658-713`: the `--skgo-version`, `--sv-addon`, `--adapter` and
  `--skgo-replace` flags and the `SkgoVersion`, `SVAddonSpec`, `AdapterSpec`
  and `SkgoReplace` options named at plan line 166 exist; `file:` add-on specs
  are staged; `AdapterSpec` is used verbatim as the dev dependency.
  `internal/sv` and `internal/adapter` exist as the plan says.
- `refresh.go:67,102`: `Refresh` and `RefreshNoArg` exist.
- Installed Kit `3.0.0` with `patches/skgo-kit-3.0.0-queue.patch` applied
  through `example/web/pnpm-workspace.yaml:17-18`; the installed
  `src/core/postbuild/queue.js` SHA-256 is
  `400bf34544e2b3c5512e489eb5a0ca99a7483f0b6d733f70cab9e81e28f3a0f5`, matching
  line 77. `@voidzero-dev/vite-plus-core@1.0.0` bundles Vite `8.3.1` and
  exports `./module-runner`; the readable Vite checkout is `8.3.4` at
  `c2c9ef4`; x/tools is at `16517d9`; GoLoader is at `2695cfc`.
- Kit `exports/vite/dev/index.js`: `update_manifest` at `:101` with
  `load_and_validate_params` loading matcher modules through
  `loud_ssr_load_module` (`:110-116`), `full-reload`/`error` sends at
  `:117-130`, the post hook at `:301`, `remove_static_middlewares` at `:309`,
  the saved static responder at `:325-338` and `:451-455`, devtools at `:341`,
  base check at `:345`, service worker at `:349`, runner imports of env
  config, instrumentation and `server/index.js` at `:383-389`, and the adapter
  `vite.getRequest`/`vite.setResponse` seams at `:402,455,459`. `core/params.js`:
  `load_and_validate_params` begins at `:46`. Kit's client exports
  `refreshAll` (`runtime/client/client.js:2716`) and defines `_invalidate`
  (`:569`). The skgo adapter already uses `vite.plugins.pre/post` and
  `emulate` (`internal/adapter/skgo-adapter.js:100,114`).
- Every cache path and line anchor in the plan's mechanism table resolves to
  the named symbol or comment, including the executed preflight packet. The
  preflight notes record Go `1.27.2 darwin/arm64` with a private source GOROOT
  whose `VERSION` is `go1.27.2` and no installed Go sources changed.
- GoLoader: `mmap/mmap_darwin_arm64.go` and `libdl/dl_unix.go` both use cgo
  (`-lpthread` on darwin, `-ldl` on linux), confirming "both loader profiles
  require cgo". Its Go-version-specific files are tagged at minor-version
  granularity (`*.1.27.go`, `module.1.26.go`); no patch-level check exists.
- Go's `cmd/go/internal/work/gc.go:605-607` links with `-s -w` when
  `OmitDebug` is set, which `go test` does for test binaries, so the plan's
  "normally stripped `go test` executable" at line 109 is accurate.

## Disposition of round-04 findings

1. Promotion title cutting `v1.0.0` (issue): addressed at line 221 with a
   non-breaking subject, the computed `v0.34.0`, and a recalculation note.
2. Step 3 exit proved through step 4's listener (issue): addressed. Step 3's
   exit condition (line 158) is stated at the private Vite listener and step
   4 (line 168) owns the public-origin proofs.
3. Fresh-app qualification path (nitpick): addressed at line 166 with the
   actual flags and options, which this round verified.
4. darwin/arm64 CI lane after step 1 (nitpick): addressed at lines 53 and
   109.
5. Suppressed handler also removes matcher validation (nitpick): addressed at
   line 32 and in the new mechanism-table row.

The naming requirement is still met: no separately named mode, flag or lane
appears.

## Findings

### 1. Issue: the restored Ubuntu development qualification has no execution path before promotion

Line 182: "this step restores complete Ubuntu development qualification
through the integrated command, not a platform skip". Line 184: "CI describe
this one system." Line 227: "On `dev`, restore and run `just e2e prod` and
`just e2e dev` through the ordinary entry points before promoting the completed
replacement. After promotion, run the required main qualification through the
release process."

`qualification.yml` runs only through `workflow_call` from `release.yml` or a
manual `workflow_dispatch`, and `release.yml` triggers only on a push to
`main`. `ci.yml` runs on `dev` but is the root Go suite, not the browser lane.
So the first time the restored development lane executes on an Ubuntu runner
is the release gate on `main`, after the promotion squash has already landed.
Line 227's pre-promotion runs are local, which on this project means the Mac;
the plan applies the "not marked passed by a Mac-only run" rule to step 1
(line 109) but not to the step that owns the Ubuntu lane.

Impact: a development-lane failure that only shows on Linux (cgo loader path,
inotify watcher semantics, runner arch) is discovered on `main`, which then
sits unqualified and unreleased. That is the outcome the user chose the `dev`
integration branch to avoid, and the plan's own line 7 says released `main`
stays separate until the whole replacement is qualified.

Fix: in step 5's exit condition and in "Validation and finish", require one
green manual dispatch of `qualification.yml` against the `dev` ref (the
workflow already supports `workflow_dispatch`) covering both browser modes
before the promotion PR is opened, or give the qualification workflow a `dev`
trigger. Either is a one-sentence change; the plan need not pick the
mechanism, only state that the Ubuntu lane must have run on `dev` first.

### 2. Issue: the Go 1.27.2 patch-level pin stands in for an unstated invariant

Line 53: "Step 1's code replacement primitive must pass on Go 1.27.2 ...
Step 1 owns aligning CI's selected toolchain and the automated private
development preparation to Go 1.27.2: current `go-version-file` declarations
select 1.27.1 and are not sufficient. ... Other unsupported configurations
produce a startup diagnostic instead of silently losing code replacement."
Lines 109 and 125 repeat "stock Go 1.27.2" as the exit condition.

The evidence behind the number is that the preflight ran on the Mac's
installed `go1.27.2` with a private source GOROOT of the same version. The
loader's actual coupling is weaker and different: GoLoader selects runtime
layouts by minor version (`*.1.27.go`) and has no patch check, and the
requirement that matters is that the host binary, the replacement objects and
the private source view all come from one identical toolchain in a series the
loader supports. `1.27.1` is "not sufficient" only if the implementation
hard-codes `1.27.2`; a session that builds host and objects with CI's `1.27.1`
satisfies the invariant equally. Conversely, as written, the day Go `1.27.3`
ships and a developer's Mac updates, the plan's rule turns a supported
toolchain into "an unsupported configuration" with a startup diagnostic and
lost code replacement, and every Go patch release requires a skgo change.

This is the plan transmitting an interpretation as fact (line 223 forbids
exactly that), on the step it calls the first priority and risk. It also
leaves the alignment mechanism open in a way that matters: raising the
`go.mod` directive raises the minimum for every generated application, whereas
a `toolchain` line or the workflows' `go-version` does not.

Impact: step 1's owner either hard-codes a patch version that will be wrong
within weeks, or silently departs from the plan's stated exit condition.

Fix: state the invariant (one identical toolchain for host, objects and
private source view; the toolchain's minor series must be one GoLoader
supports at `2695cfc`), record `1.27.2` as the version the preflight and the
initial qualification used, and require the session to derive the version
from the toolchain it actually runs, with the diagnostic reserved for a
genuinely unsupported series or a host/object mismatch. Say whether CI is
aligned through the module directive or the workflow, since the first changes
the generated app's requirements.

### 3. Nitpick: step 2's accepted proof asserts "served" values, which step 2 does not own

Line 135: "Atomic editor saves, preserved-mtime writes, populated-directory
creation and rename/removal lead to fixture-defined generated/served values
without request-triggered discovery." Line 131 scopes step 2 to "Project/file
state and shared analysis/generation", and line 107 gives the host to step 1,
the session to step 4.

A served value needs a host answering HTTP. Step 2 can prove generated outputs
and compiler inputs on its own; it can prove a served value only through step
1's host, which the plan lets proceed in parallel. As written, the step-2
owner either waits on step 1 or builds a stand-in host.

Fix: "fixture-defined generated outputs and compiler inputs, and served values
once step 1's host is available" or simply drop "served" from step 2 and leave
it to step 4's save-to-response proof (line 166).

### 4. Nitpick: matcher validation imports application modules in Node, and the plan should say that is the accepted exception

Line 30: "no reachable Node application handler". Line 32: "Step 3 preserves
Kit's frontend matcher-module/export/name validation during tooling metadata
preparation ... without using Node for server request matching."

Kit's validation is `load_and_validate_params` with `load: (file) =>
loud_ssr_load_module(vite, vite_dev_server, runner, file)`
(`dev/index.js:110-116`): it imports `src/params/*` through the SSR module
runner, so the matcher modules execute in Node to check their exports. That is
consistent with the plan's intent (validation, not request matching), but a
reader of line 30 alone will take it as forbidden and either skip the
validation or port it. One clause stating that this import is tooling
validation and the only application code Node executes removes the ambiguity.
The `core/params.js:39` anchor in the table lands in the JSDoc; the function
starts at `:46`.

## Below the five-finding threshold

Noted, not counted: the mechanism-table links remain absolute paths on one
machine; the cache's `index/README.md` still names the canonical plan on
`codex/upstream-adoption-research`, which will be stale once this plan lands
on `dev`, and the plan could say the cache pointer is informational; line 101's
"managed worktree tool" with `ref:` is one session's tooling, and the manual
`git worktree add` beside it suffices; Kit 3's `AdapterViteConfig`
(`plugins.pre/post`, `getRequest`, `setResponse`, `emulate`) is the pinned
seam for inserting the step-3 guard ahead of Kit's post hook, and the adapter
already uses it, so no mapping gap exists there.

## Outcome

material findings remain
