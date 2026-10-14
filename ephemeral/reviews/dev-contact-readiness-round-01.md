# Adversarial review: `codex/dev-contact-readiness` follow-up — round 01

Date: 2026-10-14. Reviewer: Claude (Fable 5.1), read-only except this file.

## Review target

- Branch `codex/dev-contact-readiness`, HEAD `bf12eac` "fix(e2e): wait for contact page
  hydration before inbox readiness", one commit on top of `31010eb` (main, the squash of
  PR #315 / warm generation + shared dev supervisor). Working tree clean.
- Diff: `example/e2e/steps/form.ts:131-139` adds `await hydrated(page);` as the first
  statement of `When('the contact page has loaded')`, before the three unchanged
  positive assertions (`*-failed` count 0, `messages` attached, `contact-form` visible);
  `ephemeral/worklog/warm-dev-supervisor.md` adds one "correction" entry.
- Question posed by the caller: does the change repair a test prerequisite or mask a
  product failure? Both are answered below; the caller's framing did not narrow scope.
- Authority: CLAUDE.md ("A passing command is not evidence on its own", "Never derive
  what you expect from the thing you are testing", "Then open the box"), the failed
  release gate run 38535859820, and the preceding successful release gate 38520695701.

## Evidence inspected

- Failure log `/tmp/skgo-release-dev-failed.log` and the dev-lane job logs of runs
  38535859820 (job 115758439720) and 38520695701 (job 115709457008), fetched with `gh`.
- `/tmp/skgo-release-dev-artifacts`: both failing scenarios' `trace.zip` (network
  timelines parsed), `error-context.md` DOM snapshots, `test-failed-1.png`, `server.log`.
- Local suite `/tmp/skgo-contact-browser.log` (candidate source, dev mode on :8197):
  176 passed (3.5m) regular, 11 passed source-edit, 187 passed (5.1m) total.
- Source: `example/e2e/steps/fixtures.ts:239-266` (`hydrated`), `form.ts`,
  `example/web/src/routes/contact/ContactForms.svelte:179-203`, `internal/dev/server.go`
  (`ServeHTTP`, `noticeEdit`), `internal/dev/reload.go:22-35,124-137` (`control`,
  `isViteClient`), `internal/dev/inputs.go`, `internal/dev/watch.go:29-80`,
  `internal/dev/digest.go`, root `proxy.go:149` (`viteOwned`), and the same paths at
  `525bf42` for comparison.
- `go test ../../internal/dev -bench .` via `vp exec`: `BenchmarkDevRequestWithSourceTree`
  1.91 ms/op locally (20 iterations).

## What the failure actually was

The DOM snapshot at the 5 s timeout shows the complete SSR document: nav, sign-in,
both contact forms, and the inbox rendered as the boundary's pending snippet
(`<p data-testid="messages-pending">loading…</p>`). The `messages` `<ul>` only exists
once kit's client has hydrated and the `getMessages` query has resolved
(`ContactForms.svelte:179-203`). So the step as written has always required hydration
plus one remote round trip to finish within Playwright's 5 s expect timeout, with no
explicit wait for either. In that narrow sense the candidate is a legitimate repair:
`hydrated()` (`fixtures.ts:239-266`) is kit's own `history.scrollRestoration === 'manual'`
marker with a 30 s ceiling, it is already used by the sibling `the optional form is
ready` step (`form.ts:35-36`) and by the other contact steps, and the three positive
assertions stay in place. Nothing is inverted or weakened.

Why it took more than 5 s is the part the candidate does not address. Both traces show
111–112 module requests spread over 5.3 s with no JS error; the first remote request
(`/_app/remote/14q4me9/getMessages`) is only *starting* at 5.3 s in the second trace.
Every module request routed through the supervisor's `noticeEdit` path has a server
wait of 150–300 ms, uniformly, even for kit runtime modules Vite had already transformed
for earlier scenarios (the dev server had been up since 08:34, the failures are at
08:39). The two requests that bypass that path (`/@vite/client`, `/@vite/env`, matched
by `isViteClient`, `reload.go:124-137`) took 5 ms and 7 ms.

## Findings

### 1. issue — the candidate makes the release gate blind to a 2.7× dev-mode slowdown that the gate just caught

Apples-to-apples on `ubuntu-latest`, same suite, same six workers:

