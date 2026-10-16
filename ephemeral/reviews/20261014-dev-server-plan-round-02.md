# Adversarial review: replacement development-server plan, round 02

Date: 2026-10-14 14:45. Reviewer: Claude Fable 5.1, read-only.

## Target

`ephemeral/projects/dev-server/PLAN.md`, uncommitted working copy on
`codex/dev-server-replacement-plan` (209 lines; `git diff` against `3dcdaea`
shows the full rewrite that followed round 01). The authoritative request is
unchanged: a complete, executable plan for rebuilding Go development
functionality after issue #321 deletes the incumbent, with no implementation
foundation or compatibility obligation to the old server or the failed branch.

Caller constraints honored: read-only except this artifact; artifact path as
supplied. The caller said to review "against the same authoritative sources";
that was read as a pointer to the evidence set, not a limit on subject matter,
and nothing was excluded on its account.

## Evidence inspected

- The plan in full and its diff against the withdrawn outline at `3dcdaea`;
  round-01 review; worklog `ephemeral/worklog/20261014-dev-server-replacement-plan.md`
  (its last three decisions are the changes under review).
- Issue #321 body (open, no comments).
- `CLAUDE.md`: no cgo in the engine, mirroring not design, missions not
  methods, Go tests at the real handler, no silent skips, no "done" command.
- Repository: `Justfile` (`test` runs `go test ./... ./example/...` through
  `vp exec`), `.github/workflows/ci.yml` (`just test` on `ubuntu-latest`,
  Go from `go.mod`), `.github/workflows/qualification.yml` (`mode: [prod, dev]`
  on `ubuntu-latest`), `go.mod`/`go.work`/`example/go.mod` (`go 1.27.1`, no
  `toolchain` directive), `example/e2e/features/zz-source-update.feature`
  (the current dev-lane source-edit scenarios), `internal/adapter/skgo-adapter.js`
  (plugin registration at the pre-deletion tree).
- Installed pins: Kit `3.0.0`, `@voidzero-dev/vite-plus-core` `1.0.0`;
  `client.js:2716` (`refreshAll`), `client.js:569-653` (`_invalidate`);
  Kit dev plugin `exports/vite/dev/index.js:272-335` (asset middleware in the
  non-post hook, page/data/remote handler appended in the returned post hook
  after `remove_static_middlewares`).
- Semantic index: `index/README.md`, `adoption-development.md` (D5–D14 with
  the direct-runner caveat), `executed-preflight.md`, `publication-lifetime.md`,
  `native-engines.md`, `vite-kit-refresh.md`.
