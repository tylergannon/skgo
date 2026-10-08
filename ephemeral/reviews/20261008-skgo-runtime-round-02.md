# Adversarial review: skgo standalone devalue v5 runtime migration (round 02)

Date: 2026-10-08 (local), worktree `codex/skgo-devalue-migration`, HEAD
`108f747 docs: preserve the Kit citation and record validation traps`.

## Review target

Same authoritative task as round 01: skgo and its example use released
polytype v1.5.0 and `github.com/tylergannon/devalue/v5` v5.0.0, generated
codecs are regenerated, and SvelteKit remote functions, forms, SSR,
streaming, transport hooks and prerender behavior remain compatible. The
relevant existing tests must be load-bearing and must run, with native pnpm
12.9.1 and Node 24.21.0 selected for root tests.

Range reviewed: `main..108f747` (three commits: `9a2f25b` feat, `6783b3b`
docs, `108f747` docs) plus the working tree as observed at the start of this
round. The target did not move during this round.

Operating constraints honored: read-only except this artifact. Mutation and
regeneration checks ran in scratch copies of HEAD under the session
scratchpad, never in the worktree.

## What changed since round 01

- `108f747` restores the mangled quote in the `document_form_test.go:305`
  comment (round 01 finding 2) and adds two worklog entries: the dev-mode
  e2e suite's temporary route and generated-file churn is transient and
  self-restoring (round 01 finding 1), and the exact native pnpm and Node
  paths used for root tests (round 01 finding 4).
- The working tree no longer carries the dev-e2e residue: `git status` shows
  only untracked `ephemeral/` evidence files. No `go-dev-endpoint` route, no
  modified generated codecs, and `example/web/build` (04:01:19) is newer than
  `example/web/src/app.html` (03:59:21), so the embedded-build freshness
  guard passes again.
- `ephemeral/skgo-runtime-pr.md` (the squash body draft) now names the relay
  probe rewrite and why (round 01 finding 3).

## Evidence inspected this round

- Branch diff: 58 files, 90 insertions, 66 deletions. Non-rename hunks are
  unchanged from round 01: `internal/gen/scan.go:682` (the `UndefinedValue`
  package-path check now names `github.com/tylergannon/devalue/v5`),
  `internal/adapter/prerender_predicate_test.go` (relay probe handshake),
  README.md (+6 lines), and the comment fix.
- Leftover references: `grep -rn polytype/devalue` outside `ephemeral/` finds
  only the two `devalue/codegen` imports in `internal/gen/{codecs,clients}.go`
  (correct: polytype v1.5.0 still ships codegen at that path and its
  `generate.go:114` hardcodes `github.com/tylergannon/devalue/v5` as the
  runtime) and the README migration note. New-app templates under
  `internal/newapp` carry no direct devalue or polytype pin.
- Module graph: `go.mod` and `example/go.mod` both require devalue/v5 v5.0.0
  and polytype v1.5.0; the polytype module cache has no `devalue` runtime
  package (only `devalue/codegen`) and itself requires devalue/v5 v5.0.0.
  One devalue on the build.
- `go build`, `go vet`, `go mod tidy -diff` on both modules: clean, empty.
- Regeneration: in a scratch extraction of HEAD with the installed kit pin
  linked in, `go generate ./...` under `example/internal/skgo` exited 0 and
  `diff -rq` against the committed `example/internal/skgo`, `example/web/src`
  and `example/web/skgo.remotes.json` reported no differences. Committed
  codecs are the generator's output and import `devalue/v5`.
- Load-bearing by mutation, scratch copy, devalue/v5 `sentinelUndefined`
  changed from -1 to -2 via `replace` in both modules:
  - root: `TestWireEmitsDevalueBytes`, `TestWireAcceptsDevalueBytes`,
    `TestPrerenderServiceInputsSerializesExplicitUndefined` fail;
  - `internal/remotearg`: `TestKitQueryArgGoldens` fails;
  - example: `TestATransportedValueReachesTheEngineWithItsType`,
    `TestARemoteAnswersTransportedValueIsRenderedByItsOwnMethod`,
    `TestATransportedArgumentReachesGoAsItsOwnType`,
    `TestTheTransformedStreamedShellArrivesBeforeAnyDeferredValueAndTheChunksAreUntouched`,
    `TestTheStreamDataResponseIsNotTransformedButIsMarkedByTheMiddleware`
    fail. These cover transport hooks, SSR of transported values and
    streaming at the real example handler, which round 01 did not mutate.
  - `example/devrender` also failed under the mutation but at `vp dev`
    startup in the scratch copy; treated as scratch-environment noise, not
    signal.
