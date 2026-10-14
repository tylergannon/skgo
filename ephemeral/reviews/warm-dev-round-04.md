# Adversarial review: warm generation worker + shared dev supervisor, round 04

Date: 2026-10-14 (local). Reviewer: Claude (Fable 5.1), read-only except this file.

## Review target

- skgo branch `codex/warm-dev-supervisor` at `b9cfabe` (unchanged since round 03; working tree
  clean apart from the untracked round-02/03 reviews). PR #315 head is `b9cfabe`; its body now
  states that ambiguous legacy route-local files with the generic Polytype header are preserved
  on upgrade and may need one-time manual removal, and still says "Closes #313. Advances #311".
- CI: push run 38534078628 on `b9cfabe` completed with conclusion `success` for both jobs
  ("Build, tests, and Conventional Commit" 8m29s, "Swift client and generated production HTTP
  calls" 5m39s). The two "skipping" rows belong to the duplicate PR-event run 38534084217, not
  to a skipped step; `.github/workflows/ci.yml` runs `just install`, `just build`, `just vet`,
  `just test` and the `-race` native subsets.
- Authoritative sources as in rounds 01-03: issue #311, issue #313, CLAUDE.md, and the delivery
  context (publish bounded gains, keep #311 open, skgo release before Gimbal pins it).
- Gimbal: `/Users/tyler/src/gimbal-worktrees/gimbal-warm-dev`, now at `c0aeabef`
  "fix: separate runtime registry from projection analysis", working tree clean.

No scope-narrowing instruction was given. The caller's "additional evidence" was treated as
material to verify; the harness sources were read in full.

## Evidence inspected

- `/tmp/gimbal-browser-final.cjs` (older harness) and `/tmp/gimbal-registry-api.cjs` (newer):
  both register `page.on('requestfailed')` and `page.on('pageerror')` loggers before any
  navigation, edit the real `web/src/routes/workflows/page.server.go` and `+page.svelte`, wait
  for literal strings ("BODY UPDATED", "API FIELD LITERAL", "gimbalProofBroken", "API REPAIRED
  LITERAL"), assert the generated `types.ts` contains `badge: string`, count matched rows, and
  restore both files in `finally`. The expectations are literals supplied by the script, not
  read back from the system under test, so they are load-bearing.
- `/tmp/gimbal-registry-api.log`: `HYDRATED 8`, `api_field` 9.39 s with `items: 8`, and no
  `FAILED` or `PAGEERROR` line. `/tmp/gimbal-browser-final.log` (older run): `FAILED
  http://127.0.0.1:48741/workflows net::ERR_ABORTED` once after each of `go_body`, `api_field`
  and `repair`, none after `frontend_hmr` or `go_error`.
- Pinned Kit 3.0.0 `src/exports/vite/dev/index.js:100-125, 240-252`: Kit itself sends
  `full-reload` only on manifest-error recovery and on `app.html` changes.
- `internal/adapter/skgo-adapter/env.js:1296-1313`: the supervisor's own status becomes either
  a Vite `error` payload (diagnostic) or a `full-reload`.
- `internal/dev/server.go:656-674`: a request held in `awaitSettled` returns `nil` when its
  context is cancelled, after which `ServeHTTP` writes a 503 to a connection the browser has
  already dropped. No goroutine or counter is leaked by that path (`requests.Add` happens only
  when a generation is returned).
- Everything inspected in rounds 01-03 remains as recorded there; the Polytype v1.9.0 pin,
  `--debug`/DWARF implementation and tests, `hasSettled`, `closeStreams`, digest reuse and
  prune ownership were not changed by any commit since.

## Findings

### 1. nitpick — the older Gimbal run's aborted navigations are consistent with a benign double reload, still unconfirmed

The pattern in `/tmp/gimbal-browser-final.log` is exactly the set of cases in which the
supervisor sends `full-reload` (success after a Go edit) and excludes the case in which it sends
`error` (`go_error`), and the aborted URL is the document itself, which Playwright reports only
when a navigation is superseded. That matches the caller's hypothesis: the regenerated stubs
and `types.ts` reach Vite's watcher during generation and Vite triggers its own page reload,
whose document request is then held by `awaitSettled`; when the supervisor's `full-reload`
arrives after the rebuild, the browser abandons the held navigation (`ERR_ABORTED`) and starts
the one that succeeds. Kit's own code (`dev/index.js:119, 248`) does not emit that first
reload, so the source would be Vite's HMR for a module with no accepting boundary; I could not
confirm that from the pinned source in this round, and no skgo test records the sequence of
Vite messages across a Go edit against a real Vite. The newer harness shows no failed request
for the `api_field` case, but it covers only that case. Impact: a wasted navigation and a
longer spinner per Go edit, no incorrect page; the user-visible consequence of a double reload
is latency, which is explicitly left to #311. The Gimbal owner's planned WebSocket-frame /
request-timeline check in final qualification is the right place to establish the cause.

### 2. nitpick — `cmd/skgo` still exercises only `--debug` at the CLI (carried from round 03)

Unchanged; proven elsewhere (`TestDevBuildRetainsSymbolsAndOffersDWARF`, plugin experiment).

## Requirement checks

- #313: supported as in round 03; the legacy-header caveat is now in the PR description.
- #311: PR claims advancement only; no sub-second claim anywhere in the PR, worklog or logs.
- DWARF / `--debug`: met (round 02/03 evidence unchanged, CI green on the same SHA).
- CLAUDE.md evidence triad: `just test` green in CI and locally in order; browser evidence for
  the three dev scenarios on this branch; screenshot looked at in round 03. The Gimbal half of
  "same technology" is deferred to the post-release pin by the stated delivery sequence.

## Outcome

only nitpicks remain
