# Adversarial review: native Go–Zig binary exchange, round 03

Date: 2026-10-08. Branch `codex/native-binary-exchange`, PR #290 (open, draft).
Reviewed at head `4484efd` (local HEAD, remote head and PR head all agree).
Read-only except this file. Working tree carries two untracked files only:
`ephemeral/native-binary-review-prompt-03.txt` and the zero-byte
`ephemeral/native-binary-review-run-03.json`.

Instruction precedence is as round 02 stated: caller's rule that review
prompts/results and raw session artifacts are tracked under `ephemeral/`,
then `CLAUDE.md`, then `agent-protocol` and `proof-of-work`. No narrowing was
requested in the round 03 prompt and none was ignored.

## Target

Implementation diff against merge-base `15b8015` (origin/main):
`native/core/src/root.zig` (guard admits `array_buffer`, `typed_array`,
`data_view`; doc comment), `native/core/build.zig.zon` (devalue pin moved to
commit `f4c94b9a…`, `README.md` added to `.paths`),
`native/core/src/binary_tests.zig` and `native/core/src/kit-binary.json` (new),
`native/core/src/tests.zig` (imports the new file), `native/binary_test.go`
(new), `native/remote_test.go` (one line, runs the binary subtest),
`native/core/README.md` (new). `native/swift/` is byte-identical to main.
Everything else in the diff is under `ephemeral/`.

Commits after the last implementation change `3877011` touch `ephemeral/`
only (`git show --stat 4484efd`), so the transcripts and CI recorded at
`3877011` describe the same `native/` tree as the head.

## Evidence inspected

- Spec `ephemeral/native-binary-exchange.md`, `CLAUDE.md`, worklog, both
  prior review rounds and their run JSON files, PR #290 body, commits and
  checks.
- Kit 3.0.0 pinned source `example/web/node_modules/@sveltejs/kit/src/runtime/shared.js`:
  `to_sorted` (line 75) clones only what `__skrao` hands it, and `__skrao`
  (line 157) returns `undefined` for anything that is not a plain object, so
  arrays, ArrayBuffer, typed arrays and DataView reach devalue unchanged in
  both the sorted query path and the unsorted command path. `root.zig:84–102`
  clones only `.object` nodes and leaves every other ref as is. Same
  delegation.
- Pins. `build.zig.zon` URL commit `f4c94b9a…` is what the `v5.0.0` tag of
  `tylergannon/devalue` resolves to (`gh api …/git/ref/tags/v5.0.0`).
  `go.mod` consumes `devalue/v5 v5.0.0`, whose `UpstreamVersion` is `5.9.4`.
  The fetched Zig package under `native/core/zig-pkg/` carries the hash in
  the zon file and exports `typedArray`, `dataView`, `uint8ArrayCopy`,
  `viewBytes`, `equal` and `upstream_version = "5.9.4"`, so every API the
  README and tests name exists at the pin.
- Fixture re-derived a third time, from the JS shape written above the Zig
  fixture struct, with a scratch script outside the repo importing the
  installed `shared.js` and the devalue 5.9.4 beside it. Node 26.11.0 this
  time (rounds 01 and 02 used 24.21.0). `input`, `query`, `command`,
  `result`, `envelope`: all MATCH.
- Runs in this worktree at `4484efd`:
  - `zig build test` with a fresh `--cache-dir`, Debug: 13/13 pass.
    ReleaseSafe, fresh cache: 13/13 pass (11 core, 2 ABI, not served from
    cache).
  - `go vet ./native/`: clean.
  - `go test -count=1 -race -run TestZigRemoteCalls -v ./native`: PASS;
    `binary/query` and `binary/command` pass over loopback HTTP against
    `skgo.NewRemotes`.
