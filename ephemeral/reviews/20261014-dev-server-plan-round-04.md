# Adversarial review: dev server plan, round 04

Date: 2026-10-14 (local). Reviewer: Claude Fable 5.1, read-only.

## Target

`ephemeral/projects/dev-server/PLAN.md` as committed at `d8261bb` on
`codex/dev-server-replacement-plan` (224 lines; the diff from `9b8534e` is
49 insertions and 45 deletions, all in the plan). The working tree is clean.
The authoritative request is unchanged: a complete, executable plan for
rebuilding the dev server after the deletion that issue #321 requested and PR
#322 merged to `dev` at `b2f2c00`, built from Vite, gopls, the selected
`pkujhd/goloader` and the indexed upstream implementations, with no
implementation foundation or compatibility obligation to the old server or to
the failed branch at `360b874`.

The caller restated one conversation-only requirement: the product is the dev
server, and GoLoader code replacement is its mechanism, not a separately named
mode. That was treated as a requirement to check, not a prediction of
findings.

Caller constraints honored: read-only except this artifact; artifact path as
supplied. No caller instruction narrowed subject matter or requested a verdict.
"The same authoritative sources" was read as the evidence set, not a limit.

## Evidence inspected

- The plan in full; rounds 01 to 03; the worklog
  `ephemeral/worklog/20261014-dev-server-replacement-plan.md` (its last four
  entries are the changes under review).
- Issue #321 (open, no comments) and PR #322 (merged 2026-10-14T20:47Z, base
  `dev`). `origin/dev` is at `b2f2c00`; `origin/main` is at `53179fc`, tagged
  `v0.33.0`.
- Repository process: `CLAUDE.md`/`AGENTS.md`, the `agent-protocol` skill,
  `Justfile` (`test`, `e2e mode="prod"`, `worktree` branching from `main`),
  `.github/workflows/ci.yml` (`just test` on `ubuntu-latest`; HEAD subject
  check via `cmd/skgo-release-version --check`), `qualification.yml` (prod
  only), `release.yml` (push to `main`; version from
  `cmd/skgo-release-version`; adapter and sv npm publication gated on the
  package changing).
- `cmd/skgo-release-version/main.go` (`--check` validates the subject only,
  so a merge commit with a Conventional subject passes; the release level is
  derived from every message since the last `v*` tag) and
  `internal/releaseversion/releaseversion.go:47,65-87` (`!` or a breaking
  footer maps to `Major`, and `Major` sets `major+1, 0, 0` with no special
  case for `v0`). `git log origin/main` contains no commit with a `!` subject;
  every tag to date is `v0.x`.
- `internal/newapp/newapp.go:41,68-76,695-735`: a generated app takes the
  adapter and sv add-on from the registry at the highest version compatible
  with the running skgo version, with `AdapterSpec`, `SVAddonSpec` (`file:`)
  and `SkgoReplace` overrides for qualifying a checkout; an unreleased
  checkout must pass `--skgo-version`.
- Installed pins: Kit `3.0.0`; `vite` resolves to
  `@voidzero-dev/vite-plus-core@1.0.0`, which exports `./module-runner`
  (`dist/vite/node/module-runner.js`) and whose bundled server reads
  `server.ws.clientPort`. Kit dev plugin `exports/vite/dev/index.js`:
  `update_manifest(false)` runs once in `configureServer` at `:149`; the
  asset middleware at `:272`; `remove_static_middlewares` at `:309`; the
  page/data/remote handler at `:311` is also the only caller of
  `update_manifest()` with parameter validation (`:315`); `full-reload` sends
  at `:119`, `:125`, `:248`.
- The plan's terminology: "native" appears only in cited cache paths
  (`reload-native`, `history-native`); "mode" appears only for the browser
  suite's prod/dev selection and the "not a new restart mode" denial;
  "profile" names the development build profile at lines 51, 55 and 118.

## Disposition of round-03 findings

1. Worktree and PR base (issue): addressed at line 98 (fetch `origin/dev`,
   explicit `git worktree add ... origin/dev`, `--base dev`, the `just
   worktree` recipe is not to be used and the first replacement PR owns
   fixing it).
