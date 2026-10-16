# Adapter publisher qualification review, round 01

Outcome: no findings

Target: commit `b040e25` on `codex/reuse-adapter-qualification`, removing the adapter publisher's repeated frontend installation/integration-test setup while retaining its qualification dependency. Reviewed against the original release request, repository instructions, the complete workflow change, surrounding publication helpers/package contracts, and run `38848380793`. No caller scope narrowing was supplied. Review remained read-only except this report and normal test scratch, with no secret reads or signing operations.

## Findings

No material findings or genuine nitpicks remain.

The failed adapter publisher's actual log shows `TestDevelopmentSnapshotUsesLiveKitGraphAtGoHandler` could not read `example/web/node_modules/$app/types/index.d.ts`. That integration test expects the built example's generated Kit state; installing dependencies into the publisher's separate checkout does not prepare it. The same run's qualification job had already built the candidate, successfully run the complete `internal/adapter` package (46.259 seconds), and passed all 176 production browser scenarios (45.1 seconds). This is concrete prior test execution, not an assumed passing command.

The adapter publisher still depends on `[version, qualification, linux-cli, macos-cli]` and therefore requires successful qualification from this same workflow run before publication. Qualification and publication both check out the run's source SHA. The correction removes the second, differently prepared integration-test environment rather than weakening the original full qualification.

Inspected the adapter package manifest and source plus `cmd/skgo-adapter-changed`: the npm package consists of tracked JavaScript/type/runtime assets, has no build or publish lifecycle scripts, and needs no generated frontend output for its publication. Comparison/stamping are standalone Go operations. The existing top-level release workflow path, GitHub-hosted npm job, `id-token: write`, configured registry, and direct npm trusted publication remain intact. Thus removing mise/frontend installation does not remove a publication prerequisite or alter the qualified adapter bytes.

## Load-bearing checks and verification

Ran from `example/web` through the declared VP environment:

```text
vp exec go -C ../.. test -count=1 ./cmd/skgo-adapter-changed
vp exec go -C ../.. test -count=1 ./internal/adapter -run '^TestThePublishedPackage' -v
```

Both passed. The three named package tests actually ran and packed the adapter once through VP: they require the literal expected package entry set, byte equality between the published runtime files and the embedded Go copy, and the expected package exports/peer-dependency shape. A missing runtime file or mismatched embedded copy would fail. The comparison/stamping tests exercise selection and preserve unrelated manifest bytes, including nested versions; the helper does not invoke npm or VP. `git diff --check` also passed.

The original run also records successful macOS notarization and all three archive/plugin jobs, and successful sv publication, followed by the correctly skipped final GitHub/Homebrew publication after the adapter job failed. This review does not claim that the corrected adapter publish or final release has run; that remaining delivery outcome requires the next real workflow execution. The already successful complete production qualification was not repeated for this workflow-only correction.
