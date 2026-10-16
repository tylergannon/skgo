# Adversarial review: replacement development-server plan, round 03

Date: 2026-10-14 (local). Reviewer: Claude Fable 5.1, read-only.

## Target

`ephemeral/projects/dev-server/PLAN.md` as committed at `9b8534e` on
`codex/dev-server-replacement-plan` (219 lines), rebased onto `dev` at
`b2f2c00483139b821072a94d22875674909e9eaf`, where PR #322 squash-merged the
deletion that issue #321 requested. The authoritative request is unchanged: a
complete, executable plan for rebuilding Go development functionality after the
deletion, from Vite, gopls, the selected `pkujhd/goloader` and the indexed
upstream implementations, with no implementation foundation or compatibility
obligation to the old server or to the failed branch at `360b874`.

Caller constraints honored: read-only except this artifact; artifact path as
supplied. No caller instruction narrowed subject matter or predicted a verdict.
"Linked source and evidence" was read as the evidence set, not a limit.

## Evidence inspected

- The plan in full; rounds 01 and 02; the worklog
  `ephemeral/worklog/20261014-dev-server-replacement-plan.md` (its last three
  entries are the changes under review).
- Issue #321 (open, no comments) and PR #322 (merged 2026-10-14T20:47Z, base
  `dev`, merge commit `b2f2c00`). `b2f2c00` is an ancestor of `origin/dev`
  and of this worktree's HEAD, and is not on `origin/main` (`53179fc`).
- Deleted tree: `dev/`, `internal/dev/`, `internal/vite/` absent; no
  `NewDevSSR`/`NewDevProxy`/`gojaDevEnvironment`/`__skgo_dev` references
  outside `ephemeral/`; `internal/gen/worker*.go` removed and
  `generate.Prepare`/`Run` retained (`generate/generate.go:34,48`);
  `Refresh`/`RefreshNoArg` present (`refresh.go:67,102`). README, AGENTS.md
  and SVELTEKIT_COMPATIBILITY.md state development mode is unavailable.
- Repository process: `CLAUDE.md`/`AGENTS.md` (identical head), the
  `agent-protocol` skill, `Justfile` (`test`, `e2e mode="prod"` which exits 1
  for any other mode, `worktree` which branches from `main` at line 85),
  `.github/workflows/ci.yml` (`just test` on `ubuntu-latest`, Go from
  `go.mod`, Conventional Commit check on HEAD), `qualification.yml` (prod
  only, asserts `X-Skgo-Mode: prod`), `release.yml` (push to `main` only).
  `go.mod`, `go.work`, `example/go.mod`: `go 1.27.1`, no `toolchain`
  directive. Local toolchain `go1.27.2 darwin/arm64`.
- Browser suite after deletion: `example/e2e/playwright.config.ts` has only
  `chromium`/`noscript` projects matching every feature; `steps/app.ts`
  asserts the production mode header; no mode selector remains.
- Installed pins: Kit `3.0.0`; `vite` resolves to
  `@voidzero-dev/vite-plus-core@1.0.0` whose bundled Vite reports `8.3.1`;
  the installed `src/core/postbuild/queue.js` hashes to
  `400bf34544e2b3c5512e489eb5a0ca99a7483f0b6d733f70cab9e81e28f3a0f5`, the
  `correctedSHA256` of correction `skgo-kit-3.0.0-prerender-queue-v1`
  (`internal/adapter/skgo-adapter/compat/kit-3.0.0-queue.json`). Kit dev
  plugin `exports/vite/dev/index.js`: asset middleware at `:272`,
  `remove_static_middlewares` at `:307`, page/data/remote handler at `:311`,
  `full-reload` sends at `:119,:125,:248`.
- Research cache `/Users/tyler/src/skgo-token-cache`: every path and line the
  plan's mechanism table cites resolves to the named symbol (gopls
  `fsnotify_watcher.go:26`, `view.go:785`, `snapshot.go:1504,2032`,
  `graph.go:20`, `check.go:817`, `parse_cache.go:149`, `future.go:12`,
  `invoke.go:31`; Vite `transformRequest.ts:78,438`, `environment.ts:40`,
  `runner.ts:39`, `types.ts:49`, `evaluatedModules.ts:11`,
  `hmrHandler.ts:7`; got-reload `gotreload.go:461`; goloader
  `readobj.go:69`, `ld.go:561`, `init.1.21.go:125`; POC `reload.go:223`;
  boundary fixture `boundary_test.go:29`). Repo revisions match the plan:
  Vite `c2c9ef4` at package version `8.3.4`, x/tools `16517d9`, goloader
  `2695cfc`. goloader `libdl/dl_unix.go` is cgo on linux with `-ldl` and is
  imported by `link/addcgosymbol.go`; `mmap_linux_amd64.go` is pure Go.
  `index/README.md`, `adoption-development.md`, `executed-preflight.md`,
  `native-validation.md` (Go 1.27.2 darwin/arm64, `-ldflags=-s=false`,
  private GOROOT).
