# Adversarial review: skgo standalone devalue v5 runtime migration (round 01)

Date: 2026-10-08 (local), worktree `codex/skgo-devalue-migration`.

## Review target

Authoritative task (from the caller): skgo and its example use released
polytype v1.5.0 and `github.com/tylergannon/devalue/v5` v5.0.0, generated
codecs are regenerated, and SvelteKit remote functions, forms, SSR, streaming,
transport hooks and prerender behavior remain compatible. Validate that the
relevant existing tests are load-bearing and run them, using native pnpm
12.9.1 and Node 24.21.0 for root tests.

The target moved while the review ran. At start the migration was an
uncommitted working-tree diff on `89208d3`. During the review the other
session committed it as `9a2f25b feat: use the standalone devalue v5 runtime`
and then `6783b3b docs: explain the standalone runtime import migration`, and
its dev-mode e2e run left new residue in the working tree. This review covers
the range `89208d3..6783b3b` plus the working tree as observed at 03:59 local.

Operating constraints honored: read-only except this artifact; pinned
toolchain for root tests. No caller instruction narrowed the subject matter,
so nothing was ignored.

## Evidence inspected

- `git diff 89208d3..6783b3b`: 58 files. All but four hunks are the import
  rename `github.com/tylergannon/polytype/devalue` to
  `github.com/tylergannon/devalue/v5`, plus `go.mod`/`go.sum` bumps in both
  modules (polytype v1.4.0 to v1.5.0, new direct require of devalue/v5
  v5.0.0). The non-rename hunks are: `internal/gen/scan.go` (the
  `UndefinedValue` package-path check), `internal/adapter/prerender_predicate_test.go`
  (relay timeout probe rewritten), `document_form_test.go:305` (a comment
  character change), and README.md (+6 lines).
- Module cache: polytype v1.5.0 no longer ships a `devalue` runtime package
  (only `devalue/codegen`, whose `generate.go:114` hardcodes
  `github.com/tylergannon/devalue/v5`) and itself requires devalue/v5 v5.0.0.
  So there is one devalue on the build, matching the project memory that the
  bundled port is gone. The exported API of devalue/v5 v5.0.0 is identical to
  the removed polytype@v1.4.0/devalue package except for one addition,
  `NewArrayBuffer`. Source differences are typed-array/DataView/Float16Array
  support in the flat format and a zero-capacity `ArrayBuffer` identity rule;
  skgo uses none of `ArrayBuffer`, `TypedArray` or `DataView`.
- Kit pin: the example's frozen install resolves devalue 5.9.4; v5.0.0 declares
  parity with 5.9.4 (worklog decision, confirmed by the module's own
  stringify.go comment).
- `go build`, `go vet` on both modules: clean. `go mod tidy -diff` on both
  modules: empty.
- Regeneration: in a scratch checkout of `9a2f25b` with the installed kit pin
  linked in, `go generate ./...` under `example/internal/skgo` produced no
  diff, so the committed codecs are the generator's output, not hand edits.
- Load-bearing check by mutation (scratch copy of devalue/v5 with
  `sentinelUndefined` changed from -1 to -2, wired by a `replace`):
  `TestWireEmitsDevalueBytes`, `TestWireAcceptsDevalueBytes`
  (`devalue_wire_test.go`), `TestKitQueryArgGoldens`
  (`internal/remotearg/kitgolden_test.go`) and
  `TestPrerenderServiceInputsSerializesExplicitUndefined` all fail. Their
  fixtures are bytes recorded from JavaScript devalue 5.9.2 and kit's own
  reducers, not from the Go codec, so they are anchored independently.
- Full suite, this worktree, `just test` with pnpm 12.9.1 and Node 24.21.0
  first on PATH: 23 of 24 packages ok. The one failure is
  `TestTheEmbeddedBuildIsNewerThanTheFrontendSource` in `example`, because
  `example/web/src/app.html` was touched at 03:51:17 by the other session's
  concurrent dev e2e run after the embedded build at 03:47:21. The other
  session's own `ephemeral/skgo-runtime-test-final.log` shows all 24 packages
  ok, same package set, taken before that mtime change.