2. Step 4's browser lane (issue): addressed at line 160 (step 4 restores
   `just e2e dev`, the Playwright development selection and the server-mode
   assertion; step 5 owns the CI matrix).
3. `dev`/`main` synchronization and promotion (issue): addressed at lines
   213 to 215 with a named sync subject and a named promotion title. The
   chosen title has an unstated consequence; finding 1.
4. `remove_static_middlewares` (issue): addressed at lines 30 and 154. The
   test it added is assigned to a step that does not own the listener it
   runs through; finding 2.
5. Preparation budget (nitpick): addressed at lines 106 and 184.

The caller's naming requirement is met: the plan calls the product the dev
server throughout and treats GoLoader as its mechanism. No separately named
mode, flag or lane remains in the text.

## Findings

### 1. Issue: the promotion title `feat!: replace the dev server` cuts `v1.0.0`, and the plan does not say so

Line 215: "Promotion is one squash PR from fully qualified `dev` into current
`main`, using the Conventional Commit title `feat!: replace the dev server`.
... The breaking marker preserves the deletion/replacement's release
significance in the squash commit."

`release.yml` derives the version from every commit since the last `v*` tag
with `cmd/skgo-release-version`, and `releaseversion.go:47` maps a `!`
subject to `Major`. `Next` at `:78` then produces `major+1, 0, 0` with no
`v0` exception, so the first push of that squash to `main` from `v0.33.0`
tags `v1.0.0` and, because the adapter package will have changed, publishes
`@skgo/sveltekit-adapter@1.0.0` and `@skgo/sv@1.0.0` under the same number.
No commit with a `!` subject has ever reached `main` (none in `git log
origin/main`), so this would be the repository's first major and its first
stable release, on a change whose own text says development support is being
restored rather than that the public API is stable. The plan presents the
marker as preserving "significance"; the release tooling treats it as a
version command, and the plan's "Validation and finish" section neither
states the resulting version nor says that the user chose it. A `refactor!:`
title would do the same. Only a non-breaking subject (`feat: ...`) produces
the `v0.34.0` that every other minor has produced.

