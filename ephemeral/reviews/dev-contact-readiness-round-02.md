# Adversarial review: `codex/dev-contact-readiness` — round 02

Date: 2026-10-14. Reviewer: Claude (Fable 5.1), read-only except this file. Round 01 is
`ephemeral/reviews/dev-contact-readiness-round-01.md`; it is not modified here.

## Review target

- Branch `codex/dev-contact-readiness`, HEAD `bf12eac` (the hydration wait in
  `example/e2e/steps/form.ts:131-139`, reviewed in round 01) plus uncommitted changes:
  `internal/dev/digest.go` (+13/−5), new `internal/dev/digest_test.go`,
  `internal/dev/watch_bench_test.go` (+18/−3), and two worklog entries in
  `ephemeral/worklog/warm-dev-supervisor.md`.
- Authoritative sources as in round 01: CLAUDE.md, the failed release gate 38535859820
  versus the passing gate 38520695701, and round 01 finding 1 (the dev-lane slowdown).
- The caller asked for the concurrency and freshness tests to be checked for being
  load-bearing and for the relevant tests to be run; no scope narrowing was requested.

## Evidence inspected

- Full diff of the working tree and of `31010eb..HEAD`.
- `internal/dev/digest.go`, `digest_test.go`, `watch_bench_test.go`, `watch.go:29-33,88`,
  `server.go:195,338,362,372,472,642` (every `fingerprint()` caller), `inputs.go:74-78`,
  `reload.go:22-35,124-137`.
- Tests run from `example/web` via `vp exec`:
  - `go test -race -count=1 ../../internal/dev` → `ok 49.267s`.
  - `go vet ../../internal/dev` → clean.
  - `go test -bench . -benchtime 20x`: `BenchmarkDevRequestWithSourceTree` 2.02 ms/op,
    `BenchmarkDevConcurrentRequestsWithSourceTree` 0.87 ms/op (10-core M-series).
  - Mutation checks using `go test -overlay` against copies of `digest.go` in my
    scratchpad (no project file touched):
    1. restore the whole-walk lock (hold `c.mu` from `scan()` to `close()`):
       `TestDigestScansOverlapAndValidateTheirOwnFileVersion` fails at `digest_test.go:59`
       "second request's scan blocked behind the open first scan" after the 2 s timeout.
    2. drop the change-stamp from `sameVersion` (size+mtime+inode only): the same test
       fails at `digest_test.go:68` "stale published digest", and
       `TestFingerprintReusesBytesAndDetectsPreservedMtimeEdits` fails at
       `watch_test.go:168`.
    Both tests are therefore load-bearing for exactly the two properties they claim.
- Browser qualification `/tmp/skgo-concurrent-browser.log` (dev mode, :8197, current
  source) and its server log, against `/tmp/skgo-contact-browser.log` (same machine,
  same suite, `bf12eac` without the digest change):

  | dev mode, local, 6 workers | before digest fix | after digest fix |
  |---|---|---|
  | regular lane total | 176 passed (3.5 m) | 176 passed (1.9 m) |
  | chromium scenarios median / p90 | 7.8 s / 9.7 s | 3.3 s / 4.6 s |
  | contact :65 / :82 | 6.3 s / 6.0 s | 2.5 s / 2.3 s |
  | noscript median | 4.3 s | 4.3 s |
  | source-edit lane | 11 passed (1.6 m) | 11 passed (1.3 m) |
  | whole suite | 187 passed (5.1 m) | 187 passed (3.2 m) |

  No scenario failed or was flaky; the server log holds only the usual `proxy error:
  context canceled` lines from navigations that abandon in-flight module requests and
  the two expected `/actions/options/no-ssr` warnings.

## Assessment of the change

`scan()` now takes the mutex only to snapshot the published map, each request walks and
stats against that immutable snapshot, and `close()` publishes its own map under a
short lock (`digest.go:33-52`). Freshness is unaffected: a reused digest still requires
the live `stat` to match ctime, size, mtime and inode (`sameVersion`, `digest.go:54`), so
an older scan publishing after a newer one can at worst cause a re-read, never a stale
sum; the test's "older scan publishes last" branch covers that path and fails under
mutation 2. Two scans publishing concurrently lose each other's newly hashed entries,
which costs one extra read on the next request and nothing else. `reads` is now
accumulated per scan and folded in under the lock, which is what made the race run clean.

This confirms round 01 finding 1's leading explanation: the whole-walk mutex was the
dominant cause of the 2.7× dev-lane slowdown. The hydration wait from `bf12eac` remains
correct on its own terms and now covers the actual remaining gap (hydration plus one
query round trip), not a serialised server.

## Findings

### 1. nitpick — the recovery is proven locally; the gate that caught the regression has not yet been re-run

The regression was measured on `ubuntu-latest` (median 2.7 s → 7.3 s). The recovery is
measured on a 10-core laptop (7.8 s → 3.3 s). CLAUDE.md's standard is `just e2e` green in
both modes on `main` and a release gate someone looked at; the branch's proof will be
complete when the dev job of the next release gate shows the regular lane back near 2.3 m
and the contact scenarios near 2 s. Nothing in the diff is blocked on that, but the PR
body should not claim the regression fixed until that number exists.

### 2. nitpick — the request path still walks the repository per module request

Every non-Vite-client request still runs `noticeEdit` → full `WalkDir` + ~1,000 `stat`
calls (`watch.go:35-80`), now in parallel. Locally that is ~2 ms of CPU per module
request, times ~110 module requests per hydrating page, times six browsers. The
`isViteClient` check also reads and parses `.svelte-kit/skgo-dev-supervisor.json` on
every request (`reload.go:22-35,124-137`). Neither is a correctness defect and the
measured latency is back to the pre-supervisor shape, so this is the remaining #311 work
the worklog already names, not a blocker. It is listed so the next optimisation starts
from the right place: document requests are the only ones that need the freshness
guarantee (`proxy.go:149` already classifies module paths), and the control descriptor
changes only when Vite restarts.

### 3. nitpick — benchmark `b.Error` inside `RunParallel` reports after the fact

`watch_bench_test.go:40` switched `b.Fatal` to `b.Error` so it is legal inside
`RunParallel`. A wrong response now lets the benchmark finish and print a number before
failing, which is fine for `go test -bench` but means a reader scanning only the ns/op
line could miss the failure. Harmless; noted because the project's rule is that a
passing number is not evidence on its own.

## Outcome

only nitpicks remain