- e2e, run by the other session on the migrated tree: prod 186 passed, 0
  skipped, 0 flaky (`skgo-runtime-e2e-prod.json`); dev 175 passed
  (`skgo-runtime-e2e-dev.log`). These exercise hydration, enhanced forms,
  streaming, transport classes, remote batching and prerender from kit's own
  client over Go's bytes, which is the compatibility claim in the task.
- Relay failure: `ephemeral/skgo-runtime-baseline-relay.log` shows
  `TestPrerenderPredicateRelayBooleanErrorsAndTerminalLateReply` failing on the
  pre-migration tree with `2 !== 1` / `3 !== 1`. I reproduced it: at `89208d3`
  the test fails 2 of 5 runs under load; at `9a2f25b` it passes 5 of 5.
  Cause in the old probe: the owner answered on a 200ms wall-clock timer
  against a 100ms worker wait. When the worker is descheduled between posting
  and `Atomics.wait`, the late answer sets the cell first, `Atomics.wait`
  returns `not-equal`, the call succeeds, and the relay never goes terminal.
  The rewrite makes the owner withhold its answer until the relay's failure
  message and hands the late reply through a shared cell before the third
  call, so the terminal-failure assertions (`calls === 1`, identical error on
  all three calls, late reply ignored) are kept and the scheduling guess is
  removed. `ephemeral/skgo-runtime-relay-diagnostic.log` records an
  intermediate version that still failed; the committed version is the one I
  ran. This is a pre-existing flake in a test, not a product change, and is
  unrelated to the codec migration.

## Findings

### 1. issue: working tree carries dev-e2e residue that would ship a phantom route

Evidence (observed 03:59 local, after the commits):

- untracked `example/web/src/routes/go-dev-endpoint/server.go` (an endpoint
  the dev scenario "A server route and its Go handler that did not exist at
  launch are served and removed again" creates; `example/e2e/steps/dev.ts:624`);
- modified `example/internal/skgo/links.json`, `example/web/skgo.remotes.json`,
  `example/internal/skgo/skgo_gen.go`, `.../client/skgo_gen.go`,
  `.../params/skgo_gen.go`, all adding `/go-dev-endpoint` and renumbering
  link imports;
- `example/web/src/app.html` mtime newer than `example/web/build`, so
  `just test` in this worktree now fails the freshness guard.

Impact: a checkpoint commit made from this tree would commit a route that
does not belong to the example and generated codecs that reference it, and
the PR's `just test` would fail until `just build` is rerun. The migration
commits themselves are clean; this is state the next commit must not pick up.
Remedy is outside this review: discard the generated-file changes and the
untracked route, rebuild, and check the e2e scenario's cleanup if it did not
run on a passing scenario.

### 2. nitpick: a kit-source citation in a comment was corrupted

`document_form_test.go:305`: the comment quoting kit's
`runtime/app/server/remote/form.js:163` changed from
`... : '')` to `... : ”)`. The surrounding paragraph exists to say the fixture
is kit's literal bytes; the quoted source should stay verbatim. Looks like a
quote-mangling tool edit, not intent.

### 3. nitpick: the feat commit bundles an unrelated test de-flake without saying so

`9a2f25b` rewrites the relay timeout probe (see evidence above) but its
message mentions only the runtime migration. The worklog's `friction` entry
explains it well; the squash-merge body should carry that sentence so the
change is findable from history.

### 4. nitpick: the worklog's pnpm instruction does not say where the binary is

The worklog says to put "the installed native pnpm 12.9.1 package directory
first on PATH" but names no path. It is
`~/Library/pnpm/package-manager-store/v11/links/@pnpm/exe.darwin-arm64/12.9.1/<hash>/node_modules/@pnpm/exe.darwin-arm64/pnpm`,
and the homebrew `pnpm` shim reports 12.10.1 outside the example. A
reproducing reader has to rediscover this.

No defects found in the migration itself: import paths are complete (no
`polytype/devalue` references remain outside `ephemeral/` and the codegen
import), both modules tidy, generated output matches the generator, the wire
anchors are independently fixtured and fail under mutation, the Go suite is
green on a clean tree, and both e2e modes pass on the migrated tree.

## Outcome

material findings remain
