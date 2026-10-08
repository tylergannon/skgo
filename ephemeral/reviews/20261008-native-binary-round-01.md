# Adversarial review: native Go–Zig binary exchange, round 01

Date: 2026-10-08. Branch `codex/native-binary-exchange`, PR #290 (open draft).
Caller constraints honored: read-only except this file; log directory
`ephemeral/reviewer-logs/native-binary-round-01` is empty. No caller narrowing
of scope was requested or ignored.

## Target

Working tree of the worktree against `ephemeral/native-binary-exchange.md` and
`CLAUDE.md`. The working tree's `native/` files are byte-identical to the PR
branch head `c5a1241` (`git diff origin/codex/native-binary-exchange -- native`
is empty).

Changed files: `native/core/build.zig.zon` (devalue pin), `native/core/src/root.zig`
(guard admits `array_buffer`, `typed_array`, `data_view`), `native/core/src/tests.zig`
(imports new test file), `native/core/src/binary_tests.zig` (new),
`native/core/src/kit-binary.json` (new fixture), `native/binary_test.go` (new),
`native/remote_test.go` (runs the binary subtest), `native/swift/README.md`,
plus eight files under `ephemeral/`.

## Evidence inspected

- Spec `ephemeral/native-binary-exchange.md`, worklog, PR #290 body and commits.
- Kit 3.0.0 pinned source `example/web/node_modules/@sveltejs/kit/src/runtime/shared.js`
  lines 95–268: `create_remote_arg_reducers` returns `undefined` from
  `__skrao` for non-plain objects, so ArrayBuffer/typed arrays/DataView fall
  through to devalue's own encoding. The Zig `Canonical.reduce` returns `null`
  for non-object nodes and the guard now admits the three binary node kinds,
  which is the same delegation.
- devalue under Kit resolves to `example/web/node_modules/.pnpm/devalue@5.9.4`.
- Zig pin: GitHub tag `v5.0.0` of `tylergannon/devalue` resolves to commit
  `f4c94b9a…`, the URL in `build.zig.zon`; the fetched package directory under
  `native/core/zig-pkg/` carries the new hash. Go consumes
  `github.com/tylergannon/devalue/v5 v5.0.0` (go.mod line 8). Same release on
  both sides.
- Zig devalue API at the pin: `typedArray(kind, buffer, byte_offset, length)`
  takes length in elements, `dataView` in bytes, `viewBytes` returns visible
  bytes, `equal` compares handles. Go devalue: `NewArrayBuffer` allocates with
  capacity ≥ 1 so owned empty buffers have distinct pointers, and the parser
  routes every decoded empty buffer through `NewArrayBuffer(nil)`. The Go
  empty-buffer identity assertions in `checkBinaryGraph` are therefore
  load-bearing, not trivially true.
- Independent fixture re-derivation. I built the same JS graphs in Node
  24.21.0 (Float16Array present) and ran the installed Kit 3.0.0
  `stringify_remote_arg` / `stringify_command_arg` and devalue 5.9.4
  `stringify` against `kit-binary.json`. All five fields matched exactly:
  `input`, `query`, `command`, `result`, and `envelope` (`{type:"result",
  data: stringify({_: reply})}`). The fixture is upstream bytes, not codec
  output.
- Runs performed here:
  - `mise exec -- zig build test --summary all` in `native/core`: 13/13 pass
    (11 core including the 3 new binary tests, 2 ABI).
  - `go vet ./native/`: clean.
  - `go test -count=1 -race -run TestZigRemoteCalls -v ./native`: PASS, with
    `binary/query` and `binary/command` subtests both passing over real
    loopback HTTP against `skgo.NewRemotes`.
  - Mutation on a scratch copy: reverting the guard to `.object, .array` makes
    `binary query and command arguments match pinned Kit bytes` and the
    allocation-failure test fail with `UnsupportedValue`. The Go HTTP test would
    fail on the same mutation because the CLI exits non-zero before fetching.
