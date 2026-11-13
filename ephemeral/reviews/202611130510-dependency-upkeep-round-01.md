# Dependency upkeep review — round 01

Review target: `codex/dependency-upkeep` (`faf14796`, `27bd29a7`) rebased on `origin/main` (`e03b6fab`), against the dependency-maintenance request and repository instructions.

Evidence inspected:

- The complete `origin/main...HEAD` diff, including all package manifests, lockfiles, Kit queue compatibility assets, adapter changes, and generated `sv` add-on.
- The qualified production client handoff proof recorded in `ephemeral/worklog/2026111105-dependency-upkeep.md`.
- `go test -count=1 ./internal/toolchain` on the current review target (passes after the active toolchain-pin repair).
- The toolchain-pin contract, generated-app environment gate, VitePlus workspace policy, and `sv` add-on source.

## Findings

### Issue — public and package setup guidance now tells users to install versions skgo rejects

The corrected runtime requires VitePlus `1.6.0` (`internal/toolchain/pins.go:30`; the runtime gate is `internal/newapp/environment.go:43-45`) and the release now targets Kit `3.2.0` (`internal/toolchain/pins.go:16`). But the user-facing root `README.md:8,22` still says the release targets Kit `3.1.1` and requires VitePlus `1.3.0`. Likewise, the published add-on package guidance in `internal/sv/README.md:14` and adapter guidance in `internal/adapter/README.md:42` still name the old Kit queue correction.

Following the root README's VitePlus installation requirement makes `skgo new` fail before creating a project, because its documented 1.3.0 is explicitly rejected by the current 1.6.0 environment gate. Update the non-`docs/` setup/package guidance with the adopted release values. `docs/index.html` contains the same obsolete public requirements, but repository instructions prohibit writing under `docs/` without separate permission; preserve that boundary and obtain the required authorization for it rather than silently changing it.

Outcome: material findings remain.
