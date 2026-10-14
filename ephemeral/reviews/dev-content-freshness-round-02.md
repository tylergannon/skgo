# Adversarial review: dev content freshness, round 02

Date: 2026-10-14 (local). Reviewer: Claude Fable 5.1, read-only except this file.

## Target

- Branch `codex/dev-content-freshness` at `bf41765 test: exercise preserved timestamps through the real dev server`, on top of `ca7748a fix: read current source bytes for dev freshness`. Base `d69891c`. Working tree clean. Draft PR #319.
- Mission (unchanged): correct the released Linux dev freshness failure without weakening request-before-poll freshness, then qualify and publish.
- Authoritative sources: `CLAUDE.md`, `ephemeral/worklog/dev-content-freshness.md`, PR #319 body, the round-01 review. No caller narrowing to note.

## Evidence inspected

- Diff `ca7748a..bf41765`: `internal/dev/digest_test.go` (overlap test deleted), `server_test.go:396-407` (in-place `os.WriteFile` replaces the rename-based `session.write`), `watch_test.go:129-195` (fingerprint test now built on `New(Config{...})` and `Server.fingerprint()`), worklog additions.
- Implementation unchanged since round 01: `internal/dev/digest.go`, `watch.go`, `server.go`.
- Runs (`go vet` clean everywhere, no skips):

| Tree | Platform | Run | Result |
| --- | --- | --- | --- |
| `bf41765` | macOS 27.0, go1.27.2 darwin/arm64 | targeted `Digest\|Fingerprint\|TestDevHoldsARequest…` `-v` | 4/4 PASS |
| `bf41765` | Linux 6.12.76-linuxkit, go1.27.2 linux/arm64 (Docker `golang:1.27`, source read-only mount, temp dirs on the container fs) | targeted `-count=3` | 12/12 PASS |
| `bf41765` | Linux | whole `internal/dev` package | ok, 35.5s |
| mutant: `d69891c` implementation (`digest.go`, three platform files, `watch.go`, `server.go`, byte-verified with `cmp`) under `bf41765` tests | macOS | targeted `-count=3` | 12/12 PASS (expected: APFS updates ctime on `Chtimes`, so the released cache reread) |
| mutant | Linux | targeted `-count=3` | `TestFingerprintDetectsPreservedMtimeEdits` FAIL 3/3 at `watch_test.go:167`; `TestDevHoldsARequestThatArrivesBeforeThePollSeesTheEdit` PASS 3/3; others PASS |

- CI: `.github/workflows/ci.yml` runs on `ubuntu-latest` and `macos-15`; the last two `main` pushes (`3fc316a`, `d69891c`, both carrying the released cache and its original tests) are green on both.

## Round-01 status

- Finding 1 (critical, no load-bearing regression): **resolved for the unit level.** The real-`Server` fingerprint regression fails deterministically against the released implementation on Linux and passes on the corrected tree. The worklog's "fails 5/5 on Linux" is now a claim about a shipped test and is reproduced here 3/3.
- Finding 2 (cost model, no bind-mount measurement): **addressed.** The worklog and PR body record 1.6 ms → 44 ms per request with the benchmark tree on a macOS bind mount inside Docker Linux, and the benchmark comment change is now explained as a decision rather than a silent edit.
- Finding 3 (dead overlap scenario): **resolved** by deleting the test.
- Finding 4 (PR proof paragraph names a deleted test): **resolved**; the paragraph now cites the shipped fingerprint regression.

## Findings

### 1. Issue: the HTTP handler test is still presented as regression proof but does not detect the released defect

Evidence: commit title `bf41765 test: exercise preserved timestamps through the real dev server`; PR #319 body paragraph 3, "an immediate real HTTP request after an in-place preserved-timestamp edit"; worklog, "the HTTP test now writes the existing file in place before restoring mtime". Against the released implementation on Linux the test passes 3/3 (mutant row above). Cause: in `server_test.go:393-407` the file was created before a roughly 3-second build, so the released cache holds a ctime seconds old; the in-place write assigns a new ctime regardless of the `Chtimes` restore, `sameVersion` fails, and the released code rereads the bytes. The defect requires the cache fill and the edit to fall in the same coarse kernel tick, which `TestFingerprintDetectsPreservedMtimeEdits` arranges and this test cannot.

Impact: the test remains a valid request-before-poll contract (it was before the change), but the `Chtimes` lines add nothing a reader can rely on, and the commit title and PR body describe it as exercising the preserved-timestamp defect through the real server. The PR body is the squash commit message. CLAUDE.md: never claim anything that has not been demonstrated running. Either drop the inert `Chtimes` restore and the claim, or state plainly that the sole load-bearing regression is the fingerprint test built on `New`.

### 2. Issue: the PR body overstates how the released defect reached a developer

Evidence: PR body paragraph 1, "The dev supervisor then reused a stale digest and could serve the application built before that edit." The released `sameVersion` required equal ctime, mtime, size and inode. On Linux both ctime and mtime come from the coarse clock, so a stale digest needed an in-place, equal-size write whose mtime matched the previous mtime and whose ctime matched the ctime the cache recorded, i.e. the previous write, the cache fill and the new write all inside one tick of roughly 1 to 4 ms. An editor save does not restore mtime. CI on `ubuntu-latest` passed the released tests on `main` twice; the failure reproduced only in Docker linuxkit here. The code change is the right one: a content check that depends on clock granularity is not a guarantee, and the mission chose correctness over the cache. But the publication text should say what the window was, so the recorded 1.6 → 44 ms bind-mount cost is understood as the price of removing a sub-tick window rather than of fixing stale pages developers were seeing. The decision belongs to the person publishing; the review's point is that the text must let them make it knowingly.

### 3. Nitpick: `digest_test.go` comment claims more than the test shows

Evidence: `internal/dev/digest_test.go:12-13`, "ctime and mtime can both repeat across rapid writes." The test asserts only that a reused `digestScan` returns current bytes; it does not observe ctime. The sentence is true on Linux but belongs beside the fingerprint regression, which is the test that would fail if it were false.

## Outcome

material findings remain