- Preserved items: worktree `/Users/tyler/.codex/worktrees/73b6/skgo` at
  `360b874` on `codex/dev-server-edit-loop`; the research cache present.

## Disposition of round-02 findings

1. Linux native in step 1 (critical): addressed. Lines 51, 106 and 122 put
   linux/amd64 and the cgo/libdl prerequisite inside step 1's accepted proof
   and exit condition, name the `go.mod` 1.27.1 mismatch, and assign
   toolchain alignment to step 1.
2. Helper with other callers (issue): addressed at lines 59 and 111 (complete
   executable caller closure including inlining; two-handler proof; an
   out-of-boundary caller classifies for rebuild).
3. Escaping native-backed data (issue): addressed at lines 63 and 117 (data
   escape is an admission category; candidate-backed data must be tested).
4. Manifest allowlist (issue): addressed at line 30 (Vite precedence is the
   rule; `:272` kept, `:311` suppressed). One omission remains; finding 4.
5. Steps 2 and 3 gated on step 1 (issue): addressed at lines 18 and 120
   (parallel ownership; only 4 to 6 depend on step 1). The contingent shape
   is answered by the user's recorded decision that no weaker product is
   pre-authorized, which the plan states at line 120.

Round-01's five findings remain addressed as recorded in round 02.

## Findings

### 1. Issue: the repository's only worktree recipe and the protocol's root-checkout rule both start from `main`, which still contains the deleted server

Line 98: "Replacement implementation starts from current `dev` in its own
worktree and targets `dev`." Lines 7 and 96 repeat that `dev` is the base.

The mechanism a dispatched implementer will actually reach for says
otherwise. `Justfile:85` is `git worktree add -b "{{branch}}" "$dest" main`,
with a comment that the branch is confirmed "rather than assumed" because an
agent once landed on `main` by accident. The `agent-protocol` skill says to
fast-forward the root checkout and branch from it, and the root checkout
(`/Users/tyler/src/skgo`) is on `main` at `53179fc`, the pre-deletion tree
with `internal/dev`, `internal/vite`, `cmd/skgo/dev.go` and `generate.Worker`
all present. `gh pr create` defaults its base to `main`. Nothing in the plan
tells an owner to pass `dev` to `git worktree add`, to open PRs with
`--base dev`, or assigns a one-line fix to the recipe. The first owner who
follows the repository's own tooling will build on the old server that #321
forbids building on, and the first PR opened without `--base dev` will
propose merging step-1 work into released `main` ahead of the deletion.

Fix: in step 0, state the worktree and PR-base gesture for the `dev` period
(`git worktree add <dir> -b <branch> origin/dev`; `gh pr create --base dev`),
and either assign the `Justfile` recipe's base to the first replacement PR or
say that the recipe is not to be used while `dev` is the integration branch.

### 2. Issue: step 4's browser proof has no lane to run in, and no step owns restoring the `e2e dev` entry point

Line 162 (step 4 accepted proof): "A browser scenario edits a meaningful
compatible body and sees its new value with the same application PID,
document identity, local counter, shallow state and unsent input."
Line 217: "On `dev`, restore and run `just e2e prod` and `just e2e dev`
through the ordinary entry points before promoting."

After #322, `just e2e` exits 1 for any mode other than `prod`
(`Justfile`), `playwright.config.ts` has two projects that together match
every feature file, `steps/app.ts` fails any scenario whose server does not
answer `X-Skgo-Mode: prod`, and the mode selectors were deleted by design.
So a step-4 scenario added under `example/e2e/features` is either collected
by the production lane and fails there, or has no lane at all. Step 4's
ownership (line 160: "command/template/recipe wiring, application
control/readiness and narrow adapter-to-Kit readiness delivery") does not
name the browser-suite lane, and step 5's exit condition (line 178:
"Restored CLI help, examples, generated docs/recipes and CI") is the first
place qualification is restored, one step after the scenario is required.

Fix: assign the `just e2e dev` recipe, the Playwright lane or project that
selects development-only scenarios, and the server-mode assertion to step 4,
since step 4's own accepted proof cannot run without them; leave the CI
matrix lane to step 5.

### 3. Issue: the plan does not say how `dev` tracks `main` or what gesture promotes it

Lines 7 and 217 define `dev` as a temporary integration branch and say the
finished replacement is promoted to `main` after validation, then qualified
"through the release process".

