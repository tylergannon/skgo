# Frozen frontend installation repair — adversarial review

Outcome: material findings remain.

The lockfile change itself is correct and complete for the frozen installation
requirement. The two issues below concern what the repair has to carry when it
is committed and proven: neither asks for a different lockfile.

## Review target

- CI run <https://github.com/tylergannon/skgo/actions/runs/43588170049>
  (push, `codex/dependency-upkeep` at `03cdf3ed`, merged as #454 /
  `4f8ab3b6`).
- The uncommitted repair on `codex/fix-frozen-frontend-install` at `4f8ab3b6`:
  one changed line in `example/web/pnpm-lock.yaml:166` and the untracked
  `ephemeral/worklog/frozen-frontend-install.md`.
- Instructions applied: the worktree `CLAUDE.md`, and the reviewer and
  organization instructions read from
  `/Users/tyler/.agents/skills/adversarial-review/SKILL.md` and
  `/Users/tyler/.agents/skills/agent-protocol/SKILL.md` (slash resolution was
  unavailable, so both were loaded by path).

No caller instruction narrowed the review, so none was ignored.

## Evidence inspected

Read:

- The run's failed-step logs for both failing jobs, its job timestamps, and
  PR #454's merge time, commits, and check rollup.
- `git diff` of the working tree and the worklog.
- `example/web/pnpm-workspace.yaml`, `example/web/package.json`,
  `example/mise.toml`, `internal/toolchain/pins.go` and `pins_test.go`.
- Every frozen-install site: `.github/workflows/ci.yml:75,129`,
  `qualification.yml:111,137,160`, `release.yml:283`, and `Justfile:26`.
- The other three lockfiles and their workspace files (`internal/sv`,
  `plugins/cli/example/web`, `plugins/native-shell/example/web`).
- `cmd/skgo-release-version/main.go` and
  `internal/releaseversion/releaseversion.go`.

Run, all in scratch clones of this worktree with no `example/web/node_modules`,
using `mise -C example/web exec -- vp` (VitePlus 1.6.0, pnpm 12.14.0, macOS
arm64). The worktree itself was not touched.

| Experiment | State | Command | Result |
| --- | --- | --- | --- |
| A | `4f8ab3b6` as merged | `vp install --frozen-lockfile` | exit 1, `ERR_PNPM_LOCKFILE_CONFIG_MISMATCH` on `overrides`, the same error as the run |
| B | A plus the repaired lockfile | `vp install --frozen-lockfile` | exit 0, lockfile byte-identical afterwards |
| B | same clone | `vp run check` | exit 0, 1280 files, 0 errors, 0 warnings |
| B | same clone, `CI=true` | `just vet` | exit 0 |
| B | same clone, `CI=true` | `just test` | exit 0, 50 packages `ok`, no failures |
| C | `4f8ab3b6` as merged | plain `vp install` | exit 0, rewrites the lockfile; the result is byte-identical to the repair |
| D | installed `node_modules`, stale lockfile restored | plain `vp install` | exit 0, rewrites the same line |
| D2 | installed `node_modules`, stale lockfile restored | `vp install --frozen-lockfile` | exit 1, same mismatch |
| E | `4f8ab3b6` as merged | `vp install --frozen-lockfile` in `internal/sv`, `plugins/cli/example/web`, `plugins/native-shell/example/web` | all exit 0 |

Not run: the qualification workflow (`skgo_qualification` and
`skgo_dev_integration` tests), and nothing on Linux.

## What holds

- The worklog's diagnosis is right. The workspace override
  (`example/web/pnpm-workspace.yaml:27`), `package.json:30`, and the whole
  resolved graph are at `@voidzero-dev/vite-plus-core@1.6.0`; only the
  lockfile's stored `overrides` entry still said `1.3.0`.
- The one-line change is exactly what the package manager writes (C), not a
  hand edit that merely satisfies the comparison.
- No workflow change is needed. The run's own log shows mise resolving
  `npm:vite-plus 1.6.0` from `example/mise.toml`.
