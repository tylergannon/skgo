On main d034f77, independent Opus validation of issue #247 reproduced a generator rollback defect. This is a generator cleanup defect, not a failing fixture contract.

Reproduction in a disposable checkout with installed dependencies:

1. Append `var _ int = "unrelated demo consumer is broken"` to `example/web/src/routes/about/page.server.go`.
2. From `example/internal/skgo`, run `go tool skgo generate --web ../../web`.
3. The command correctly exits 1 on that invalid assignment. Inspect `git diff -- example/internal/skgo/links` from the repository root.

Observed: 49 tracked mirror files changed, including `skgo_remotes_gen.go` copies reduced to headers and the invalid about handler copied into the mirror. Restoration in `Run` / `resetStaleGeneratedFiles` restores blanked original sources but leaves mirror copies modified. The stated failed-generation restoration contract does not hold for this directory. This is separate from intentionally retaining refreshed typed-parameter declarations before stale-handler compilation fails; those declarations must remain refreshed.

Expected: a failed generation should not leave transient blanked remote declarations or the failed staging copies in the link tree. Preserve the intentional typed-parameter refresh behavior while cleaning up these unrelated transient mutations.

The validator restored all experimental edits and verified original status, diff hash and source checksums. This does not block the isolated typed-load fixture slice: the isolated test passes with the unrelated consumer broken; the old whole-example-copy test fails before reaching its assertions. No runtime fix is included in the fixture PR. Full reproduction output is in the committed `ephemeral/reviews/20261006-typed-load-slice-validation.md` report accompanying that PR.
