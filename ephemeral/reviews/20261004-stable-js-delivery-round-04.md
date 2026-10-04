# Adversarial review — stable JavaScript dependency delivery, round 04

Reviewed 2026-10-03 18:17–18:25 PDT (2026-10-04 UTC) in worktree
`/Users/tyler/Codex/2026-10-02/task-18/skgo-js-dependencies`, under the cached
`agent-protocol` and `adversarial-review` skills (hashes unchanged since round
03; no compaction occurred). Read-only except for this file; the builder owns
push, PR, merge and release. Bounded concurrency (`GOMAXPROCS=2
GOFLAGS=-p=2`); outputs kept under the transport limit. No scope narrowing in
the launch prompt to refuse; `--no-doppler` treated as an operating flag; the
project status site is out of scope by the caller's statement, not by mine.

## Review target

PR #237 "fix: adopt stable SvelteKit 3 and qualify current tooling", head
`4dbcc11` on `origin/codex/stable-js-dependencies`, base `main` = `bfea45a`
(tagged v0.15.1). Six commits; the two dependency commits are unchanged since
round 01, then `97d8b3d`/`18737ff` docs, `5d3e1a0` VitePlus bootstrap
prerequisite, and new since round 03: `18737ff` (journal: final local review
recorded; README gains `npm install --global vite-plus@1.0.0`) and `4dbcc11`
(pnpm 12.8.1 → 12.9.1 in both `devEngines`, three workflow pins, both
lockfiles' package-manager documents, registry.json entry). Working tree
clean; no orchestrator process alive.

## Evidence inspected

- `git diff 5d3e1a0 HEAD` in full (non-lock files) and lockfile hunks: both
  lockfiles change only within lines 7–159, i.e. pnpm's own
  `packageManagerDependencies` document (`@pnpm/exe.*` integrities); the
  application graph document (from line 158's `---`) is untouched.
- pnpm pin consistency: 12.9.1 in `example/web/package.json:30`,
  `example/e2e/package.json:17`, `ci.yml:44`, `qualification.yml:46`,
  `release.yml:78,148`; `mise x -- pnpm --version` → 12.9.1; `internal/sv`
  has no `devEngines` and no package-manager document (so "three frozen
  installs" = web, e2e, sv). `registry.json` records the 12.9.1 tarball,
  integrity and 2026-10-03T20:48Z publish time.
- Logs since round 03: `pnpm1291-{web,e2e,sv}-install.log` ("Lockfile is up
  to date … Done … using pnpm v12.9.1"), `pnpm1291-{web,e2e}-lock.log`.
  Note these were up-to-date frozen checks, not relinks:
  `node_modules/.modules.yaml` in web and e2e still records
  `packageManager: pnpm@12.8.1`.
- GitHub: PR #237 `mergeable: MERGEABLE`, `mergeStateStatus: CLEAN` after CI
  run 37167486860 on `4dbcc11` completed **success** ("Build, tests, and
  Conventional Commit": `skgo-release-version --check HEAD`, `just install`
  with pnpm 12.9.1 from scratch, `just build`, `just vet`, `just test`). That
  is the only from-scratch 12.9.1 install and the only `just test` at the
  exact head; it is genuine evidence.
- Release path: `release.yml` on push to `main` runs the reusable
  `qualification.yml` (build, vet/test, example BDD against the production
  build) → `version` (`skgo-release-version`) → `adapter-npm`/`sv-npm`
  (publish only if the package differs) → `tag`. `go run
  ./cmd/skgo-release-version` on this head derives **v0.15.2**;
  `internal/releaseversion/releaseversion.go:47-54`: `!`/BREAKING → major,
  `feat` → minor, `fix|perf|revert` → patch.
- PR body (read via `gh pr view`): states the base (`bfea45a`), the
  migrations, the validation counts, and the pnpm recheck; it is the intended
  squash message. The former `ephemeral/js-dependencies/pr-body.md` has been
  removed from the tree.
- Journal `SVELTEKIT_COMPATIBILITY.md` deltas: Status now "Delivery was
  explicitly authorized on 2026-10-04; exact-head CI and the normal release
  workflow are the remaining adoption gates"; pnpm row and a 12.9.1
  paragraph; "No code changed after those runs … Committing already checked
  content does not require repeating the checks."
- My own run at `4dbcc11`: `just vet` clean; `just test` 20 packages ok and
  one failure, `TestTheEmbeddedBuildIsNewerThanTheFrontendSource`:
  `web/pnpm-lock.yaml changed at 18:13:56 after the embedded build was
  written at 07:59:36` — the worktree's embedded build predates the pnpm
  commit. I did not run `just build` (it can rewrite generated sources); CI's
  fresh build-then-test at the same SHA is the controlling evidence.
- Round-03 verifications stand (code identical): redirect mirroring, Kit
  file identity, manifest wrapper, CORS literals, reproducible add-on, Go-owned
  HTTP / goja SSR / untouched private-field lowering, bootstrap probe with
  mutation controls, generated apps on VitePlus 1.0.0, screenshots viewed.

## Findings

### 1. issue — the squash title releases a consumer-visible compatibility break as a patch (v0.15.2)

*Incorrect implementation of the release semantics the builder now owns.*
The PR title `fix: adopt stable SvelteKit 3 and qualify current tooling`
becomes the squash subject, and `skgo-release-version` derives **v0.15.2**
from it. The change narrows what the published packages accept and what the
CLI accepts:

- `internal/adapter/package.json` peer `@sveltejs/kit`: `^3.0.0-next.0` →
  `^3.0.0` — a project on next.28 (the previous pin) can no longer install
  the new adapter.
- `internal/sv/package.json` peer `sv`: `^1.0.0-0` → `^1.0.1`.
- `internal/newapp/newapp.go:544` + `sv-addon.source.js:120`: `skgo new`
  and the add-on now refuse VitePlus other than exactly 1.0.0, including the
  0.3.3 that skgo itself pinned until this PR.

Each is deliberate and qualified, but to a consumer they are breaking, and
the body does not say so. The project's mapping makes `!`/BREAKING jump to
1.0.0, which a pre-1.0 project does not want, so the honest available signal
is `feat:` (v0.16.0) plus a sentence in the body naming the moved peer floors
and the `vp` requirement. One-word change before merge; no code involved.

### 2. nitpick — the head's local proof is CI's, not the worktree's

The journal says "No code changed after those runs" and "previous runtime
evidence covers unchanged application code". True of application code, but
at `4dbcc11` the worktree's embedded build (07:59) is older than the
committed lockfile (18:13), so `just test` fails its freshness guard locally
until `just build`; and the three "frozen installs with 12.9.1" were
up-to-date checks that left `node_modules` linked by 12.8.1. CI at the exact
SHA (fresh `just install` with 12.9.1, `just build`, `just test`, green) is
the proof, and release qualification will add the browser suite at the same
SHA. The journal should name that CI run as the exact-head evidence rather
than the local 08:09 logs.

### 3. nitpick — `dev.ts` waits on a different element than it clicks (carried)

`example/e2e/steps/dev.ts:202-204`: wait on `go-dev-endpoint-button`, click
the `enhanced-go-dev-form` button. Works because the page hydrates as one
component; the wait should name the element it protects.

### 4. nitpick — the bootstrap test has no platform guard (carried)

`internal/newapp/newapp_test.go:716-720` writes a `#!/bin/sh` fixture; CI is
`ubuntu-latest` only, so no present impact.

## Outcome

material findings remain
