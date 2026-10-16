# Adversarial review: dev server replacement plan (opus, round 01)

**Target:** `ephemeral/projects/dev-server/PLAN.md` at worktree HEAD `9216abd` (branch `codex/dev-server-replacement-plan`, based on `origin/dev` = `b2f2c00`).

**Authoritative request, as restated by the caller:** build a complete replacement Go dev server by copying specific functionality and mechanisms from Vite, gopls and the selected GoLoader. The deleted implementation must not be retained or used as a foundation. Replacement work integrates on `dev`. The plan must explicitly identify which upstream functionality is reused directly and which is copied.

**Operating constraints honoured:** the only file written is this artifact. I did not read earlier review artifacts. No commits, remote changes or memory writes. The caller's request set no limits on what the review could consider.

## Evidence inspected

- `CLAUDE.md`/`AGENTS.md` rules, plus the `agent-protocol` skill.
- `git`: `origin/dev` = `b2f2c00` (the deletion commit, which is an ancestor of HEAD), `origin/main` = `53179fc`, latest tag `v0.33.0`. The old dev file list on `53179fc` shows `internal/dev/proc_unix.go` and no cgo or arch restriction.
- `Justfile`: the `worktree` recipe hardcodes `main` at line 85. `e2e` is prod-only at lines 71–73. `test` is at line 60.
- `.github/workflows/`: `ci.yml` runs on every push/PR, with `ubuntu-latest` plus `macos-15` (Swift only). `qualification.yml` is `workflow_dispatch`/`workflow_call` and production-only. `release.yml` runs on pushes to main.
- `internal/releaseversion/releaseversion.go:47,79` treats `!` or a BREAKING footer as a major bump even at v0. The plan's v0.34.0 reasoning holds.
- `internal/kitpatch/kitpatch.go` and `internal/adapter/skgo-adapter/compat/kit-3.0.0-queue.json`: the correction ID and corrected SHA match the plan's pin claim.
- `cmd/skgo/templates.go:122–125`: the `--skgo-version`, `--sv-addon` and `--skgo-replace` flags exist. `generate.Prepare`/`Run` survive (`generate/generate.go:34,48`).
- `go.mod`: `go 1.27.1`. Local `go version`: 1.27.2 darwin/arm64.
- Installed Kit `3.0.0` (patched): `src/exports/vite/dev/index.js` lines 101–117, 296–459, 475–483, and `src/core/params.js:46`. The plan's citations are accurate. I also checked `types/index.d.ts` lines 46–68, where adapter `emulate` and `vite.getRequest`/`vite.setResponse` are declared.
- `internal/adapter/skgo-adapter.js:100–114`: the adapter already implements `emulate` and `vite.plugins`.
- Upstream sources: every path linked in the plan's mechanism table exists. I also read GoLoader `link/*.go` build tags; every series file is bounded `!go1.28`.
- Semantic index: `index/README.md` and `index/adoption.md` lines 30–54, which define **direct reuse / semantic port / already adopted / investigation lead**. Also `index/adoption-development.md`, D1–D15.

## Verified correct (not findings)

- The deletion is on `dev`. The worktree recipe and qualification are correctly described as unavailable or main-bound.
- The step 1 arithmetic is independent of the code under test: 14+41=55, 14+42+10=66, 21+43+10=74.
- The Kit middleware precedence description matches the installed source: the saved `viteServeStaticMiddleware` is used at 325–338 and again on a 404 at 451–455, and static middlewares are removed at 309/475.
- The release-version reasoning and the `skgo new` override names are correct.

## Findings

### 1. Issue — incomplete requirement: the plan never says, per mechanism, whether to reuse the upstream code directly, copy it, or only follow its pattern

**Missed instruction:** the user wants *explicit identification of upstream functionality to directly reuse or copy*.