Impact: an implementer following the plan to the letter publishes `v1.0.0`
of the Go module and both npm packages as a side effect of promotion, and the
generated-app pairing rule ("skgo vX pairs with the newest adapter at or
below X") then binds every future `v1.x` skgo to adapters from this release
line.

Fix: state the version the promotion is intended to cut and choose the
subject that produces it. If `v1.0.0` is the user's decision, say so in the
plan; otherwise use `feat: replace the dev server` and record that the
deletion's breaking marker on `dev` does not survive the squash.

### 2. Issue: step 3's exit condition is proved through the public Go listener, which step 4 owns

Line 30: "Step 3 owns this adapter routing boundary; step 4 owns public Go
routing and the origin/control handshake." Line 144 (step 3 ownership):
"adapter/client glue, upstream ModuleRunner/goja integration, private Vite
routing guard and live Kit metadata." Line 160 (step 4 ownership): "Go
session integration ... application control/readiness".

Line 154 (step 3 exit condition): "A browser reaches Vite module/assets and
the HMR socket only through the public Go origin, while documents, loads,
remotes, actions and endpoints are answered by Go. ... A Go test through the
public listener requests `/package.json` with an HTML Accept header and
receives the fixture's application 404 document, not project-file bytes;
legitimate module and asset URLs still work."

Both sentences require the public listener, the reverse proxy and the
WebSocket upgrade path that line 30 assigns to step 4, and step 3's
ownership statement excludes them. The plan's own rule for parallel work
(line 18: steps 2 and 3 "can proceed alongside" step 1 "under separate
ownership") means the step-3 owner cannot meet its exit condition without
either building step 4's proxy or waiting for step 4, and the step-4 owner
has no instruction to build the proxy before step 3 closes. The guard that
step 3 does own is testable at the private listener alone: a direct request
to the private listener for a page URL must not run Kit's handler, a direct
request for `/package.json` must not serve the file, and a module URL must
still work. The public-origin claims belong to step 4's exit condition, where
line 166 currently says only "wired once through every restored development
entry point".

Impact: with the sequence as written, the step-3 PR is either held open until
step 4 lands, or its owner builds a second proxy that step 4 then replaces.

Fix: move the two public-listener sentences from line 154 into step 4's
accepted proof or exit condition, and restate step 3's exit condition at the
private listener (Kit's page/data/remote handler cannot be reached, the
project-root file is not served, Kit's asset middleware and module URLs still
work).

### 3. Nitpick: the fresh-generated-app proofs depend on packages that will not be published during the `dev` period, and the plan does not name the qualification path

Line 162: "A fresh generated app passes the same behavior, not only the
repository example." Line 221: "Run the built production example and a fresh
generated application". Line 7: no releases during the `dev` period.

`newapp.go:704-708` selects `@skgo/sveltekit-adapter` and `@skgo/sv` from
the registry at the highest version compatible with the running skgo
version, and `currentSkgoVersion` at `:717-729` fails for a checkout that
does not identify a release unless `--skgo-version` is passed. The adapter
that carries the new Vite environment and HMR delivery will not be on the
registry until promotion, so every fresh-app proof in steps 4 to 6 must use
the checkout overrides (`AdapterSpec`, `SVAddonSpec` with `file:`,
`SkgoReplace`), which exist and which PR #322 used. The plan does not say so,
and an implementer who runs `skgo new` plainly gets the released adapter
without the dev plugin and a failure that looks like a step-4 bug.

Fix: one sentence in step 4 stating that generated-app proofs qualify the
checkout through the generator's existing checkout overrides, and that the
release-pairing proof is run once after promotion.

### 4. Nitpick: darwin/arm64 is required for step 1's exit but never runs in CI after step 1

Line 51 and line 122 require the primitive to pass on darwin/arm64 and
linux/amd64, and line 106 says "step 1 is not marked passed by a Mac-only
run". After step 1, `ci.yml` runs the root suite only on `ubuntu-latest`;
the existing `native-swift` job on `macos-15` (arm64) runs selected packages
only. Nothing in the plan keeps the darwin profile green on later pushes,
although it is the profile every local developer uses and the one whose
loader path (`mmap_darwin_arm64.go`, `MAP_JIT`) differs from Linux. The plan
assigns step 1 "aligning CI's selected toolchain" but not a darwin lane.

Fix: state whether the darwin profile is qualified in CI (for example by
adding the step-1 host tests to the existing macOS job) or only locally, so
step 5's "complete Ubuntu development qualification" is not read as the whole
platform story.

### 5. Nitpick: the suppressed Kit handler is also the only caller that validates param matchers

Line 30 suppresses Kit's post-installed handler at `:311`. That handler is
the only place `update_manifest()` runs with parameter validation
(`dev/index.js:315`); the `configureServer` call at `:149` passes `false`.
Kit's own dev server therefore reports an invalid `src/params/*` matcher as
a manifest error with a `full-reload`, and the plan's matrix row for
"New/removed/renamed route, layout, endpoint or matcher" (line 201) claims
"matching Go registrations" without saying which side validates a matcher.
This is a mirroring detail for the step-3 owner to map, not a design
question, but the plan's guard description implies the suppression is
behavior-neutral and it is not.

Fix: note in the topology paragraph that suppressing `:311` also removes
Kit's matcher validation pass, and leave the step-3 owner to decide whether
Go's registration already reports the same error.

## Below the five-finding threshold

Noted, not counted: the plan's source routes remain absolute paths on one
machine; the cache's `index/README.md` still names the canonical plan on
`codex/upstream-adoption-research`; line 98 refers to "the managed worktree
tool" with a `ref:` parameter, which is a tooling detail of one session
rather than a repository gesture, and the manual command beside it is
sufficient; the sync-commit rule at line 213 is correct for the checker as
written (`--check` validates the subject only), so merge commits with a
Conventional subject do pass CI on `dev`; and `vite/module-runner` is an
exported entry point of the installed `vite-plus-core@1.0.0`, so step 3's
direct-use check has a concrete module to bundle.

## Outcome

material findings remain