- Spec requirements traced:
  - Real loopback HTTP to production dispatcher: `binary_test.go:136–143`.
  - Go inspects backing bytes, geometry, repeated vs distinct view identity,
    shared buffer, distinct empty buffers, cycle: `checkBinaryGraph`
    lines 52–98, anchored on literal byte arrays supplied by the test.
  - Independent reply with different bytes: line 133.
  - Zig's actual response path decodes the reply: `cli.zig:31` calls
    `core.receive`, output asserted equal to upstream `result` at
    `binary_test.go:148`, then re-parsed in Go and inspected again.
  - Query and command arguments match pinned Kit bytes: `binary_tests.zig:31–47`
    for both kinds against the recorded base64 payloads.
  - Graph content and identity after response storage is released:
    `binary_tests.zig:52–54` frees the envelope bytes before every assertion.
  - Allocator failure coverage: `checkAllAllocationFailures` over prepare
    (both kinds) and receive, lines 103–120.
  - Swift JSON profile unchanged: `abi.zig` keeps its own finite-model walker
    and still rejects binary nodes; no change in the diff.
  - Bounded to the existing consumer: no new transport, no harness, no
    `.mjs` in the tree.

## Findings

### 1. issue — evidence transcripts and a status document are committed under `ephemeral/`

Files: `ephemeral/native-binary-baseline.txt`, `native-binary-final-tests.txt`,
`native-binary-initial-build.txt`, `native-binary-focused.txt`,
`native-binary-vet.txt`, `native-binary-review-prompt.txt`,
`native-binary-pr.md` (all in PR #290), and the untracked zero-byte
`native-binary-review-run.json`.

`CLAUDE.md` "Build the software": not "proof directories, or evidence
manifests"; and "Don't narrate": no status documents or progress reports, the
worklog carries only actionable intelligence. The PR body links two of these
transcripts as its validation, which is exactly an evidence manifest.
`initial-build.txt` records a transient compile error that was fixed in the
same session, `review-prompt.txt` is the slash command that launched this
review, `vet.txt` is one line, and `native-binary-pr.md` duplicates the PR
body as a tracked status document. None of these changes a future decision.
`review-run.json` is empty.

Impact: repository instruction violation on merge; every future agent that
greps `ephemeral/` reads a stale "draft is not yet merge-ready" document.
Remove them before merge; the spec file and the worklog are the only two
`ephemeral/` additions this slice needs. The `go test` and `go vet` runs are
reproducible from the Justfile in seconds.

### 2. nitpick — the binary paragraph is documented in the Swift client README

`native/swift/README.md:43–49` describes the generic Zig graph API, the
`skgo-remote` CLI, and the codec's `uint8ArrayCopy`, none of which a Swift
reader can reach through the C ABI the same file documents. The final
sentence restating that the Swift JSON API keeps its finite contract is the
only line relevant to that file. The Zig-facing text belongs beside the Zig
core (a doc comment on `prepare` already carries half of it at
`root.zig:33–35`).

### 3. nitpick — fixture provenance is a worklog sentence, not a recorded input

`kit-binary.json` was produced by an inline Node command that was not kept,
and the worklog names Node 26.8.1 while Node 24.21.0 in this checkout
reproduces it exactly. This follows the precedent of `kit-arguments.json`, and
`CLAUDE.md` forbids `.mjs` harnesses, so the tension is by design. It would
cost nothing to state the JS graph shape in the fixture comment at
`binary_tests.zig:6–7` (the Go `binaryGraph` already is that shape) so the
next person can re-derive it the way this review did.

## Not findings, checked and cleared

- Query canonicalization with cycles and shared views: Zig output matches
  Kit's sorted clone (`self` points at the clone, `separateEmpty` hoisted to
  ref 21) byte for byte.
- Twelve typed-array kinds including Float16Array round-trip through Go's
  production `internal/remotearg` parsing on both the URL payload and the
  JSON command body.
- `TestZigRemoteCalls/binary` nests inside the existing test so the Zig
  toolchain builds once per package, and a missing `mise`/`zig` fails rather
  than skips (`remote_test.go:24–28`).
- `ctx` 30 s budget is shared across the binary subtests and the original
  steps; measured total was under 1 s for the HTTP portion.

## Outcome

material findings remain