| lane (dev job) | run 38520695701 on `525bf42` | run 38535859820 on `31010eb` |
|---|---|---|
| regular chromium, 143 scenarios | 2.3 m total; median 2.7 s, p90 4.2 s | 5.0 m total; median 7.3 s, p90 8.9 s |
| noscript scenarios | median 1.4 s | median 1.8 s |
| source-edit, 11 scenarios | 2.5 m | passed |
| contact scenarios :65 / :82 | 2.1 s / 2.2 s | timed out at 5 s (5.6 s / 5.4 s) |

The prod job is unchanged between the two runs (176 passed in 1.5 m both times, the
same two contact scenarios 1.0–1.3 s). Noscript scenarios, which load no modules, barely
move. Only hydrating pages in dev mode slowed, by ~2.7× at the median, and the local run
of the candidate shows the same shape (chromium median 7.8 s, p90 9.7 s; the two
contact scenarios 6.3 s and 6.0 s). The two timeouts were the tail of that distribution
crossing a fixed 5 s expect, not an isolated ordering race.

What changed on the hot path between `525bf42` and `31010eb` (`server.go:604-628` vs
the old `:422-438`): every non-Vite-client request still calls `noticeEdit` →
`fingerprint`, but `in.fingerprint()` now takes `in.digests.scan()` which holds
`fileDigests.mu` for the entire repository walk (`digest.go:33-44`, `watch.go:29-33`),
serialising all concurrent module requests behind one ~2 ms (local M-series; slower on
the CI runner) walk of ~1,000 files, and `isViteClient` additionally reads and parses
`.svelte-kit/skgo-dev-supervisor.json` on every request (`reload.go:22-35`). The old
code's per-request scan was heavier per call (it hashed every file) but ran in parallel.
With six browsers each issuing bursts of ~30 module imports, a serialised scan produces
exactly the uniform 150–300 ms waits the traces show. I have not instrumented the
server, so the mutex is the leading explanation rather than a proven one; the lane-level
numbers do not depend on it.

Impact: the release gate's dev lane is the only CI that exercises the supervisor with a
real browser (the PR's push CI 38534078628 ran "Build, tests, and Conventional Commit"
and the Swift job only). Its failure was a correct signal that `31010eb` roughly
tripled dev page-load time. With a 30 s hydration wait in front of the assertion, that
lane goes green and the regression ships unrecorded, which is what CLAUDE.md's "an exit
code of zero is exactly what a silently skipped test prints" warns about. The worklog
entry explicitly defers this: "Per-request scan latency remains separate #311 work" —
but #311 is about generation speed; a ~2.7× regression in serving every module request
is a new defect introduced by this release, and nothing on the branch or in #311 records
the measured magnitude or the lane durations above.

Acceptable shape: keep the hydration wait (it is correct), but before this is treated as
the fix, either (a) remove the serialisation on the request path (the request path only
needs to know "did anything change since `built`"; a scan that races the poller's scan
can share its result rather than its lock, or the request path can be gated to document
requests as `proxy.go:149` already distinguishes), or (b) at minimum file the regression
with the table above and have the release note say dev page loads are currently slower,
so the gate's silence is a decision rather than an accident.

### 2. nitpick — the worklog correction describes the symptom and omits the measurement that was in the same log

`ephemeral/worklog/warm-dev-supervisor.md` (added line) says the trace had "an import
still pending at the five-second DOM assertion" and prescribes waiting for hydration.
The dev job log it came from also says the regular lane went from 2.3 m to 5.0 m, and
every sibling form scenario went from ~2–3 s to 5.8–8.9 s. A worklog entry that will
"change a future decision" should carry that number; as written it would lead the next
reader to believe the step was merely racy.

### 3. nitpick — no pre-merge browser proof existed for the supervisor in dev mode on CI

The PR's green CI did not include the example BDD lanes; the first browser run of the
supervisor on a CI runner was the release gate. CLAUDE.md asks for `just e2e` green in
both modes on `main` before a release someone cares about. This is process rather than
code, and the local runs did exist, but a local machine without a `525bf42` baseline
cannot see a relative slowdown, which is exactly what was missed.

## Verdict on the question asked

The candidate repairs a real prerequisite (the inbox cannot be counted before kit's
client has started the query) and does so with kit's own marker, consistently with the
rest of the step file. It also masks a product regression: a measured ~2.7× slowdown of
dev-mode page loads introduced by `31010eb`, which this gate run surfaced and which the
30 s hydration wait will hide on the rerun. Merge the step change only together with
either a fix for, or an explicit record of, finding 1.

## Outcome

material findings remain
