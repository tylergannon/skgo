# PR #438 independent review, round 01

## Target and authority

- PR: https://github.com/tylergannon/skgo/pull/438, “ci: consolidate ARM64 runners and retire old installers.”
- Reviewed local head `a2745f95416c043e01a7dfdd5537b24fa44dcfa4` against `origin/main` at `550ff39065452e65d2f8157b5f0d6192125b484d`. The remote PR still reported `db709fd6ed0f493dbefb07b523de87ee5048874d` during review; the local head refreshes that branch with current main.
- Authoritative outcome: review and merge #438, then verify actual ARM64 release jobs on shared runners and certify Bob and Amritanandamayi. The caller clarified that fresh build-only release verification on merged main is planned, because the release environment restricts branch runs. No final certification has been asserted.
- Read repository `AGENTS.md`, the agent-protocol and adversarial-review skills, the PR description, and relevant runner migration worklogs. No scope-narrowing instruction was supplied or ignored. Review remained read-only except this artifact.

## Evidence inspected

- Full diff against current main: shared Linux ARM64 routing in `.github/workflows/release.yml`, matching actionlint label, centralized runner ownership instructions, removal of the superseded repository-owned Docker/Compose/systemd/launchd deployment definitions, and the migration decision worklog.
- Surrounding release workflow, ordinary CI and qualification workflow routing, `cmd/skgo-release/archive_test.go`, release build implementation and its explicit Go toolchain identity checks. Searches found no live workflow or script references to the removed deployment files.
- Shared runner infrastructure repository's current main tree, Linux image, example fleets, and ownership/requirements worklogs. These support the ownership transfer and ARM64-capable shared image; example fleet files are templates, not proof of deployed configuration.
- [Release run 40023734890](https://github.com/tylergannon/skgo/actions/runs/40023734890): Linux ARM64 job `120656265106` ran on `runner-99b5c7da` using label `skgo-managed-linux-arm64`; setup reported `go1.27.2 linux/arm64`; actual CLI packaging, `TestPackagedCLI` (PASS, no skip), external plugin use and artifact upload succeeded. The signed Mac job failed before starting, with no runner assigned. Publication jobs were skipped.
- [Earlier successful build-only release run 39999767885](https://github.com/tylergannon/skgo/actions/runs/39999767885): signed Mac job `120572656464` ran on `runner-e6e00e4a` with label `skgo-managed-macos`; signing setup, signing/notarization, packaged CLI/plugin witness, artifact upload and signing cleanup all succeeded. Its Linux ARM64 job still used the legacy Bob runner, so this run alone does not establish the new Linux route.
- [Ordinary CI/qualification run 40025494832](https://github.com/tylergannon/skgo/actions/runs/40025494832): six successful checks used managed Linux/Mac workers, including native Mac process qualification. This historical run predates current main's Go 1.27.3 updates.
- `git diff --check origin/main...HEAD` passed. `actionlint -ignore 'unexpected key "queue" for "concurrency" section' .github/workflows/release.yml` passed. The exclusion is limited to the pre-existing supported queue field.

## Findings

No material implementation findings or genuine nitpicks. The change preserves the release build commands, executable/plugin witness, matrix failure isolation and publication dependencies while replacing only the ARM64 runner route. Removing the old infrastructure definitions is consistent with the explicit ownership transfer. The corresponding managed ARM64 job has actually executed successfully, and the unchanged managed Mac route has historical signed-release proof.

## Proof boundary and remaining delivery

This review does not certify the present fleet. Historical release proof uses older workflow/helper revisions and Go 1.27.2, whereas the reviewed head includes current main's Go 1.27.3 alignment. The PR's branch release run also never exercised signed Mac release steps. The authorized next stage is therefore a fresh build-only release dispatch from merged main, with publication disabled, inspecting both ARM64 jobs' actual runner identity, build/package results, non-skipped external plugin witnesses, upload results and Mac signing cleanup. Host/fleet inspection must establish that the managed routes are operated by Bob and Amritanandamayi; a transient runner name alone does not identify its physical host. This is explicitly outstanding delivery work, not a claim that the implementation review found a defect.

## Outcome

no findings