- Four mutations new to this round, each on a detached scratch worktree of
  `4484efd` and reverted:
  - A. Go reply omits the `self` cycle: both subtests fail at
    `binary_test.go:149`; the CLI's decoded document lacks `"self":0`.
  - B. Go reply reuses the `view` pointer for `equal` (identity collapse):
    both subtests fail at the same line; the document shows `"equal":1`.
    Zig's response path preserves identity exactly as sent.
  - C. Zig guard admits every node kind (`else => {}`): the pre-existing
    `unsupported argument nodes…` test fails, 12/13. Widening the admitted set
    beyond Kit's is still caught.
  - D. Zig guard admits typed arrays but not DataView: the byte-match and
    allocation-failure tests fail with `UnsupportedValue`, 11/13.
  Rounds 01 and 02 already showed the echoed-reply, array-buffer-only,
  flipped-fixture-byte and CLI-bypassing-`receive` mutations fail.
- CI. Push run for `3877011` succeeded on both jobs. Push run
  `37778940477` for head `4484efd` completed during this review: `Build,
  tests, and Conventional Commit` success, `Swift client and generated
  production HTTP calls` success. The pull_request-event runs are skipped by
  design (`ci.yml` `if:` gates same-repo PRs to the push event), which is why
  the PR shows two skipped checks beside the green ones.
- PR body. Validation is now anchored at `4484efd`, and all six artifact
  links resolve (HTTP 200) on the PR branch's history. The body names the
  proved SHA, links the post-rebase transcript, both review rounds and both
  review-run files, and no longer says "not yet merge-ready". Its statement
  that CI passed at `3877011` with ephemeral-only changes after is accurate.
- Round 02 findings, status at head:
  1. Evidence anchored to an unreachable commit: resolved in `4484efd`.
  2. Raw session artifacts deleted: all four restored in `4484efd`
     (`baseline`, `initial-build`, `vet`, `native-binary-pr.md`).
  3. Round 02 run JSON empty and untracked: committed with content in
     `4484efd` (1856 bytes).
  4. `just test` needs the frozen pnpm first in PATH: observation, unchanged,
     not this PR's.

## Spec coverage

| Requirement | Where | Status |
| --- | --- | --- |
| Loopback HTTP to production dispatcher | `binary_test.go:136–143`, `skgo.NewRemotes` | met |
| Go inspects bytes, geometry, repeated vs distinct identity, shared buffer, distinct empties, cycle | `checkBinaryGraph`, literal byte arrays | met; mutations A and B load-bearing |
| Independently constructed binary reply | `binary_test.go:133`; round 02 mutation | met |
| Zig's actual response path decodes the reply | `cli.zig:31` `core.receive`; round 02 mutation | met |
| Query and command bytes match Kit 3.0.0 / devalue 5.9.4 | `binary_tests.zig:31–47`; fixture re-derived three times on two Node majors | met |
| Zig content and identity after response storage released | `binary_tests.zig:52–54` frees input before every assertion | met |
| Allocator failure coverage | `binary_tests.zig:103–120` over prepare (both kinds) and receive | met |
| Bounded to existing consumer; Swift JSON profile unchanged | `native/swift` identical to main; no harness or transport added | met |
| Passing repository checks | CI green at head `4484efd`, both jobs | met |
| Independent review | rounds 01, 02, 03 | this file |
| Merged PR, synchronized main, task cleanup | PR still draft | pending, not a defect |

## Findings

No material findings remain. The implementation mirrors Kit's delegation
exactly, the pins agree on both sides, the fixture is upstream bytes, every
spec claim has a test that fails under a targeted mutation, and CI is green at
the head the PR body names.

### 1. nitpick — round 03's prompt and result pair are not yet in the tree

`ephemeral/native-binary-review-prompt-03.txt` is untracked and
`ephemeral/native-binary-review-run-03.json` is an untracked zero-byte
placeholder, the same state round 02's pair was in before `4484efd`. Under
the caller's tracked-ephemeral rule both belong in a checkpoint commit once
this run's result is written, and the PR body's review list should gain this
round and its run file by the new head SHA. This is the expected
post-review step, noted so it is not forgotten.

### 2. nitpick — PR body could cite CI at the head it names

The body states CI passed at `3877011` and that later commits are
ephemeral-only. Both are true, but the push run for `4484efd` is now green
too, and `proof-of-work` asks the closeout to name the proved head's checks.
One sentence citing run `37778940477` (or whichever run covers the final
head) removes the reader's need to reason about which commits changed
`native/`.

## Outcome

only nitpicks remain
