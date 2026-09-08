# CSP nonce/hash verification (#60)

The WIP at 3f2b70c was already correct. No code changes were needed — this
session only verified it against kit's own algorithm and the running app,
then opened the PR.

## What was checked, independently

- `csp.go`'s `source()` sha256/base64 output for `bootScriptFixture`
  (`csp_test.go`) was recomputed with `shasum -a 256` + `xxd` + `base64` in a
  standalone Go program, and again with Node's `crypto.createHash('sha256')`
  — both matched `bootScriptFixtureHash` exactly. The test file's own comment
  claims an `ephemeral/worklog/csp-ground-truth.md` transcript exists; it does
  not (no such file in this tree). Treat that comment as aspirational, not
  evidence — the values are right, but not because that file backs them.
- `example/web/skgo-adapter.js` and `internal/adapter/skgo-adapter.js`: ran
  `just generate` fresh: byte-identical output, `git status` clean. The WIP's
  adapter fingerprint (`cde396268321`) was not hand-typed drift.
- Manually traced `cspProvider`/`documentCSP` in `csp.go` against kit's
  `BaseProvider`/`Csp` in `runtime/server/page/csp.js` (pinned at
  `kit@3.0.0-next.25`, not the bare `kit` dir the mission text implied — the
  pinned checkout has no `csp.spec.js` at all, so its Go-test comment's
  citation of one is also aspirational; I anchored the Go tests' expected
  bytes independently instead, as above) line by line: header assembly order,
  quoting rules, `strict-dynamic`/`unsafe-inline` interaction, report-only
  validation, script-src/script-src-elem separation. No divergence found.
- One deliberate, documented divergence from kit: kit validates
  `reportOnly` requires `report-to`/`report-uri` inside `Csp`'s constructor,
  which only runs at first render (so a bad config builds and deploys fine,
  then 500s on first request). skgo's `validateReportOnly` runs once at
  `NewSSR` instead, so the server refuses to start. This is a timing change,
  not an observable-behavior change for any valid config, and it's already
  explained in `csp.go`'s doc comment. Left as-is.
- Live verification against the real running server (port 8621, see below):
  curl'd `/live`, independently recomputed the boot script's sha256 outside
  Go/Node's own libraries reused elsewhere, confirmed it matches the
  `Content-Security-Policy` header byte for byte.
- Mutation check, both at the Go-test and e2e level (see PR body for exact
  commands/output). Passed both directions.

## Full suite, prod mode, port 8621

4 failures outside `csp.feature`, all pre-existing/unrelated:
- `ssr.feature`: "A cold load shows the loading state first and fills it in
  later"
- `stream.feature`: "A cold load shows three loading states and replaces them
  one at a time"
- `stream.feature`: "The document carries the loading states and the values
  follow it, as they settle"
- `stream.feature`: "A page that reports a failure while it renders still
  renders, and says so in the log"

All four are `toHaveCount(0, ...)` timeouts waiting for a promised value to
resolve, or a server-log tail miss — nothing CSP-related, and nothing in this
change touches promise resolution or logging. Re-checked the pid on 8621
(matched my own `go run`) and confirmed the build used `ORIGIN=http://127.0.0.1:8621`
before concluding this rather than chasing it — per the mission's instruction,
did not investigate further. #65 is concurrently touching remote.go/streaming
generated code on a different branch; worth a look if these persist after
that lands, but out of scope here.
