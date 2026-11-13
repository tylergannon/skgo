# Dependency upkeep review — round 01

Review target: `codex/dependency-upkeep` (`faf14796`, `27bd29a7`) rebased on `origin/main` (`e03b6fab`), against the dependency-maintenance request and repository instructions.

Evidence inspected:

- The complete `origin/main...HEAD` diff, including all package manifests, lockfiles, Kit queue compatibility assets, adapter changes, and generated `sv` add-on.
- The qualified production client handoff proof recorded in `ephemeral/worklog/2026111105-dependency-upkeep.md`.
- `go test -count=1 ./internal/toolchain` on the current review target (passes after the active toolchain-pin repair).
- The toolchain-pin contract, generated-app environment gate, VitePlus workspace policy, and `sv` add-on source.

## Findings

### Issue — the generated-app add-on still rejects the newly qualified VitePlus

The active repair correctly moves the authoritative toolchain pin to VitePlus `1.6.0` (`internal/toolchain/pins.go:30`) and the example to the matching core. But `internal/sv/sv-addon.source.js:253-254` continues to throw whenever the installed VitePlus version is not `1.3.0`; its later Storybook compatibility conditional and exception remain keyed to `1.3.0` at lines 296-303. `skgo new` checks the user environment against `toolchain.VitePlus` (`internal/newapp/environment.go:43-45`) before invoking this add-on, so a correctly-qualified `vp 1.6.0` project proceeds to `sv add` and is then rejected by skgo itself.

This is a generated-consumer regression caused by the update: qualify and adopt the 1.6.0 add-on behavior (including its Storybook policy), regenerate `sv-addon.js`, and prove `skgo new`/the generated consumer. If that compatibility cannot be established, retain the prior qualified VitePlus instead of advertising 1.6.0.

Outcome: material findings remain.
