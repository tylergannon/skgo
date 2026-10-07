# Adversarial review: RequestEvent after the rebase onto #269

Reviewer: Claude Opus 5.5, 2026-10-07. Read-only except for this file.

## Target

Branch `codex/request-event-plan` at `5a6176c`, rebased onto `origin/main`
`085456e` (#269, generator dependency-export reuse). Focus: integration repair
`5a6176c` (`internal/gen/locals.go`, `request_event.go`,
`request_metadata_test.go`) and the contracts it affects. Spec:
`ephemeral/plans/20261006-request-event.md` (stages 1–3; stage 4 not started),
`AGENTS.md`/`CLAUDE.md`, installed `@sveltejs/kit` 3.0.0.

## Rebase integrity

`git range-diff c6906b7..06a6fd6 085456e..dd170b1` reports all 18 pre-stage-3
commits as `=`, so their patches are identical. `f5c63d5` is the stage-3 change
reviewed in `20261007-request-event-stage3-opus.md`. Together these are the 19
replayed commits.

## Repair 1: staged locals path (`internal/gen/locals.go:113-120`)

#269's `preserveDependencyExports` → `stagePackageOverlay`
(`internal/gen/package_stage.go`) moves `packages.Config.Dir` into a temporary
`skgo-package-view-*` tree whenever a fresh overlay is staged. As a result,
`pkg.GoFiles` names files inside that view. The old code compared those files
against the authored module root, so it rejected valid locals packages as
"must be owned by the application module". Had the check passed, it would have
written `skgo_gen.go` into the temporary view instead.

The repair measures ownership relative to `load.Dir`, which is either the view
root or the authored root when nothing is staged. It then joins that relative
path onto the authored `host`. This works because `stagePackageOverlay` sets
`cfg.Dir = view + rel(logicalRoot, cfg.Dir)` and `cfg.Dir == host` here, so the
relative path is the same in both trees. A package outside the module still
produces `..` and is still refused.

**Load-bearing check (run):** in a cloned copy I restored the pre-repair two
lines and ran `go test -count=1 ./internal/gen/`. It failed 20+ tests, including
`TestStage2GeneratedLayoutDomainsAndTracking`,
`TestStage2RejectsInvalidEventAndParameterDomains`,
`TestSharedParamsSourceDiagnostics`, `TestTypedLoadActualParameterDependencies`
and `TestSharedParamsGeneratedNilLoads`, each with "locals package … must be
owned by the application module". With only the ownership line reverted (output
path left fixed), the narrower locals/caller/fresh-package selection stayed
green, so the full generator suite is what proves this repair. That suite runs
under `just test`.

Verdict: correct and complete for the regression #269 introduced.

## Repair 2: command/form context helper keeps cookie permission (`request_event.go:25`)

Kit 3.0.0 behaviour:

- `command.js:85`: `run_remote_function(event, state, true, …)` lets a command
  write cookies.
- `form.js:116`: a form body also runs through `run_remote_function`.
- `query.js:83-107`: `run_remote_function(…, { is_in_remote_query: true }, false, …)`
  forbids cookie writes in a query, including one called from inside a command.

The generator passes `event.Context(), event` to command and form bodies
(`internal/gen/emit.go:903-907`). That narrowed context is therefore the
command's own context. Stage 3 had cleared `mutable` there, which broke every
generated command or form that writes a cookie: Kit permits the write, and the
stage-3 code refused it. `5a6176c` restores `mutable` and keeps `query = true`,
which only gates caller reads (`loadevent.go:99,146,187`, `remote_caller.go:99`).

Actual query paths still derive an immutable event: refresh at `refresh.go:386`
and render-time queries at `document.go:1296`, both through `immutable()`. They
still refuse cookies and headers.

**Load-bearing check (run):** with `derived.mutable = false` restored in a copy:

- `TestRemoteRequestMetadataAndQueryRestrictionsAtServedEntries` fails.
- `TestGeneratedCommandFormCallerEvents` fails with
  `remote function ji2ixw/act panicked: 500 skgo: cannot set cookies in a query; only a command may write them`.

Both pass at HEAD. The updated root test now asserts that the narrowed command
context permits a cookie write while 12 caller-property reads are refused. It
also asserts that the refresh receipt refuses cookie writes.

Correction to my stage-3 review: that review recorded "keep command context
helper mutable → FAIL" as evidence that the stage-3 change was tested. It did
not notice that the generated command body itself receives `event.Context()`.
So the stage-3 change was a regression, and the review missed it. The repair
here is correct.

## Checks actually run

- In an APFS clone of the worktree at HEAD (byte-identical outside build
  products): `go test -count=1 . ./internal/...` passed for every package
  (root 2.2s, adapter 167s, gen 149s, dev 38s, newapp 18s, and the rest). This
  is the full generator suite after the final repair. Sol's record shows only
  the single repaired test rerun after that change. These runs did not use
  `-v`, so I did not independently count skips; Sol's zero-skip claim was not
  re-derived.
- Focused locals/caller/fresh-package generator tests in the real worktree
  passed (14s).
- The two mutation probes above.
- `go vet . ./internal/gen/` is clean; `git show --check HEAD` is clean.
- Not rerun: example regeneration drift, `just e2e`, and the served-example
  smoke test. Neither repair changes served bytes for an app that does not write
  cookies in a command, and the stage-3 served smoke check in the prior review
  still applies.

## Findings

### Blockers

None.

### Nonblocking

1. **issue (existed before this commit, not a regression): a query function
   called directly from a command can write cookies.** Kit forbids this
   (`query.js:104-107` passes `allow_cookies = false` even inside a command).
   In skgo, a nested query is a plain Go call, `nested(ctx)`
   (`internal/gen/remote_params_test.go:123-124`), on the same narrowed context
   the command body receives. That context is now mutable again, so
   `EventFrom(ctx).SetCookie` inside the query body succeeds.
   `request_metadata_test.go:189` asserts exactly that on this context.

   This matches the accepted stage-1/2 baseline at `06a6fd6`, where `Context()`
   never touched `mutable`. The stage-3 attempt to close the gap broke commands,
   because the command body and a direct nested query cannot be told apart by
   context. Closing it properly needs the nested query call to pass through
   something that derives an immutable event, which is a design question rather
   than a rebase repair. File it against the plan's "cookie writes remain
   forbidden in queries" rule. A refreshed or render-time query is unaffected.

2. Carried over from the stage-3 review and still open, nonblocking:
   `fetch.go:160-164`'s caller-page `fetchURL` has no load-bearing test.

No over-engineering. Both repairs are minimal.

## Outcome

material findings remain: no blockers. One nonblocking issue that predates this
commit (finding 1) should be filed, and the stage-3 test gap is still open. The
rebase and both integration repairs are correct, load-bearing, and green across
the full root and internal suites.