**Evidence:**
- `PLAN.md:83–97` titles its table "Mechanism to adopt", but the rows use the verbs "Use", "Apply", "Extend", "Adopt the … pattern" and "Carry". The reuse mode is never stated.
- The semantic index the plan routes to already classifies each item as direct reuse, semantic port, already adopted or investigation lead (`index/adoption.md:34–38`, D1–D15/G1–G14/R1–R6). The plan dropped that classification.

**Concrete ambiguities that leave the reuse-versus-reinvent decision open:**
- **Watcher (row 85).** `gopls/internal/filewatcher` cannot be imported, so it must be copied as an attributed port. The plan states the "attributed Go ports" rule only on the parse row (88). It never names `github.com/fsnotify/fsnotify` as the directly reused library underneath.
- **Snapshot/metadata/packageHandle (rows 86–87).** The instruction "Extend existing shared Go analysis owners" does not say whether `Snapshot.clone`/`metadata.Graph` structures are copied or only imitated. It also does not say which surviving SKGO owner they extend.
- **Vite `transformRequest` and optimizer (row 89).** Is this a Go port of the timestamp/pending rules, or a pattern?
- **got-reload (row 90).** This is explicitly a "pattern", so is any of `rewriteFunc`/`rewritePkg` copied?
- **GoLoader (row 91).** It is implied to be a direct module dependency at `2695cfc`, but the plan never says how it reaches the private build view (module requirement, pseudo-version, private modfile).
- **`gocommand` (line 186).** It lives in `internal/` and therefore has to be copied. The plan calls it the "concrete command-ownership source".

**Index-mapped mechanisms the table omits entirely:**
- D9: Vite `moduleGraph` edge replacement and HMR boundary/cycle fallback.
- D11: per-file read stabilization.
- D12/D13: Vite `close`/`restart`/warmup lifecycle. The matrix row at line 219 needs a "Vite restart", but no source is routed for it.
- D2: gopls watch scope and directory expansion. Line 139 needs these for embeds and local replacements.
- R6: goja's inline source-map reader. Line 93 says only "reuse source maps".

**Impact:** each implementer has to rediscover whether to import, port or reinvent. That is exactly the decision the user asked the plan to settle. Line 227 ("the implementer performs its own upstream mapping") does not conflict with labelling the reuse mode. The mode is part of the mission, not a method.

### 2. Issue — incomplete requirement: the plan states the constraints for removing Kit's Node application handler but names no mechanism

**Missed instruction:** CLAUDE.md "We never reimplement kit … This is mirroring, not design", combined with the plan's own requirement at line 30 ("no reachable Node application handler" while preserving the tooling portions).

**Evidence:**
- In installed Kit, the tooling checks (`dev/index.js:320–366`) and application handling (`366–459`) are one closure, registered after `remove_static_middlewares` (309).
- Before any adapter request seam, that closure has already:
  - imported and executed `instrumentation.server` in Node (375–385);
  - imported Kit's server runtime through the SSR runner and run `configure(...)` and `await init()` (388–400);
  - served Kit's own `manifest_error` 500 page (408–425).
- Kit does provide adapter seams: `adapter.emulate` (296), `adapter.supports.instrumentation` (376), `adapter.vite.getRequest` (402) and `adapter.vite.setResponse` (455/459) (`types/index.d.ts:46–68`). The SKGO adapter already uses `emulate` and `vite.plugins` (`skgo-adapter.js:100–114`). SKGO also already ships an owned, SHA-verified Kit patch (`internal/kitpatch`, `compat/kit-3.0.0-queue.json`).

**What the plan omits:**
- None of those seams or the patch path is identified.
- Instrumentation and the `manifest_error` document, two Kit behaviors inside the removed closure, are not listed among what must be preserved or deliberately excluded (lines 30–32, 94, 162).

**Impact:** step 3 can only meet line 30 by one of the following:
- **Copying Kit's closure logic** into a spliced middleware or into Go: reimplementing Kit, which is forbidden.
- **Reaching `getRequest`/`setResponse` after Kit's server runtime has already been configured and initialized in Node.** This contradicts "no reachable Node application handler" unless that runtime module is substituted, for example through a Vite `load` hook.
- **Extending the owned Kit patch.** That changes the pinned correction identity the plan cites at line 81, and the patch must be distributed to fresh generated apps through `kitpatch`.

