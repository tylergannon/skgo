# Adversarial review: dev → main readiness repairs (round 02)

Date: 2026-10-15 (local). Reviewer: Claude Fable 5.1, read-only except this file.

## Target

Repairs since `3cb90f033e1698289dbe7cc5f93eec790697a55d` on `codex/dev-main-readiness`,
the candidate for PR #329 (base `dev`). The branch advanced while this review ran:
the launch prompt described an uncommitted working tree, and during the review
that tree was committed as `0d2d512` (docs: qualify recovery after native loader
failures) and `78c04cd` (fix(dev): preserve healthy host during failed rebuilds),
with a rewritten PR body, and then `5b43304` (docs: distinguish historical dev
proof from current repairs) updated CONTINUE.md. This artifact is against HEAD
`5b43304`; the working tree is clean apart from untracked `ephemeral/` logs.
Commits in range: `68b8c50`, `3f5f526`, `44c55ac`, `0d2d512`, `78c04cd`, `5b43304`.

Authority: AGENTS.md/CLAUDE.md, `ephemeral/projects/dev-server/{GOAL,DESIGN,
CONTINUE}.md`, the round‑01 review `ephemeral/reviews/20261015-dev-main-round-01.md`,
README and the on‑disk proof.

Scope note: the launch prompt narrowed nothing about defects, files or verdict.
"Avoid duplicating expensive in‑flight commands" is an operating constraint and
was honored: the `skgo_dev_integration` session test was not run here; its on‑disk
logs were read instead. Nothing was ignored.

## Evidence inspected

- Full diff `3cb90f0..78c04cd`, every file, plus the pre‑commit working tree.
- Read in full or in the relevant region: `internal/devserver/session.go`
  (host lifecycle, `publishHost`, recovery and rebuild paths), `router.go`,
  `frontend.go`; `internal/hotreload/host.go` (BuildHost readiness, `Prepare`,
  `Install`, `exchange`, `receive`), `compile.go`, `classify.go`, `toolchain.go`
  (the exact Go 1.27.2 / two‑profile gate); `internal/gen/retained.go` and
  `scan.go` (type‑loading in `Prepare`); `internal/kitpatch/kitpatch.go`
  (`runPnpm`, `vitePlusExecutable`) and its test; `cmd/skgo/kit_patch_test.go`;
  `example/e2e/global-setup.ts`; `.github/workflows/ci.yml`, `qualification.yml`;
  `Justfile`; newapp templates; README, `SVELTEKIT_COMPATIBILITY.md`,
  `docs/index.html`, `docs/features.html`, `skills/skgo/SKILL.md`; AGENTS.md
  (CLAUDE.md is a symlink to it); the three worklogs in range; PR #329 body.
- Proof on disk: `ephemeral/ci-repair-{build,test,focused,remote,final-remote}.log`,
  `ephemeral/structural-recovery-{red,red-clean,red-final,red-constructor,green}.log`,
  `structural-recovery-overlay.json`, `structural-recovery-before.txt`.
- Ran, cheap and read‑only: `go vet ./internal/devserver ./internal/kitpatch
  ./cmd/skgo` (exit 0), `go build ./...` (exit 0), `go test -count=1
  ./internal/devserver` untagged (ok).

What the logs establish (mtimes vs. the committed test, 20:22:15):

- `red.log` (20:16) started while `session.go` was being edited; its own output
  shows "application dependency … session.go" rebuilds. Contaminated, as the
  worklog says. Disregarded.
- `red-clean.log` (20:18, overlay = pre‑fix `session.go`, fixture with
  `sync.Mutex`) failed at the *first* compatible body edit (`:233 want ALPHA
  literal-5202:8`) after `symName:sync.(*Mutex).Unlock relocateType:R_ADDROFF,
  symbol is not exist!`, with no further session output for ~75 s. It proves
  nothing about the lifecycle repair; it is the evidence for finding 1.
- `red-final.log` (20:20) ran an earlier test revision whose structural edit was
  in the route package; gen's `Prepare` type‑checks routes (`scan.go:257`) and
  raised `skgo: revision 10: … cannot use "type-invalid"` before the hot‑reload
  path, so `"rebuilding revision"` never printed and the test failed at its own
  assertion. The committed test moved the edit to the application constructor
  package (`session_integration_test.go:325-333`), which gen does not import.
- `red-constructor.log` (20:22, overlay = pre‑fix `session.go`, committed
  fixture) fails at `:346 failed structural edit lost healthy application:
  status=500`. This is the load‑bearing negative: the old close‑before‑build
  order loses the healthy host on exactly the edit the test now makes.
- `green.log` (20:24, committed code and fixture) passes in 30.6 s with no skips:
  `rebuilding revision 10` then `cannot use "type-invalid"` while every sampled
  request kept `literal-8508:73` and the healthy PID; the repair published
  revision 11 on a new PID; the later Vite restart, configuration, dotenv and
  SIGKILL recovery assertions passed.

Round‑01 finding 2 (close before build) is therefore repaired and proven by a
test that fails when the repair is reverted. Round‑01 finding 1 (contradictory
"development unavailable" text) is resolved on every surface it listed and the
others that carried the same text (README, compatibility journal, SKILL.md,
`README.md.tmpl`). The PR body now states exactly Go 1.27.2 and discloses the
loader limitation; an earlier draft said "1.27.2+", which is gone.

## Findings

