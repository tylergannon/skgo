# Adversarial review: replacement development-server plan, round 01

Date: 2026-10-14. Reviewer: Claude Fable 5.1, read-only.

## Target

`ephemeral/projects/dev-server/PLAN.md` (uncommitted working copy on
`codex/dev-server-replacement-plan`, 197 lines, replacing the withdrawn
outline committed at `3dcdaea`). The authoritative request: a complete,
executable plan for rebuilding Go development functionality after issue
[#321](https://github.com/tylergannon/skgo/issues/321) deletes the incumbent
implementation. The old architecture and the failed branch
(`codex/dev-server-edit-loop` at `360b874`) cannot be retained.

Caller constraints honored: read-only except this artifact; artifact path as
supplied. No caller instruction narrowed the subject matter, so none was
ignored.

## Evidence inspected

- The plan in full, its two committed predecessors, and the worklog
  `ephemeral/worklog/20261014-dev-server-replacement-plan.md`.
- Issue #321 body (open, no comments).
- `CLAUDE.md` rules: no cgo in the engine, mirroring not design, missions not
  methods, Go tests at the real handler, no silent skips, no "done" command.
- Semantic index `/Users/tyler/src/skgo-token-cache/index/README.md`,
  `executed-preflight.md`, `publication-lifetime.md`, `vite-kit-refresh.md`.
- Preflight packet `experiments/dev-preflight-20261014/skgo/ephemeral/dev-preflight/`:
  `kit-refresh/README.md`, `skgo-boundary/boundary_test.go`,
  `skgo-boundary-prep.md`, `skgo-boundary-validation.md`, `native-validation.md`.
- Native POC `experiments/history-native/poc/README.md`, `reload.go`,
  `experiment-notes.md`.
- goloader `2695cfc`: `README.md` (requires `cp -r $GOROOT/src/cmd/internal
  $GOROOT/src/cmd/objfile`), `link/registerInExe.1.27.go` (reads the host's
  Mach-O/ELF/PE sections), `link/init.1.21.go:125` (`doInitialize`),
  `mmap/mmap_darwin_arm64.go` (the only file importing `"C"`).
- got-reload `gotreload.go:461` (`rewriteFunc`).
- Vite `8.3.4` at `c2c9ef4`: `hmr.ts:595-640` (per-environment `hotUpdate`
  dispatch), `transformRequest.ts:430-445` (stale result admission).
- Installed pins: `example/web/node_modules/vite` is
  `@voidzero-dev/vite-plus-core@1.0.0`; Kit `3.0.0`; `refreshAll` at
  `client.js:2716`, `_invalidate` at `client.js:569`. x/tools `16517d9`.
- Go toolchain: `go1.27.2 darwin/arm64`; `cmd/go/internal/test/test.go:1220`
  and `work/gc.go:606` (a `go test` binary links with `-s -w` unless `-c`).
- Repository: `Justfile`, `.github/workflows/ci.yml` (tests on
  `ubuntu-latest`), `.github/workflows/qualification.yml` (`mode: [prod, dev]`
  on `ubuntu-latest`), `internal/kitpatch`, `internal/nativebuild`,
  `example/cmd/main.go`, `example/internal/skgo` (committed `go:generate`
  output).

## Findings

### 1. Critical — the plan's acceptance is unsatisfiable on the CI it says must qualify it

The plan commits to three things at once:

- Line 47: the first supported configuration is Go 1.27.2 darwin/arm64 with
  cgo confined to the loader; unsupported configurations get a startup
  diagnostic; other platforms need execution on that platform.
- Line 156: "Temporary absence of qualification from #321 is replaced by real
  development qualification, not skipped checks."
- Line 195: "Run `just test` with no silent skips. Restore and run `just e2e
  prod` and `just e2e dev` ... through the ordinary entry points."

Both `just test` (`ci.yml:21`) and the `dev` qualification lane
(`qualification.yml:16-21`) run on `ubuntu-latest`. The plan's own cited
evidence establishes that native loading requires a cgo darwin/arm64 build
(`mmap/mmap_darwin_arm64.go` is the one `import "C"`), a private GOROOT source
clone with `cmd/objfile` (goloader README; `experiment-notes.md` "Exact paths
and environment"), and a host binary whose symbol table is not stripped. On
that last point the plan's step-1 proof shape is also wrong as written: step 1
demands "Go tests at the composed handler" (line 94) and an exit condition of
"work with stock compiler binaries" (line 104), but a plain `go test` binary is
linked with `-s -w` (`test.go:1220`, `gc.go:606`), which is exactly why every
preflight command in the packet passes `-ldflags=-s=false` and why the POC
README warns "plain `go test` ... cannot exercise native loading". `just test`
does not pass that flag.

So either the step-1 tests build and launch a separate unstripped host (the
POC's `TestWatchedFileHost` shape) and the plan should say so, or `just test`
changes link flags for every package. And on Linux CI the native tests cannot
run at all under the plan's platform statement, which means the only outcomes
available are a skip (forbidden by line 195 and by CLAUDE.md) or a failing
`main`. The plan must decide one of: Linux native support is in scope for the
first delivery (goloader ships `mmap_linux_amd64.go` but no preflight ran
there); the dev lane and native tests move to a macOS runner; or the Linux dev
lane is defined as the full-rebuild path with the native tests explicitly
platform-gated and counted, not skipped. None of these is stated, and the
"complete executable plan" has no executable answer for its own CI.

Impact: the first implementer to reach step 1 cannot satisfy "Go tests at the
composed handler" under `just test`, and the first integration PR cannot be
green on `main` without violating a rule the plan restates.

### 2. Issue — dev-only generated dispatch versus the production cgo-free build is an unowned seam

Line 49: dev generation emits got-reload-style named-function dispatch while
"normal production generation keeps its direct Go calls." Line 28: the loader
and its platform requirements "do not enter a production build." Line 140: no
`--native`, no opt-in annotation, no manual registration.

Generated output lives in the application tree and is committed
(`example/internal/skgo/skgo_gen.go` under a `go:generate` directive). The plan
never says where the dev variant of that output lives, what selects it, or how
`go build ./cmd` of the same tree after a `skgo dev` session is still free of
goloader and cgo. Issue #321 lists "production can build without the
development loader/cgo dependency" as an accepted result and the plan's
"Validation and finish" repeats it, so this is a requirement the plan must
assign. It falls between step 1 ("development-only generated dispatch", line
92) and step 4 ("command/template/recipe wiring", line 138) and neither owner
is told to produce the dev host binary or keep the production one clean. The
existing repository already has a precedent for this kind of split
(`internal/templates/open.go` is `//go:build cgo && (darwin || linux)`), which
makes the omission conspicuous rather than excusable.

Impact: two owners can both reasonably believe the other owns the build-mode
split; the likely result is a committed generated tree that flips between
modes or a production binary that links the loader.

### 3. Issue — the browser's path to Vite is unspecified

The plan deletes the proxy and WebSocket routing with #321 and states (line
24) that the session owns the public socket, Vite owns compilation and HMR,
and (line 76) that Kit is refreshed through "Vite custom HMR delivery". It
never says how the browser reaches Vite's served modules (`/@vite/client`,
`/src/...`, `/@fs/...`), the HMR WebSocket, or how the Go session's pages
displace Kit's own dev middleware, which under `vp dev` serves every page with
Node SSR and answers `__data.json` and remote endpoints itself. Kit 3 fixes
`paths.origin` at build time (`ephemeral/sveltekit-current/SKILL.md`), so a
two-origin arrangement is not a free default either.