Each choice has different consequences for kit-pin upgrades and for fresh apps. This is the one Kit-side mechanism the plan has to identify, and it does not.

### 3. Issue — incomplete requirement: no product behavior is defined outside the two qualified GoLoader environments

**Missed instruction:** a *complete* replacement dev server. The plan itself says the deliverable must not "silently become restart-only" (line 127).

**Evidence:**
- The plan makes cgo mandatory and qualifies only darwin/arm64 and linux/amd64 (lines 53, 113, 129). It says to "reject … unsupported loader series/platforms before loading" (line 55).
- Every GoLoader `link/*.go` series file is bounded `!go1.28`.
- The scaffold records the running Go version (line 57). Any app created or run with the next Go release (expected around Feb 2027) will therefore have no buildable dev host.
- The old unix dev server had no cgo or arch restriction (`53179fc:internal/dev/proc_unix.go`).

**What is undefined:** what `skgo dev` does on:
- linux/arm64, which covers Docker on Apple silicon;
- darwin/amd64;
- a machine without a C toolchain (CGO disabled);
- Go 1.28.

There are three possible answers: refuse with a diagnostic, run a structural-rebuild-only host without the loader, or something else. The plan picks none, and the behavior matrix (lines 204–219) has no row for it. Line 113's "missing prerequisites fail rather than skip" also means root `just test` fails for any contributor on those environments, and the plan does not acknowledge that.

**Impact:**
- An unknown share of developers get an undefined dev server.
- The next Go minor release breaks every scaffolded app's dev loop.
- Promoting as `feat:` (line 225) without stating the new cgo/platform/toolchain requirements hides a regression in where the dev server runs.

### 4. Issue — incomplete requirement: no behavior is specified for runtime panics, stack positions or debugging through loaded and transformed code

**Missed instruction:** the plan's own step 5 scope ("error … behavior"), plus SKGO's existing contract that handler panics are recovered and reported (`handle_panic_test.go`, `load_panic_test.go`, `remote_panic_test.go`).

**Evidence:**
- Code replacement runs bodies from "Development-transformed Go source" in a private build view (line 61), loaded by GoLoader.
- The only location requirement in the plan is for compile diagnostics ("Diagnostic locations refer to the authored file/revision", line 140; matrix line 214).
- Nothing requires the following for a panic inside a replaced remote/load/action/endpoint body:
  - it is recovered through the loaded frames with Kit's existing 500/`handleError` behavior;
  - the host stays alive;
  - the reported stack cites the authored file and line rather than the private transformed file. This needs `//line` directives or equivalent, which got-reload's rewrite does not provide by itself.
- Loaded-code unwinding depends on GoLoader's funcdata/`deferreturn` registration being correct (`link/deferreturn.go`). This is exactly the kind of loader integration step 1 is meant to prove.

**Impact:** a panicking body is one of the most common dev-loop events. It could:
- crash the host;
- or escape SKGO's recovery;
- or point the developer at a private generated path.

Every one of these outcomes would pass the current acceptance lists. Add a step 1 proof (panic in a replaced body leaves a recovered response, the same PID, and an authored position) and a matrix row.

### 5. Nitpick — ownership of the replacement compile invocation is split between steps 1 and 2

Step 1 owns "Go compilation/loading" (line 111). Step 2 owns "compiler inputs" from "archives associated with the host build" (line 142), while "the GoLoader integration owner … retains the host's actual dependency artifacts" (line 135).

Steps 1 and 2 run in parallel (line 18). No owner is named for the importcfg/archive interface that the compile invocation needs. Two parallel owners may each build one, or neither may. A one-line ownership statement fixes this.

## Outcome

material findings remain