### 1. issue — a native‑loader install failure is silent, blocks the session, and the new docs overstate what the developer sees

DESIGN.md: "Unsupported edits rebuild. Successful installation publishes the
refresh; failed edits retain the healthy service until repair." GOAL.md: "Show
useful errors and recover when fixed."

`red-clean.log:22` is the only line the session emitted for a compatible body
edit whose native install hit the relocation failure: the loader's raw stderr
message. No `skgo: revision N:` diagnostic, no `rebuilding revision`, no publish,
no Vite error notification (`front.notify` is only reached on an error return,
`session.go:430-434`). `receive` defaults to a 30‑second read deadline
(`host.go:355-361`), so a host that never replied would have produced an i/o
timeout diagnostic within 30 s; none appeared in 75 s, which means the stall is
somewhere without a deadline (the compile‑time loader call in `Prepare`,
`host.go:492` via `p.retained.Load`, is the candidate). Because `Prepare` and
`Install` run synchronously inside the single event loop, a stalled install also
stops the loop from seeing further file changes, Vite restarts or a Go crash.

The commit `0d2d512` discloses this, which is right, but its wording claims
more than the evidence: `docs/index.html` and SKILL.md say native‑loader errors
"report a diagnostic and keep serving old code". The one recorded instance
reported nothing through skgo's diagnostic path; a developer in the browser
sees an edit that silently does not apply. The worklog's "applying such an edit
therefore requires a subsequent source change or restart" is also unverified
if the loop is blocked: only a restart is known to work. Say that, and track the
defect as its own issue (loader failure must either surface through
`nextBuild.Diagnose`/`front.notify` with a deadline, or fall back to the rebuild
path). Not a regression of this PR; a material GOAL gap that the PR now ships
documented but untracked.

### 2. nitpick — do not commit the recovery evidence files

Untracked under `ephemeral/`: `structural-recovery-red.log` (141 KB), `red-clean`,
`red-final`, `red-constructor`, `green` logs, `structural-recovery-before.txt` (a
verbatim copy of the pre‑fix `session.go`), `structural-recovery-overlay.json`,
`ci-repair-*.log`, `reviews/*-agent.log`. AGENTS.md: "Build the software. Not
ledgers … proof directories, or evidence manifests." The worklog already records
the decision, the overlay technique and the loader observation; the PR body
records the run. Leave the logs out of the PR.

### 3. nitpick — lifecycle leftovers in `session.go`

- The crash‑recovery branch calls `closeHost(active)` twice (`session.go:233`
  and `:241`); the second is a no‑op after the map delete and misleads about
  ordering. Delete it.
- `publishHost` renames the `go.sock` alias before `route.setApp`
  (`session.go:165-181`). `setApp` cannot fail today (`work` is absolute), but if
  it did, the rebuild path would `closeHost(replacement)` and leave the alias
  pointing at a removed socket while the coordinator still proxies to the old
  host. Call the fallible step first or drop the error return. The recovery
  branch now returns a `publishHost` error where the old code ignored
  `setApp`'s; that is better and worth one worklog line.

### 4. nitpick — two docs sentences still read as production claims about development

`README.md` row "Server-side fetch … Direct SSR socket subfetch cookie relay is
limited in development" is fine, but `docs/features.html` keeps the production
`ssr.feature` link beside "In production, Kit's renderer runs in Go" without
saying development renders in Node, which the lede on `docs/index.html` does
say. One clause on the row would keep the two pages consistent.

## Not findings, recorded for the merge decision

- A draft of this review carried a second issue: CONTINUE.md still said the
  final product repairs had "no remaining findings" while README now links it as
  the evidence record. `5b43304` rewrote CONTINUE.md to mark the PR #327 results
  historical, name the PR #329 lifecycle repair with its green and overlay‑negative
  runs, and state the native‑loader limitation and what it does not prove. That
  matches the on‑disk evidence; the finding is withdrawn.

- Round‑01 nitpick 3 was rightly rejected: `example/e2e/global-setup.ts:10-18`
  HEADs the base URL and fails unless `X-Skgo-Mode` equals the requested mode,
  so `just e2e dev` cannot mislabel a production server. Nitpick 4's
  adjudication (library minimum in go.mod, exact gate in dev) matches the docs
  now shipped.
- Kit patch change is sound: `vp exec pnpm` runs with `cmd.Dir` set to the
  frontend (`kitpatch.go:651-659`); the `toolchain.PNPM` version check is kept
  (`:671`); `mustPNPM` resolves from `example/web`. The CLI test puts an
  exploding `pnpm` first on PATH and points `VP_CLI_BIN` at the project CLI; CI
  installs that CLI via `just install` before `just test` (`ci.yml:52,64`); the
  fixture's `devEngines` pin equals `toolchain.PNPM` (12.9.1). `ci-repair-test.log`
  is green for the whole module; a fresh CI run for the final revision was in
  progress (`ci-repair-final-remote.log`) and its result was not available here.
- The symlink publish is correct: the host listens on its private socket before
  replying ready (`host.go:597`), `connect()` follows symlinks, `os.Rename` over
  an existing symlink is atomic, and `closeHost` removes only the candidate's own
  socket. The obsolete branch now closes `current.Inputs`, fixing a leak.
- The integration fixture no longer locks; `sync.Mutex` remains only in the
  test's log buffer, outside hot‑replaced code.

## Outcome

material findings remain
