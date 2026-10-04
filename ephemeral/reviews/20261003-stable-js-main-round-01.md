# Adversarial review — stable JavaScript dependencies on current main, round 01

Reviewed 2026-10-02 19:30–19:45 PDT (2026-10-03 UTC) in worktree
`/Users/tyler/Codex/2026-10-02/task-18/skgo-js-dependencies`.

**Target.** Branch `codex/stable-js-dependencies` at `f1d0618`: two commits
(`796451d` stage, `f1d0618` qualify) with committer time 19:30:07, rebased onto
main `49cc2ea` (19:00:44, "preserve native SSR fields and settle dev reload
navigation"). The working tree was content-clean apart from uncommitted edits
to `SVELTEKIT_COMPATIBILITY.md` and the worklog made by another session during
this review; an orchestrator was validating the same worktree concurrently
(dev server, dev BDD, production build, svelte-check, production server and
BDD, generator qualification). Its logs are in `ephemeral/js-dependencies/logs`
and are cited below by name.

**Goal measured against.** Current feasible stable JS dependencies including
released Kit 3; behaviour-preserving migrations; HTTP/server logic stays in
Go; SSR stays in goja; native private fields preserved; validated delivery on
current main.

**Protocol note.** AGENTS.md mandates the `agent-protocol` skill; it is not
installed in this session. I proceeded without it and wrote only this file.

## Verdict

Substantively sound and mergeable once the journal is corrected and the
post-rebase evidence is recorded. The Go changes mirror Kit 3.0.0 exactly and
their tests are load-bearing; the adapter fix closes a real pre-existing bug;
no HTTP, server or SSR logic moved into JavaScript; the native-private-field
lowering from main is untouched and its test passes. The dependency selection
is defensible and documented. What is not yet true at `HEAD` is the "validated
on current main" claim: the committed journal describes a baseline that no
longer exists, and the post-rebase runs that make the claim true happened
during this review and are recorded nowhere in the tree yet.

## Findings

### Blocking

**F1 — The committed journal's baseline paragraph is false for the rebased
branch.** `SVELTEKIT_COMPATIBILITY.md` at `f1d0618` says main was inspected at
`69cf454`, that `8ee7a34` "and separate generator change `32a874e` were not
incorporated" and that "their combined behavior must be validated during any
later integration". On this tree: `git cherry main 8ee7a34` lists every PR 235
commit including `8ee7a34` as contained in main; `git diff 8ee7a34 main` is only
the dev-server drain change plus its tests; the branch commits were rebased
after that (committer 19:30:07). The integration the journal defers has already
happened, so a reader of `HEAD` is told to validate something that is the very
tree they are reading. `32a874e` (`internal/gen/package_overlay*.go`,
`scan.go`) is genuinely absent from main and from this branch — that half is
still correct.

An uncommitted edit in the working tree rewrites this paragraph ("rebased onto
released main `49cc2ea` … combined-tree qualification in progress … initial
validation below belongs to checkpoint `21573e1`"). `21573e1` exists (same
content as `f1d0618`, pre-rebase). That edit must be committed, and before
merge "in progress" must become results — a journal that lands on main saying
"in progress" is a stale claim the moment it lands. See the evidence section
for what those results now are.

**F2 — Post-rebase validation existed nowhere in the tree at review time.**
Every number in the journal's Validation section (dev 174, prod 174, `just
test`, vet, svelte-check, generator apps, retention bytes) comes from logs
written 18:50–19:25, i.e. against checkpoint `21573e1` before the rebase. The
orchestrator's post-rebase runs completed during this review (below) and are
green, but the first dev attempt on the rebased tree failed at startup
(`main-dev-server.log`: "the frontend build and this program come from
different skgo versions", adapter `3e2489e1a828` vs `96ba05c03469") until the
frontend was rebuilt — a reminder that a rebase onto a main whose adapter
identity changed is not a no-op for the embedded build. The journal must cite
the post-rebase logs and SHAs, not the pre-rebase ones.

### Should fix

**F3 — Generated-app Storybook evidence and the Storybook peer rule come from
different apps and the journal conflates them.** Timeline from log mtimes:
`stable-js-generator-ts-final` 18:50:23 and `-jsdoc-final` 18:51:39 (JSDoc demo
with Storybook; `pnpm install` printed `[WARN] Issues with peer dependencies
found`), then `stable-js-addon-build-final` 19:04:45 (the build that added
`storybook@10.6.1>vite-plus: 1.0.0`), then `stable-js-generator-peers` 19:05:51
(a second app, `generated-stable-peer`, with Storybook installed). I verified
the second app's `web/pnpm-workspace.yaml` carries the rule and `pnpm peers
check` reports "No peer dependency issues found". Chain is acceptable — the
rule only silences a warning, so the 18:51 Storybook build remains valid
evidence that the pair works — but the journal sentence "The JSDoc demo's
Storybook build passed; a fresh generated app's peer check passed" should say
these are two apps and that the Storybook build predates the rule.

**F4 — Two rot vectors in `sv-addon.source.js` with nothing to catch them.**
`vitest-browser-svelte` is hard-coded to `'3.1.0'` while the Vitest family it
must match is read live from `vite-plus/package.json`; and the Storybook peer
rule is guarded by `vitePlus?.version === '1.0.0'` exactly. A VitePlus `1.0.1`
in a generated project silently drops the rule (the peer warning returns; a
strict-peers install fails), and the browser-svelte pin drifts independently of
the Vitest version it was chosen for. Both are right today and documented in
the journal; neither has a test that fails when the qualified set changes.
Suggest a `newapp`/`internal/sv` test that asserts these literals against
`ephemeral/js-dependencies/registry.json` or fails loudly outside the qualified
VitePlus range.

**F5 — `interactive()` couples the suite to a Svelte internal, and one use
waits on the wrong element.** `example/e2e/steps/fixtures.ts` looks for an own
symbol with description `events` carrying a `click` handler. Verified against
installed Svelte 5.57.1: `src/internal/client/dom/elements/events.js:19`
`export const event_symbol = Symbol('events')` and `:149`
`(element[event_symbol] ??= {})[event_name] = handler`. It fails loudly on
rename (waitForFunction timeout), which is the right failure mode, but it is an
internal and will need re-mapping on Svelte upgrades. In `steps/dev.ts:202-204`
the wait is on `go-dev-endpoint-button` while the click is on the
`enhanced-go-dev-form` submit button; it works because the page hydrates as one
component, but the wait should name the element it protects.

**F6 — A product change was made to make a scenario deterministic.**
`example/web/src/routes/actions/+page.svelte` now renders
`disabled={sendRemoteNote.pending > 0}` and `actions-shared.ts` waits for
`toBeEnabled()` before reading the receipt. The rationale is source-backed:
Kit assigns `result` (`form.svelte.js:260`) before `await refreshAll()`
(`:279`), and `pending_count++` happens synchronously in the submit path
(`:467-469`), so by the time Playwright's click resolves the button is disabled
and the wait is load-bearing. It is a reasonable UX, but the scenario now
depends on the example's markup rather than on Kit alone; note it in the
feature or step comment.

### Informational

**F7 — Generated apps are looser about `vite` peers than the example.**
VitePlus's own migration writes `allowAny: [vite]` and `allowedVersions: vite:
'*'` into generated projects (seen in `generated-stable-peer`), while the
example pins `vite: "1.0.0"`. The journal states this difference; nothing to
change, but it means the example's "future core upgrades need a new
compatibility check" guard does not exist in generated apps.

**F8 — On-disk orphans in `example/web/node_modules/.pnpm`.** Two extra
peer-variant directories (`…vite-plus-core@1.0.0_…esbuild@0.28.2…`,
`@sveltejs+kit@3.0.0_…_f42c79…`) exist on disk but are absent from
`.pnpm/lock.yaml`; `pnpm why` reports one version each of kit and
vite-plus-core and the app's `node_modules/@sveltejs/kit` links to the
lockfile's variant. Leftovers of an earlier install; nothing tracked is wrong.

**F9 — `just test` depends on PATH order for Staticcheck.** `internal/check`
resolves `staticcheck` by `exec.LookPath`; `just test`'s `tools` dependency
installs 2026.2.1 into `.tools/bin` but does not put it first on PATH. With
`~/go/bin/staticcheck` 2025.1.1 ahead, `TestCLIAndInitializedStdioMCPExposeSameAdviceAndFailedCheck`
fails with "export data version 4 is greater than maximum supported version 2".
Pre-existing, not this branch; worth a worklog line because it looks like a
branch failure.

**F10 — Two-document lockfiles are pnpm 12's format, not corruption.**
`example/web/pnpm-lock.yaml` and `example/e2e/pnpm-lock.yaml` each contain two
YAML documents (`packageManagerDependencies` then the graph). Main's lockfile
has the same shape. Not a finding; recorded because it looks alarming.

## Verified claims (no action)

- **Trailing-slash redirect mirrors Kit 3.0.0 exactly.** next.28
  `src/utils/url.js:36-40` returned `${segment}/`; 3.0.0 `:38-43` returns
  `./${segment}/` ("prevents a colon in the segment from being interpreted as a
  URL scheme"); the removal case is `../${segment}` in both. Go
  `relativePathname` (`static.go:679-688`) matches both arms, and
  `endpoint.go` now calls it instead of its own copy. Query handling is
  equivalent: Kit drops a lone `?`, Go appends only when `RawQuery != ""`.
  `TestAPrerenderedColonPathRedirectStaysOnTheSameOrigin` and
  `TestABasedRootRedirectCannotTurnAColonIntoAURLScheme` assert the literal
  `./foo:bar/?q=1` and then follow the redirect to the Go handler / fixture
  page; both are anchored in fixtures, not in the code under test.
- **"Byte-identical to next.28"** holds for `src/runtime/server/remote-functions.js`,
  `server/data/index.js`, `server/csrf.js` and `core/generate_manifest/index.js`
  (`cmp` against `/tmp/skgo-stable-js-source-task18/kit-next28`, version
  `3.0.0-next.28`).
- **Adapter manifest extraction fix is real and pre-existing.** Kit 3.0.0
  `src/core/adapt/builder.js:218-233` writes `import { create_server } …; const
  manifest = …; export const server = create_server(manifest)`. The old regexes
  matched `new Server(manifest)` only, and the old guard `source === original`
  was satisfied by the `export const manifest` substitution, so the server
  import and call survived and importing the file evaluated Kit's server graph
  at build time. The new code tests all three shapes before writing or
  importing anything. `internal/adapter/manifest_test.go` is a literal
  upstream-shaped fixture with a server module that throws if evaluated and a
  lazy node; it refuses a missing `node` (`t.Fatal`, not skip). It runs `node`
  with cwd `example/web`, which is harmless: `skgo-adapter.js` imports only
  builtins and relative files. Passed in my run.
- **CORS diagnostic literals** match Kit 3.0.0
  `src/messages/server-errors.js:194-196` (`load_fetch_cors`, backticked header
  name, `https://svelte.dev/e/kit/load_fetch_cors`).
- **`sv-addon.js` is reproducible.** Rebuilt from `sv-addon.source.js` with the
  package's own tsdown command into my scratchpad: byte-identical to the
  tracked dist.
- **`newapp` prerelease exclusion is load-bearing.** The registry fixture now
  offers `1.1.0-beta.2`, which must lose to `1.0.1`.
- **Dependency selection.** `registry.json` records latest vs selected with a
  reason for every divergence (TypeScript 7.0.2 vs Kit `^6.0.0` and
  svelte-check `^5||^6`; `@types/node` 26 vs pinned Node 24.21.0; standalone
  Vite 8.3.2 / Vitest 5.0.3 vs the versions VitePlus 1.0.0 bundles). Kit 3.0.0's
  peers (`vite ^8.0.12`, `svelte ^5.57.1`, `typescript ^6.0.0`,
  `@sveltejs/vite-plugin-svelte ^7.0.0`) are satisfied by the installed graph.
  The lockfile's `@sveltejs/kit@3.0.0` integrity equals the registry entry.
  pnpm 12.8.1 is pinned consistently in both `devEngines`, all three workflows,
  and `mise.toml` bootstraps `vite-plus` 1.0.0.
- **Go owns HTTP, goja owns SSR, no application I/O in JS.** The branch's
  JavaScript changes are confined to the build-time adapter
  (`readKitManifest`), the generator add-on and the e2e fixtures; no runtime
  JavaScript was added or changed. `internal/adapter/skgo-adapter/env.js`
  (the syntax-based es2017 lowering from main `49cc2ea`) is not in the diff;
  `TestEngineLoweringUsesSyntaxRatherThanComments` passes in `internal/adapter`;
  the post-rebase production build logs "lowered to es2017 for the engine's
  parser: 7 module(s)" through that path.
- **Docs and instructions.** AGENTS.md (CLAUDE.md is a symlink to it), the
  Justfile comment and `ephemeral/sveltekit-current/SKILL.md` now point at the
  installed pin instead of the absolute inspiration path; `docs/` untouched;
  README status names 3.0.0 and the journal. Remaining `3.0.0-next.27/28`
  mentions in Go comments are provenance of captured fixtures, not claims about
  the current pin; the devalue-dependent fixtures still pass under Kit 3.0.0's
  `devalue ^5.9.4`.

## Evidence on the rebased tree (`f1d0618` on `49cc2ea`)

Orchestrator runs, observed from their logs and PIDs during this review:

| Step | Log | Result |
| --- | --- | --- |
| Dev server, first attempt | `main-dev-server.log` 19:31 | failed: adapter identity mismatch until frontend rebuilt |
| Production build | `main-prod-build-initial.log` 19:33 | built; 7 modules lowered |
| Dev server | `main-dev-server-fresh.log` 19:33 | listening in dev mode |
| Dev BDD | `main-dev-bdd-fresh.log` 19:34–19:39 | **174 passed (4.8m)**, no skips |
| Production build (post dev scenarios) | `main-prod-build-qualified.log` 19:40 | built |
| svelte-check | `main-svelte-check.log` 19:41 | 0 errors, 0 warnings |
| Production server | `main-prod-server.log` 19:41 | listening in prod mode |
| Production BDD | `main-prod-bdd.log` 19:42 | **174 passed (1.2m)**, no skips |
| Generator TS qualification | `main-generator-ts.log` 19:42 | running at review end |

My own runs (`PATH=.tools/bin:$PATH` where noted):

- `go vet ./... ./example/...` — clean.
- `go test -count=1 ./... ./example/...` at 19:35 — 19 ok; two failures, both
  environmental and both confirmed as such: `cmd/skgo` (F9, wrong Staticcheck
  on PATH) and `example` (`TestTheEmbeddedBuildIsNewerThanTheFrontendSource`:
  `destinations/types.ts` regenerated 19:33:47 by the orchestrator's dev
  session after its 19:33:13 build — the guard working as designed).
- A second attempt at `./example/` hit `[build failed]` because the dev suite's
  `zz-source-update` scenarios were rewriting generated sources at that moment;
  the tree returned to content-clean when they finished.
- `go test -count=1 ./cmd/skgo` with `.tools/bin` first — ok (37s).
- `go test -count=1 ./example/...` after the 19:40 build — all 5 packages ok,
  including the freshness guard and production SSR retention.
- Generated peer app `generated-stable-peer/web`: `pnpm peers check` — no
  issues.

Together: every package in both modules passed on the rebased tree, but in
three invocations rather than one `just test`. A single `just test` on the
final tree, after the orchestrator's generator runs stop touching it, should be
the number the journal cites.

What I did not do: open the box. No one has looked at a post-rebase page.
The journal's pre-rebase screenshot inspections do not carry over.

## Required before this counts as validated delivery on current main

1. Commit the journal correction (F1) and replace "qualification in progress"
   with the post-rebase results and log names above, naming `49cc2ea` as the
   base and the final branch SHA as what was qualified (F2).
2. Record one `just test` on the final tree after `just build`, with
   `.tools/bin` first on PATH, and note the PATH trap in the worklog (F9).
3. Open the example once post-rebase — home and the enhanced remote form at
   least — and say who looked.
4. Fix the journal's two-app Storybook sentence (F3).
5. Optional but cheap: F4 rot guard, F5 wait target.

Nothing here requires a Go or JavaScript change to the delivered behaviour.