Two repository facts make the gesture non-obvious and the plan silent on
both. First, `ci.yml` runs `go run ./cmd/skgo-release-version --check HEAD`
on every push and PR, and `release.yml` derives the release version from the
Conventional Commit subject of what lands on `main`; a merge commit from
`dev` fails the check (the repository's own rule is to rebase PR branches,
never merge `main` in), and a squash of the whole replacement is one
`feat!:`/`refactor!:` commit whose subject the plan has not named. Second,
`main` keeps moving during the replacement (the user runs parallel sessions;
#318 through #320 landed in the days before #322), and `dev` must absorb
those changes somehow; a merge of `main` into `dev` creates the same
rejected merge commits on `dev`, and a rebase of a shared integration branch
rewrites every open replacement worktree. The plan assigns neither policy.

Fix: one short paragraph under "Validation and finish" stating (a) how `dev`
takes `main`'s advances during the work (for example, periodic rebase with
every replacement worktree rebased behind it, or merge commits tolerated on
`dev` only because the check gates PR heads, whichever the user chooses) and
(b) the promotion gesture (a single squash PR `dev` into `main` with a
stated Conventional Commit subject), so the release that follows is the one
the plan intends.

### 4. Issue: the private-listener guard preserves Kit's asset middleware and suppresses its page handler, but says nothing about Kit's removal of Vite's static middleware, which sits between them

Line 30: "Its private listener keeps Kit's non-post asset middleware
(`dev/index.js:272`) but suppresses Kit's post-installed page/data/remote
handler (`:311`), so forwarding can never fall through to Node application
execution." Line 30 also makes "remaining tool/module/asset requests" forward
to Vite under Vite's own precedence.

Kit's returned post hook does two things before installing the handler at
`:311`: it finds `viteServeStaticMiddleware` and calls
`remove_static_middlewares` (`:303-307`), because Vite's static middleware
otherwise serves project files at URLs like `/package.json`, `/static` and
`/src/...` (Kit's comment cites vitejs/vite#7363). A guard that suppresses
Kit's post hook as a unit removes that call too. Under the plan's forwarding
rule, a browser request for `/package.json` or `/svelte.config.js` on the
public origin is not a document, data request, remote, action or endpoint,
so Go forwards it, and the private Vite listener serves the file from the
project root. The behavior matrix has no row for a project-file URL and the
step-3 exit condition ("Direct requests to the private Vite listener cannot
invoke Node application handlers") does not cover file serving. This is a
mirroring gap: Kit's dev server does not serve those files, so Go's must not
either.

Fix: name `remove_static_middlewares` (`:307`) as the second thing the guard
preserves, and add one Go test at the public handler that requests a
project-root file such as `/package.json` and receives the application 404
document rather than the file.

### 5. Nitpick: step 1 puts host preparation into `just test` without a budget, against the repository's seconds-tier rule

Line 106: root `just test` "exercises native code through the real host on
Ubuntu", with the test driver building and launching an unstripped host under
a prepared private Go 1.27.2 source view, cgo and goloader, and "Preparation
is automated by the tested build primitive". `CLAUDE.md` defines one of the
three evidence legs as "`just test` green in seconds", and the preflight
records that the private source view is a copied GOROOT whose `pkg`/`lib`
links depend on the installed toolchain. The plan does not say that the
private view and the host binary are prepared once per package and reused
across assertions, nor that step 6's measurements include the suite's wall
time. Without that sentence the first implementation will prepare the view
per test, and the root suite stops being seconds-tier on every push.

Fix: add to step 1 that preparation and the host build happen once per test
binary and are cached across runs where the toolchain identity is unchanged,
and that the root suite's wall time is reported alongside step 6's latency
observations.

## Below the five-finding threshold

Noted, not counted: the plan's source routes are still absolute paths on
one machine, although #321 names that cache path itself; the cache's
`index/README.md` still names the canonical plan on
`codex/upstream-adoption-research` and the baseline as `53179fc`, which the
plan supersedes at lines 71 and 73 without being able to edit the cache; the
word "native" now means goloader-backed Go code in this plan and the Swift
client in `native/`, `internal/nativebuild` and the `native-swift` CI job,
so handoffs should say "native loader" or "goloader" where confusion is
possible; `server.ws.clientPort` is the non-deprecated form of the HMR
advertisement at the installed pin and the plan's "configuration supported by
the actual installed pin" should be read that way; and the matrix row for
route changes should note that Kit's own dev plugin sends `full-reload` on
route-file add/remove (`dev/index.js:119,125,248`), so "Kit navigation
actually reaches the new route" is a reload-then-navigate claim, not a
state-preserving one.

## Outcome

material findings remain