- Preflight packet `experiments/dev-preflight-20261014/`: `README.md`,
  `native-validation.md` (copy-removal control still passes; "general
  mapped-return copying remains unproven"), `skgo-boundary-prep.md`,
  `skgo-boundary-validation.md`, `kit-refresh/README.md`.
- Native POC `experiments/history-native/poc/README.md` (closed admitted lane:
  no globals, no pointer writes through host pointers, no escaping values).
- goloader `2695cfc`: `README.md` (`cp -r $GOROOT/src/cmd/internal
  $GOROOT/src/cmd/objfile`; "Don't use go run / go test"), `mmap/mmap_linux_amd64.go`
  (pure Go, `MAP_32BIT`), `mmap/mmap_darwin_arm64.go` (cgo, `MAP_JIT`),
  `libdl/dl_unix.go` (cgo on every unix including linux, `-ldl`),
  `link/registerInExe.1.27.go` (reads host ELF/Mach-O sections).
- Vite `8.3.4` at `c2c9ef4`: `module-runner/runner.ts`, `esmEvaluator.ts`
  (`AsyncFunction`, `import()` only for externals), `importMetaResolver.ts`
  (`process.getBuiltinModule('node:module')`), `utils.ts` (optional `Buffer`),
  `server/index.ts:1022-1110` (middleware order: configureServer hooks,
  transform/@fs/static, then post hooks, then 404), `server/hmr.ts:52-74`
  (`clientPort` on `server.ws`, deprecated on `server.hmr`).
- Go toolchain here: `go1.27.2 darwin/arm64`.

## Disposition of round-01 findings

1. CI/platform (critical) — partly addressed: the plan now requires linux/amd64
   and an unstripped fixture host. The sequencing it chose reintroduces the
   defect; see finding 1 below.
2. Dev/production build seam (issue) — addressed: step 1 owns the private build
   view, the CLI/SDK binaries do not link the loader, and line 110 is a
   load-bearing proof of a cgo-free production build after a dev session.
3. Browser-to-Vite topology (issue) — addressed by the topology paragraph at
   line 28, with one new concern; see finding 4.
4. Failure decision for the native primitive (issue) — addressed with a stop
   rule at line 112, which creates finding 3.
5. Prescribed Go port of the module runner (issue) — addressed: step 3 begins
   with a bounded direct-use check and records the blocker before porting.

## Findings

### 1. Critical — step 1 puts native fixture tests into `just test` on darwin only, while CI runs `just test` on Ubuntu

Line 49: "Begin integration on the recorded Go 1.27.2 darwin/arm64 setup.
Delivery also requires Go 1.27.2 linux/amd64 native qualification."
Line 100: "Root `just test` can therefore stay an ordinary Go test invocation
while covering real native execution through its fixture host", and "missing
compiler/cgo/package access is a failure, never a skip." Line 166 places
"Run the native generated-application tests on linux/amd64" in step 5.

`ci.yml` runs `just test` on `ubuntu-latest` for every push and PR, and the
repository merges only green PRs. So the step-1 PR, which by its own accepted
proof adds native tests to the root suite, either passes on Linux (meaning
Linux native support is part of step 1, not step 5) or is red on `main` until
step 5, or hides behind a platform constraint that the plan forbids as a
silent skip. The plan states all three positions and resolves none. The same
gap recurs at step 4: its browser scenario ("same application PID", line 152)
belongs in `example/e2e`, and `just e2e dev` runs on `ubuntu-latest` in
`qualification.yml`, two steps before the Linux lane is scheduled.

The Linux requirement is also understated. `mmap_linux_amd64.go` is pure Go,
but `libdl/dl_unix.go` is cgo on every unix including Linux and links `-ldl`,
so the "cgo is confined to development where the loader requires it" clause
applies to Ubuntu CI too, and the private `cmd/objfile` copy must be prepared
from the runner's `GOROOT/src` inside `just test`. None of that is impossible
on `ubuntu-latest`, but the plan names a darwin-first order that cannot land.

Two smaller facts in the same cluster: `go.mod`, `go.work` and
`example/go.mod` say `go 1.27.1` with no `toolchain` directive, so
`actions/setup-go` with `go-version-file: go.mod` installs 1.27.1, not the
"Go 1.27.2" the plan repeats; and goloader's own README forbids `go test`
output, which the plan handles for the host but has to handle for the
`cmd/objfile` preparation as well.

Fix: move linux/amd64 into step 1's accepted proof and exit condition (or
define step 1's root-suite coverage as darwin-only with a counted, visible
platform gate and say so), and pin the toolchain the plan actually validates.

### 2. Issue — helper replacement has no rule or proof for a helper with other callers in the host

Line 55: "Calls through helpers must execute the intended revision too;
replacing only the registered entry while silently calling an outdated helper
fails the capability." Line 59 admits edits "using ... a helper". Line 104's
proof changes the helper to `3*x` and checks the one registered caller.

A helper in an application package is ordinarily called by more than one
handler, and by host code that is not a handler at all. After the native
installer replaces handler A together with the edited helper, handler B and
every other host caller still execute the old helper text, and the Go compiler
may have inlined the old body into them so that even swapping B's dispatch
would not reach the new helper without recompiling B. The plan's only mention
of inlining is in the cross-package import row (line 191). With one caller in
the proof, an implementation that violates line 55 for every real application
passes step 1.

Fix: state the classification outcome for an edited function that has callers
outside the replaced set (replace all of them natively, or classify as a
rebuild), and add a proof case with two generated handlers sharing one helper
where both observe the new value or the edit is rebuilt.

### 3. Issue — native-backed data escaping into host state is not an admission category, so the no-unload rule and the bounded-revision rule cannot both hold

Line 59 admits bodies "using ... existing structs ... and initialized state"
and names callbacks, goroutines, deferred values and runtime types as the
constructs that are not admitted. Line 61: "Do not call `Unload` on code or
metadata that may remain reachable." Line 178: "The supported path cannot
accumulate unloaded-code hazards or unbounded native revisions."

