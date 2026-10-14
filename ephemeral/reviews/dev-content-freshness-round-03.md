# Adversarial review: dev content freshness, round 03

Date: 2026-10-14 (local). Reviewer: Claude Fable 5.1, read-only except this file.

## Target

- Branch `codex/dev-content-freshness` at `bf41765`, on `ca7748a`, base `d69891c`. Draft PR #319.
- Since round 02 the only changes are the worklog (a "claim boundary" paragraph, uncommitted) and the rewritten PR #319 body. `git diff bf41765 -- internal/` is empty: implementation and tests are byte-identical to the round-02 target.
- Mission (unchanged): correct the released Linux dev freshness failure without weakening request-before-poll freshness, then qualify and publish.
- Authoritative sources: `CLAUDE.md`, `ephemeral/worklog/dev-content-freshness.md`, PR #319 body, rounds 01 and 02. No caller narrowing to note.

## Evidence inspected

- Worklog tail and the current PR body in full.
- `internal/dev/digest.go`, `watch.go`, `server.go`, `digest_test.go`, `server_test.go:393-408`, `watch_test.go:129-195` (unchanged, reviewed in rounds 01 and 02).
- macOS 27.0, go1.27.2: targeted `Digest|Fingerprint|TestDevHoldsARequest…` 4/4 PASS on the current tree (rerun this round).
- Round-02 results stand for identical bytes: Linux 6.12.76-linuxkit targeted `-count=3` 12/12 PASS and whole `internal/dev` package ok; released `d69891c` implementation under the current tests fails `TestFingerprintDetectsPreservedMtimeEdits` 3/3 on Linux and passes the HTTP test 3/3. Not rerun, since no byte under `internal/` changed.
- PR checks at review time: Swift client job passed on the latest run; "Build, tests, and Conventional Commit" pending on run 38547091298. Not green yet; publication waits on it per CLAUDE.md.

## Round-02 status

- Finding 1 (HTTP test presented as regression proof): **resolved in the PR body.** It now names `TestFingerprintDetectsPreservedMtimeEdits` through `New`/`Server.fingerprint` as the sole load-bearing regression and states that the HTTP test also passes against the released implementation and is not evidence of a stale browser. The worklog records the same boundary.
- Finding 2 (how the defect reached a developer): **resolved.** The PR body now describes the window as an equal-size, in-place, mtime-preserving edit sharing the recorded ctime tick with the preceding write or cache fill, reproduced on Docker Linux 6.12.76-linuxkit, and frames the 2→13 ms and 1.6→44 ms costs as an explicit trade.
- Finding 3 (digest test comment): **not addressed**; carried below as a nitpick.

## Findings

No material findings remain. The implementation is the minimal correct change for the mission, the one regression that can fail against the released code does so deterministically on Linux, the full package passes on both platforms, and the publication text now claims only what was demonstrated.

### 1. Nitpick: inert `Chtimes` restore in the HTTP test

`internal/dev/server_test.go:400-405`. The PR body now says this test passes against the released implementation, so the `os.Stat` and `os.Chtimes` lines prove nothing and cost a reader a question. Removing them, or a one-line comment pointing at the fingerprint test as the timestamp regression, would make the test say what it covers: a request arriving after an in-place edit and before the poll is answered by the rebuilt app.

### 2. Nitpick: commit title `bf41765` overstates

"test: exercise preserved timestamps through the real dev server" describes the test as proving the timestamp defect through the server, which the PR body now explicitly denies. The PR squash-merges, so the title disappears, but if the branch is ever read commit by commit the title and the PR body disagree.

### 3. Nitpick: `digest_test.go:12-13` comment

"ctime and mtime can both repeat across rapid writes" is asserted by `TestFingerprintDetectsPreservedMtimeEdits`, not by this test, which only shows a reused `digestScan` returns current bytes. Move or trim the sentence.

## Outcome

only nitpicks remain