This is precisely the kind of composition question the withdrawal note on the
previous draft said the plan must settle before an execution sequence exists.
The ownership rows for step 3 (line 126) and step 4 (line 138) do not contain
it, and the index leaf `vite-kit-refresh.md` has nothing on transport topology
to point an implementer at.

Impact: step 4 cannot be handed off as "connect ordinary `skgo dev`" when the
single most visible wiring question of that step has no owner or decision.

### 4. Issue — the step-1 primitive is unproven and the plan has no decision rule for its failure

Everything the product claims about preserved state (lines 55, 140, 144, 175)
rests on a compiled replacement body in package X reaching X's unexported
helpers and initialized globals through goloader's symbol map without loading
a second copy of X. The evidence does not cover that. `boundary_test.go` and
the POC compile a separate package (`hotpreflight`, `hotcode`) with fixed
imports and receive host state as a pointer argument; `skgo-boundary-prep.md`
lists "Fn/Refresh identity, loads, SSR retention ... full dependency closure"
as unproved; `native-validation.md` shows the admitted lane rejects globals,
init, new types, goroutines and callbacks. The Go compiler emits whole-package
objects, so a "body-only" object contains all of X's text and data;
`doInitialize` (`init.1.21.go:125`) runs the init of every imported package it
finds in the symbol map. The plan acknowledges the tension (lines 51-53:
"blind whole-package loading is not justified", "its symbol map distinguishes
host data and locally loaded text") and then delegates it with "report the
exact failure and proposed smallest adaptation" (line 102).

That is the right first task, but an executable plan needs what happens when
the report says no. Step 4's exit condition (line 144, "performs no full
application link/restart") and the matrix row at line 175 become
unsatisfiable, and the plan does not say whether the deliverable then becomes
the full-rebuild path for all Go edits (which steps 2, 3 and 5 already
require) or whether the whole program stops. The issue mandates goloader, so
this is not a request to reopen engine selection; it is a request for the
plan to state the contingent shape of steps 4 to 6 so the remaining owners
are not blocked on one spike.

Impact: three parallel owners (steps 1 to 3) are dispatched, and two of them
have no defined product to integrate into if the first fails.

### 5. Issue — step 3 prescribes a Go port of Vite's module runner without establishing that direct use is infeasible

Line 75 and line 126 fix the design: a "focused Go semantic port" of
`EvaluatedModules`, `cachedRequest`, the HMR handler, the payload queue and
the transport lifecycle. The index's stated direction is "use direct
functionality where possible and semantic Go ports where Go owns the work",
and CLAUDE.md says a plan delegates missions, not methods. Vite's
`module-runner` package is dependency-light JavaScript that evaluates
SSR-transformed code with async functions rather than native ES modules, which
is the shape goja already executes for the production renderer. Running Vite's
own runner inside the pooled goja runtime with a Go-owned transport would
mirror Vite instead of reimplementing it, keep the update-ordering, alias,
cycle and invalidate semantics in Vite's code at Vite's pin, and shrink step 3
to transport plus Go I/O bridges. The plan does not record a reason this was
rejected (goja ESM gaps, a Node builtin, a performance observation). Without
that, the port is prescribed work whose necessity has not been shown, and it
is the largest single body of new code in the plan.

Impact: if direct use works, step 3 carries a maintained Go copy of Vite
internals that drifts on every Vite bump; if it does not, the plan should say
why so the owner does not spend the first week rediscovering it.

## Below the five-finding threshold

Noted but not counted: the plan's source routes are absolute paths on one
machine (`/Users/tyler/src/skgo-token-cache/...`), unusable from a worktree
elsewhere or from CI; the semantic index README names the canonical plan on
branch `codex/upstream-adoption-research`, not this branch; the plan's pins
omit the `kitpatch` patch (`skgo-kit-3.0.0-queue.patch`) that the Kit-refresh
evidence ran against; and the matrix row for query-boundary repair (line 183)
should name what pinned `refreshAll` does for an errored query rather than
"Kit-supported recovery", since the preflight showed the refresh resolving
while the boundary stayed failed.

## Outcome

material findings remain