A replacement body can store a string, slice or composite literal, or an
error value, into host state that outlives the request (a package-level map,
a cache, a logger). Those headers point into the revision's data mapping. The
POC refused this class outright (`poc/README.md`: no writes through host
pointers, no escaping values), and the preflight recorded that returned-value
copying is unproven (`native-validation.md`). The plan's line 107 covers a
held call and serialization, and line 63 handles execution that cannot
retire, but nothing classifies data escape before load. Without that outcome,
the implementer must either never unload (violating line 178) or unload and
risk a dangling pointer in host state on the next access (violating line 61
and crashing the dev host).

Fix: add escaping native-backed data to the admission list at line 59 with
its outcome (rebuild, or copy-on-boundary where the data crosses the generated
boundary), and add a proof that a body storing a literal into host state is
either classified or survives the old revision's retirement.

### 4. Issue — the public-origin forwarding rule is phrased as a manifest allowlist, which Vite's dev URL space cannot satisfy

Line 28: "Module paths come from the actual Kit/Vite manifest and resolved
asset space; there is no catch-all fallback from an application request into
Kit's Node middleware."

The second clause is right and now owned. The first prescribes a method that
does not match Vite. Dev module URLs include `/@id/…`, `/@fs/…`,
`/node_modules/.vite/deps/…` (optimizer output discovered at runtime),
`/@vite/client`, `/@vite/env`, `/__vite_ping`, query forms such as `?import`,
`?t=`, `?svelte&type=style&lang.css`, and source maps. No manifest enumerates
them. Vite's own precedence (`server/index.ts:1036-1081` then Kit's post hook
at `:1099`) is the specification here: module and asset middlewares claim what
they recognise, and the application handler is last. Mirrored into Go, the
rule is "Go answers what it routes (documents, `__data.json`, remote
endpoints, actions, `+server` routes, `/_app/immutable` in prod); everything
else is forwarded to Vite, whose private listener has had Kit's page handler
suppressed." Kit's dev plugin also installs a non-post asset middleware
(`dev/index.js:272`) that the guard must leave in place while removing the
page/data/remote handler appended at `:311`.

Fix: replace the allowlist sentence with Vite's precedence as the rule and
leave the mechanism to step 3/4; name the two Kit middlewares the guard treats
differently.

### 5. Issue — steps 2 and 3 are blocked on step 1 although neither depends on it and both are required under every outcome

Line 112: "Do not dispatch steps 2–6 before this prerequisite passes."
Line 16: "Steps 1–3 can be built and tested without advertising a usable
development server." Step 2 owns file state, analysis and generation inputs
and has no loader dependency; step 3 owns Vite/goja rendering and the private
Vite guard and has none either. If step 1 fails and the user chooses a changed
goal, every plausible changed goal (full-rebuild dev, or no native lane) still
needs steps 2 and 3 exactly as written. The worklog records the user's
decision as "a native integration blocker stops subsequent replacement work
for user adjudication"; the plan tightened that into a dispatch gate that puts
the longest and riskiest item alone on the critical path. The round-01 request
was for the contingent shape of steps 4–6, which the plan still does not
state; the gate is a substitute, not an answer.

Fix: gate steps 4–6 on step 1 (they do integrate the primitive), run steps
2 and 3 in parallel from the start, and state in one sentence what steps 4–6
deliver if the user resolves a step-1 blocker by changing the goal.

## Below the five-finding threshold

Noted but not counted: source routes remain absolute paths on one machine,
though #321 itself names that cache path; the semantic index README still
names the canonical plan on `codex/upstream-adoption-research`, not this
branch; the pins at line 69 still omit the `kitpatch` patch the Kit-refresh
evidence ran against; `server.hmr.clientPort` is deprecated in favour of
`server.ws.clientPort` at the readable Vite pin, so "supported server/WebSocket
configuration" should be read as the `server.ws` form; the direct-use check in
step 3 should expect `importMetaResolver.ts` to reach for
`process.getBuiltinModule`, which is one of the "Go-owned or disabled" seams
rather than a blocker; and "Port the relevant x/tools command/lifecycle
mechanisms" at line 166 names no source route, unlike every other adoption in
the table.

## Outcome

material findings remain
