# Independent retained publication validation

The publication/rebuild experiment's assertions are load-bearing for the currentness guard. The original uncached test passed; an independent overlay that removed only `ProjectBuild.Commit`'s `p.current != b.snapshot` check failed at the expected obsolete-install assertion because revision 2 reached the host callback.

## Runs

Original run, `GOWORK=off go test -mod=mod -modfile ephemeral/experiments/20261015-retained-publication/experiment.mod -overlay ephemeral/experiments/20261015-retained-publication/overlay.json -tags skgo_retained_qualification ./internal/hotreload -run '^TestRetainedNativePublication$' -count=1 -v -timeout 15m`: PASS, 13.756s. Key observations are preserved in `original-key-lines.log`.

The control uses `omitted-commit-guard-overlay.json`, which maps `internal/gen/retained.go` to a private copy in this directory. That copy retains the closed-project check and removes only the revision comparison inside `ProjectBuild.Commit`. The same test exits 1 at `obsolete native install admitted err=<nil> callback=true`; see `omitted-guard-key-lines.log`. No production or author-owned experiment source was edited.

## What this demonstrates

- A revision-2 `ProjectBuild` held while a newer two-file revision is written and observed remains an immutable capture: its native object contains `old4101`, not `current5202`. This is a captured-project snapshot claim for the fixture's observed revisions.
- Revision 2 cannot diagnose or publish after revision 3 is observed. The original run records actual installed/event revisions `[1 3]` at that point and checks the last-good HTTP literals remain 3000 until revision 3 is committed.
- An active current `Commit` serializes `Observe`: the test blocks inside the install/event callback, starts `Observe`, proves it has not completed, releases publication, then observes revision 5. Literal HTTP values and Vite event frames follow `[1 3 4 5]`.
- The incompatible initializer selects `CompleteBuild`; the rebuilt host answers literal 9000 under a different PID. Compatible native edits then answer 9101 and 9202 on that replacement PID, with observed Vite frames for revisions `[1 3 4 5 6 7 8]` and no obsolete revision 2.

This does not establish atomic capture against arbitrary concurrent disk writes: the multi-file fixture writes both files before calling `Observe`, and the test validates the resulting captured revision. It also does not establish a production `skgo dev` integration or prove every structural change can rebuild; it exercises one initializer change and one retained post-rebuild edit path.