- Load-bearing by mutation, scratch copy, `scan.go:682` package path reverted
  to `github.com/tylergannon/polytype/devalue`:
  `TestNoArgumentPrerenderInputsCompileWithoutFmtImport` and
  `TestPrerenderInputsGenerateGoProducerAndThrowingStubs` in `internal/gen`
  fail with the generator's own "requires func() ([]devalue.UndefinedValue,
  error)" error. The one hand-written code change in the migration is
  covered.
- Relay probe: `TestPrerenderPredicateRelayBooleanErrorsAndTerminalLateReply`
  run 10 times in isolation with Node 24.21.0: 10 of 10 pass. Read against
  `skgo-adapter/prerender.js:938-949`: the worker drains the reply with
  `receiveMessageOnPort` after `Atomics.wait`, and `predicateFailure` makes
  every later call throw the recorded error without posting. In the probe's
  `timeout` mode the owner posts the stale reply and sets the shared cell only
  on receipt of the relay's `failure` message, and the worker waits on that
  cell before its third call, so the "late reply ignored" assertion is
  reached with the reply actually enqueued on the worker's port rather than
  by a timer race. The `calls === 1` and identical-error assertions are
  intact.
- Full suite, this worktree, `just test` with
  `.../links/@/pnpm/12.9.1/<hash>/node_modules/pnpm` and
  `~/.local/share/mise/installs/node/24.21.0/bin` first on PATH (verified
  `pnpm --version` 12.9.1, `node --version` v24.21.0): 24 of 24 packages ok,
  zero FAIL lines. Package timings are ordinary (longest `cmd/skgo` 65s).
- e2e evidence left by the implementing session: `skgo-runtime-e2e-prod.json`
  186 expected, 0 unexpected, 0 skipped, 0 flaky, 75s;
  `skgo-runtime-e2e-dev.json` 186 expected, 0 unexpected, 0 skipped, 0 flaky,
  447s. The dev log's 175 count seen in round 01 was a mid-run snapshot;
  the complete run reports 186. Both logs show the recipes' real commands
  (`vp run --no-cache test --add-reporter=json`, `ORIGIN=... vp build`).

## Findings

### 1. nitpick: the "preserved" Kit citation points at the wrong file in the pin

`document_form_test.go:304-306` cites
`runtime/app/server/remote/form.js:163` for
`action_id = id + (key != undefined ? \`/${JSON.stringify(key)}\` : '')`.
In the installed kit 3.0.0 pin that line is
`runtime/client/remote-functions/form.svelte.js:80`; the server-side
`form.js:162` builds the same string as
`__.key ? \`${__.id}/${encodeURIComponent(__.key)}\` : __.id` with
`__.key = JSON.stringify(key)` (line 256). The fixture `sendMessage/%22k1%22`
is still exactly what kit's client sends, so the test is sound. The comment
has been wrong since `4974f4c` (2026-09-07) and is not introduced by this
branch, but `108f747` is titled "preserve the Kit citation" and restored a
citation that does not resolve. Fix is a one-line path change when the file
is next touched.

### 2. nitpick: e2e evidence files carry no commit identity

All of `ephemeral/skgo-runtime-*.{json,log}` share one mtime (04:03:48) and
none records the SHA or tree state it ran against. Round 01 observed the dev
run in progress on the migrated tree at 03:51, after `9a2f25b`, and the
counts and zero-skip totals are consistent with a complete run on that tree,
so the claim is credited. A future run that writes `git rev-parse HEAD` into
its log would make this independent of a reviewer having watched it.

No defects found in the migration. Round 01 findings 1, 3 and 4 are
resolved; finding 2 is resolved as written but exposed the stale citation
above. Import paths are complete, both modules tidy, generated output matches
the generator, the wire and transport anchors are independently fixtured and
fail under mutation in both the root and example modules, the one hand-written
generator change fails under mutation, the relay probe is stable over ten
runs, the Go suite is green on a clean tree under the pinned toolchain, and
both e2e modes report 186 passed with zero skips.

## Outcome

no material findings remain