- No other lockfile has the defect (E). Only `example/web` declares a `vite@*`
  override.
- No new test is warranted. The frozen install in CI is the guard, and it
  caught this; a Go test comparing the two override strings would test pnpm.

## Findings

### 1. Issue — main derives no release for #454, and the repair's commit subject silently decides what ships

Incomplete requirement. The failed job is "Go contracts and Conventional
Commit" (`.github/workflows/ci.yml:20,51-52`), and agent-protocol rules 5 and 7
require a PR body that can serve as the squash message and a fix for broken CI
regardless of origin.

#454 was squashed under the subject `Refresh SvelteKit and frontend toolchain
(#454)`. On this branch:

```
$ go run ./cmd/skgo-release-version --check HEAD
"Refresh SvelteKit and frontend toolchain (#454)" is not a Conventional Commit subject (want type(scope): description)
exit status 1
```

`releaseversion.Analyze` reads only the first line of each message and skips
any that does not match (`internal/releaseversion/releaseversion.go:36-41`), so
the `feat: adopt SvelteKit 3.2.0` bullet in the squash body counts for nothing.
With the published `v0.46.1` tag fetched into a scratch clone, the derivation
on `4f8ab3b6` prints `none`.

The repair touches `example/web/pnpm-lock.yaml`, which `TouchesCore` counts as
core, so its squash subject alone now sets the next version:

- `chore:`, `ci:` or `build:`, the natural types for a lockfile fix, derive
  `none`. Kit 3.2.0, Svelte 5.57.3, and the VitePlus 1.6.0 gate stay
  unreleased until some unrelated `fix` or `feat` lands.
- `fix:` derives `v0.46.2`, shipping a Kit adoption and a changed required
  VitePlus version as a patch.
- `feat:` derives `v0.47.0`, matching the precedent: #449
  (`feat: adopt SvelteKit 3.1.1 …`) is the commit tagged `v0.46.0`.

The repair has no commit or PR yet and the worklog does not mention this, so
the choice is currently being made by accident. Decide the subject on purpose
and record why.

### 2. Issue — the recorded proof covers one command; the rest of the run has never executed on this content

Incomplete requirement. CLAUDE.md says passing tests are not proof of work, and
agent-protocol rule 4 requires the relevant proof before a PR is declared
merge-ready.

The run failed at the install step in both jobs and skipped qualification, so
no later step has ever run in CI against the Kit 3.2.0 / VitePlus 1.6.0 tree:
not `just vet`, `just test`, `vp run check`, or any qualification job. The
worklog verifies "the same frozen command" and nothing after it.

This review ran the three routine steps on a clean clone and they pass (table,
B), which discharges that part on macOS. What remains unproven is
qualification, on Linux runners. The repair's only changed path matches all
three selectors in `.github/workflows/ci.yml:64-66` (native, production,
development), so this branch's push run will be the first qualification of
#454's content. The repair is not proven until that run is green.

## Nitpicks

### 3. The worklog's correction omits how the defect actually reached main

The worklog attributes the escape to local checks that could not show a clean
frozen bootstrap. That is true but is not what let it through. #454 was merged
at 11:03:31Z; the push run started at 11:03:18Z and its two jobs failed at
11:03:46Z and 11:03:49Z. The control worked and the merge did not wait for it.
That is the actionable line for the next agent.

The instruction to reproduce "without installed frontend dependencies" is also
stricter than needed: the frozen command fails identically with `node_modules`
present (D2), and a plain install over either state rewrites the lockfile and
leaves a visible diff (C, D).

## Observed outside the target

Not a finding against this repair. `docs/index.html:140,160-161` on main still
tells users to install VitePlus 1.3.0 (and names Go 1.27.2), while
`internal/toolchain/pins.go` requires 1.6.0. This is the unresolved finding
from `ephemeral/reviews/202611130510-dependency-upkeep-round-01.md`; #454
merged with it open. `docs/` needs permission to change.
