# Safe failed generation — #253

Source: https://github.com/tylergannon/skgo/issues/253

## Original body

On main d034f77, independent Opus validation of issue #247 reproduced a generator rollback defect. This is a generator cleanup defect, not a failing fixture contract.

Reproduction in a disposable checkout with installed dependencies:

1. Append `var _ int = "unrelated demo consumer is broken"` to `example/web/src/routes/about/page.server.go`.
2. From `example/internal/skgo`, run `go tool skgo generate --web ../../web`.
3. The command correctly exits 1 on that invalid assignment. Inspect `git diff -- example/internal/skgo/links` from the repository root.

Observed: 49 tracked mirror files changed, including `skgo_remotes_gen.go` copies reduced to headers and the invalid about handler copied into the mirror. Restoration in `Run` / `resetStaleGeneratedFiles` restores blanked original sources but leaves mirror copies modified. The stated failed-generation restoration contract does not hold for this directory. This is separate from intentionally retaining refreshed typed-parameter declarations before stale-handler compilation fails; those declarations must remain refreshed.

Expected: a failed generation should not leave transient blanked remote declarations or the failed staging copies in the link tree. Preserve the intentional typed-parameter refresh behavior while cleaning up these unrelated transient mutations.

The validator restored all experimental edits and verified original status, diff hash and source checksums. This does not block the isolated typed-load fixture slice: the isolated test passes with the unrelated consumer broken; the old whole-example-copy test fails before reaching its assertions. No runtime fix is included in the fixture PR. Full reproduction output is in the committed `ephemeral/reviews/20261006-typed-load-slice-validation.md` report accompanying that PR.


## Comment by tylergannon — 2026-10-18T16:58:03Z

Source: https://github.com/tylergannon/skgo/issues/253#issuecomment-6252599696

**Triage — 2026-10-18**

Checked against current [`main` at `920b6aca`](https://github.com/tylergannon/skgo/tree/920b6aca738a65d6c046a67f4db2373596fafd03). Classification: `bounded-hard`. 

The original resetStaleGeneratedFiles/blanked-declaration mechanism is gone after #266; generation now plans output and publishes it through internal/gen/output.go, with mirror sync at publication. I looked for the analogous failure-cleanup problem: TestGenerationOwnsOneFileAndIgnoresPreviousDeclarations has an authored-error/output-preservation assertion, but the exact failed-page-load/mirror case needs requalification before closing. Attempting that current generator test failed at its prerequisite because this worktree lacks installed pinned Kit source; it did not run its assertions. Keep open to reproduce the mirror invariant on the current publication path, not to restore or patch the deleted reset mechanism.

<!-- skgo-issue-triage-20261018-920b6aca -->

## Cache annotations and execution contract

Cache root: `/Users/tyler/src/skgo-token-cache`; routed from `index/README.md` through `index/selective-generation.md`, `index/adoption-generation.md` and `index/source-families.md`. Historical observations do not establish current behavior.

**Goal.** A generation rejected for an authored page-load error preserves the previous generated link mirrors, without publishing blank declarations or the failed authored staging source. Intentionally refreshed typed-parameter declarations remain refreshed when an obsolete handler subsequently fails compilation.

**Original evidence.** `repos/architecture-20261015/skgo-dev-868c5db/ephemeral/reviews/20261006-typed-load-slice-validation.md:62-65` records the invalid `about/page.server.go` assignment and exit 1. Lines 124-138 document the 49 dirty mirror files and the old restore mechanism's gap. That mechanism has since been removed; this is a reproduction target, not a prescription to restore the old code.

**Relevant prior art, bounded.** `collection/adoption-generation.md:110-116` separates prepared artifacts from publication/cancellation in Vite's optimizer and SKGO staging. It supports preserving successful published state when preparation fails, but its architecture suggestions are historical and do not authorize a wider generation refactor. `index/selective-generation.md:5-10` distinguishes generator projection reuse from executable compiler validity; a generation cache hit cannot prove an authored body compiles.

**Current specification.** Installed `example/web/node_modules/@sveltejs/kit/package.json` reports **3.1.0**. The tracked `ephemeral/sveltekit-current/SKILL.md` was read. Kit's current `src/core/sync/utils.js:10-18` compares generated contents with the physical file before writing; Kit does not specify Go mirror failure rollback. The issue supplies that Go-owned contract. Current skgo `internal/gen/output.go` plans outputs and mirror publication, as the latest triage says. Requalify that path instead of patching deleted `resetStaleGeneratedFiles` behavior.

**Independent proof.** Ordinary Go tests should first establish a successful generated fixture containing a real page load and remote declarations, then introduce the issue's invalid authored assignment and run the real generator. Require the intended authored-error diagnostic and compare generated mirror membership and bytes to the independently established successful fixture. Verify no failed page-load body or transient blank declarations were published, and prove recovery after repairing the authored error. Retain the separate existing typed-parameter refresh regression (`internal/gen/load_params_test.go`) so failure preservation does not become rollback of intentionally refreshed params. This is a generator contract; a browser suite or repeated production builds adds no necessary proof.

**HITL.** No unresolved product or design choice. Executable now: reproduce against the current publication path; repair only a verified remaining violation, or add the exact missing regression if prior changes already satisfy the invariant. No redesign, blanket transactional guarantee for every I/O failure, or restoration of deleted mechanisms is implied.

**Current-contract clarification after validation.** #266 (`c6906b7b`) deliberately made persisted generated Go output-only; `ephemeral/worklog/consolidated-generated-go.md:2` states that blank-and-restore is not the contract. The current typed-parameter refresh occurs in the compiler overlay and is published after success. Both focused tests and Opus's mutations validate that existing behavior. The older issue's on-disk refresh wording does not authorize reversing #266; this resolution preserves the current contract and adds the missing exact mirror regression.

**Rebase onto `66215e67`.** Main replaced copied mirrors with committed relative per-file symlinks under `_links` and removed the old output fixture. Preserve that architecture: authored edits are intentionally visible through existing links, including an invalid authored body. The supported failure invariant is unchanged generated declarations, inventory, and link membership/targets until successful publication. `TestGenerationCacheFreshProcessesAndRecovery` now reuses its existing compiler fixture with a page load and remote in a route directory, checks this invariant during real package analysis and after the issue's invalid assignment is rejected, and checks that repair publishes the pending file as a relative link. The focused qualification passed in 1.637 seconds; no additional consumer build or copied-mirror fixture was introduced.
