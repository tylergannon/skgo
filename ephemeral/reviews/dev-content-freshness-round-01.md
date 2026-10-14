# Adversarial review: dev content freshness, round 01

Date: 2026-10-14 (local). Reviewer: Claude Fable 5.1, read-only except this file.

## Target

- Branch `codex/dev-content-freshness`, worktree `/Users/tyler/.codex/worktrees/skgo-dev-content-freshness`.
- The review began against an uncommitted working tree. Mid-review another session committed it as `ca7748a fix: read current source bytes for dev freshness` and opened draft PR #319. The review target is therefore commit `ca7748a` (diff against `d69891c`, its parent). The working tree matched the commit at the end of the review.
- Mission (caller, restated from worklog `ephemeral/worklog/dev-content-freshness.md`): correct the released Linux dev freshness failure without weakening request-before-poll freshness, then qualify and publish the correction.
- No caller narrowing was ignored; the launch prompt set read-only and artifact-path constraints only.

## Evidence inspected

- `CLAUDE.md`, `agent-protocol` skill, the worklog, PR #319 body, commit messages `ca7748a`, `3fc316a`, `31010eb`.
- Full diff `d69891c..ca7748a`: `internal/dev/digest.go`, deleted `digest_{darwin,linux,other}.go`, `watch.go`, `server.go`, `digest_test.go`, `server_test.go`, `watch_test.go`, `watch_bench_test.go`.
- Surrounding code: `internal/dev/server.go` (`loop`, `awaitChange`, `noticeEdit`, `ServeHTTP`, `awaitSettled`), `internal/dev/inputs.go:74` (`Server.fingerprint`), `internal/dev/watch.go` (walk, `fingerprintPathsScan`), the `session.write`/`get` helpers in `server_test.go:159-200`.
- Tests run (all `-count` as stated, `go vet` clean on both platforms):

