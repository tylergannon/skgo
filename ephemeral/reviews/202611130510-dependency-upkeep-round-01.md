# Dependency upkeep review — round 01 (final re-review)

Review target: complete `codex/dependency-upkeep` branch (`faf14796`, `27bd29a7`, `56647c37`, `d37698f8`) on `origin/main` `e03b6fab`, against the dependency-maintenance request and repository instructions.

Evidence inspected:

- Complete `origin/main...HEAD` diff and surrounding toolchain, adapter, patcher, updater, generator, and user-guidance code.
- The qualified production client handoff proof recorded in `ephemeral/worklog/2026111105-dependency-upkeep.md`.
- Rebuilt `internal/sv` add-on; its generated `sv-addon.js` remains byte-current with `sv-addon.source.js` and now gates VitePlus 1.6.0.
- `just test` (full Go suite), which passed.
- `vp run check` (twice) and `vp run build` in `example/web`, all passing on the refreshed dependencies.

The initial re-review findings are resolved: the authoritative Svelte/VitePlus pins, workspace policy, generated add-on, package guidance, and tests now agree on Svelte 5.57.3, VitePlus 1.6.0, and Kit 3.2.0.

## Findings

### Issue — the public documentation site still instructs users to install a rejected VitePlus version

The current startup gate requires VitePlus 1.6.0 (`internal/toolchain/pins.go:30`; `internal/newapp/environment.go:43-45`), and the root/package guidance was corrected. But `docs/index.html:140,160-161` still says VitePlus 1.3.0 is required and supplies a `VP_VERSION=1.3.0` install command. A new user following that public setup page receives the command's explicit “requires qualified VitePlus 1.6.0” failure rather than a project.

The repository explicitly prohibits writes under `docs/` without permission, so this review did not modify it. Route the public-site correction through the authorized `skgo-project` site/governance path or obtain the required documentation permission before claiming release readiness.

Outcome: material findings remain.