| Tree | Platform | Command | Result |
| --- | --- | --- | --- |
| `ca7748a` | macOS 27.0 / go1.27.2 darwin/arm64 | `go test ./internal/dev/ -run 'Digest\|Fingerprint\|TestDevHoldsARequest…' -v` | 5 tests PASS, none skipped |
| `ca7748a` | macOS | `go test ./internal/dev/ -count=1` (whole package) | ok, 55.0s |
| `ca7748a` | Linux 6.12.76-linuxkit / go1.27.2 linux/arm64 (Docker `golang:1.27`, source bind-mounted read-only, temp dirs on the container's own fs) | same targeted run, `-count=3` | 15/15 PASS |
| `ca7748a` | Linux | whole package `-count=1` | ok, 38.8s |
| mutant: `d69891c` implementation files (`digest.go`, three platform files, `watch.go`, `server.go`) with `ca7748a` test files | macOS | targeted run | 5/5 PASS (tests do not detect the released bug) |
| mutant | Linux | targeted run, `-count=3` | 15/15 PASS (tests do not detect the released bug) |
| pure `d69891c` (released) | Linux | its own targeted tests, `-count=3` | `TestDigestScansOverlapAndValidateTheirOwnFileVersion` FAIL 3/3 ("stale published digest"), `TestFingerprintReusesBytesAndDetectsPreservedMtimeEdits` FAIL 2/3, handler test PASS 3/3 |

The last row confirms the released defect is real on this kernel and that the deleted tests detected it. The mutant rows are the load-bearing check the caller asked for.

## Findings

### 1. Critical: no shipped regression test fails against the released implementation, on either platform

Evidence: mutant runs above. The released `fileDigests` cache plus the new test files pass 5/5 on macOS and 15/15 on Linux. The PR body's sentence "the released implementation fails the original regression five of five" is true of the *deleted* test (which called `cache.scan()`), not of any test in `ca7748a`.

Causes, per test:

- `internal/dev/server_test.go:396-404` (handler regression, the one test that still exercises the real `Server` freshness path): the added `os.Chtimes` restore is inert. `session.write` (`server_test.go:159-184`) creates a temp file and `os.Rename`s it over the target, so the edited file has a new inode. The released `sameVersion` (`d69891c:internal/dev/digest.go`) required `os.SameFile`, so the cache was never consulted for this edit and the released code always reread it. Restoring mtime on a renamed file reproduces nothing. This test passed against the released code before the change and still does.
- `internal/dev/digest_test.go` (both tests) and `watch_test.go:129` construct `&digestScan{}` or `inputs{}` with no cache. Under the released code a nil cache already read fresh bytes (`d69891c` `sum`: `if s.cache == nil { os.ReadFile }`), and `inputs.digests == nil` produced a cache-less scan. These tests cannot see a shared, cross-scan cache, which is exactly the hazard that shipped. `TestDigestReadsRapidPreservedTimestampEdits` would catch only a cache stored inside one `digestScan` value.

Impact: CLAUDE.md requires regression tests that "would actually fail if the feature were broken or removed", and the mission is to correct a *released* failure. If anyone reintroduces metadata-authorized reuse through `inputs`/`Server` (the shape it had), every test in this package stays green on every platform. The PR's proof paragraph therefore overstates what was demonstrated.

Reproduction: copy `d69891c` versions of `internal/dev/{digest.go,digest_darwin.go,digest_linux.go,digest_other.go,watch.go,server.go}` over a checkout of `ca7748a`, run `go vet ./internal/dev/ && go test ./internal/dev/ -run 'Digest|Fingerprint|TestDevHoldsARequestThatArrivesBeforeThePollSeesTheEdit' -count=3` in `golang:1.27` on Linux. All pass.

What a load-bearing version needs: the handler regression must edit `answer/answer.go` *in place* (same inode, equal size, `os.WriteFile` on the existing path, then `Chtimes` restore) and assert the literal "revision two" from a new PID. On Linux 6.12 that reproduces the ctime-equal case the released code missed. The unit-level test should drive `Server.fingerprint()` (or `inputs.fingerprint()` on the same `inputs` value twice) rather than a bare `digestScan`, so any future cache attached to `inputs` or `Server` is visible to it.

### 2. Issue: the request-path cost model changed and the benchmark no longer asserts the thing it was written to guard

Evidence: `internal/dev/watch_bench_test.go:13` comment changed from "A request must not reread a project's source bytes just to serve a route" to "Measure the source freshness check…". `server.go:634-655` `noticeEdit` now runs a full read-and-hash of every authored file on every non-Vite request, with no coalescing between concurrent requests (`3fc316a` deliberately removed serialization; `ca7748a` removes the shared result too, so N concurrent requests perform N full tree reads). The worklog and PR body report 2 ms → 13 ms per request on a 768 × 8 KB synthetic tree on an M4 with warm page cache.

Impact: the mission authorizes this trade and the PR body states it plainly, so this is not a hidden regression. It is an issue because (a) the requirement the previous commit encoded was dropped by editing a comment rather than by a stated decision in the worklog, and (b) the measurement was taken under the most favourable conditions (local APFS, warm cache). The case that motivated the fix is Docker Linux; a bind-mounted source tree from macOS into a Linux container reads files over virtiofs/gRPC-FUSE where per-file open+read is tens to hundreds of microseconds each, so the same tree costs well over 100 ms per request there, paid once per page request and again for each `__data.json`/remote call. No measurement from that environment is recorded. The publication gate should include one.

### 3. Nitpick: `TestDigestScansOverlapAndValidateTheirOwnFileVersion` now contains dead scenario

Evidence: `internal/dev/digest_test.go:12-71`. With no shared state, two independent `digestScan` values cannot block each other, so the 2-second "blocked behind the open first scan" branch and the "stale published digest" assertion (`:57-61`) describe a failure mode that no longer exists in the code. The test still verifies fresh bytes after an in-place write, which `TestDigestReadsRapidPreservedTimestampEdits` already does 64 times. The comment at `:12-13` and the failure strings should be rewritten or the test folded into the other one, so a reader does not infer there is still a publish/overlap mechanism.

### 4. Nitpick: PR #319 proof paragraph should not say "regression" for the deleted test

Evidence: PR body, paragraph 3. "the released implementation fails the original regression five of five" is accurate only for `d69891c`'s `digest_test.go`, which this PR deletes. State which test and that it no longer exists, or replace it with a result from a shipped test once finding 1 is fixed. CLAUDE.md: never claim anything that has not been demonstrated running.

## Outcome

material findings remain
