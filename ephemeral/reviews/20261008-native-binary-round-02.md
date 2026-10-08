# Adversarial review: native Go–Zig binary exchange, round 02

Date: 2026-10-08. Branch `codex/native-binary-exchange`, PR #290 (open draft).
Read-only except this file. The branch moved during the review: it started at
`b784ca9` with four uncommitted changes and ended at `3877011` (pushed), which
committed exactly those changes plus the rebased test transcript and this
round's prompt. Everything below is stated against `3877011` unless noted.

## Instruction precedence applied

1. The caller's instruction for this review: review prompts/results and every
   raw session artifact live under `ephemeral/`; proof artifacts must be
   inspectable and linked from the PR.
2. `CLAUDE.md`: "Build the software … not proof directories or evidence
   manifests"; "Don't narrate".
3. Skills: `agent-protocol` (ephemeral is tracked working material; checkpoint
   commits; proof protocol before merge-ready) and `proof-of-work` (lead with
   claims; upload artifacts, cite no local paths, verify links; closeout names
   the proved head SHA).

Under that order, `CLAUDE.md`'s prohibition governs the software tree: no
proof runners, no acceptance scripts, no evidence directories beside `native/`.
It does not reach raw session artifacts in `ephemeral/`, which the caller and
`agent-protocol` both say are tracked there. Round 01's finding 1 read
`CLAUDE.md` as reaching `ephemeral/` and asked for the transcripts to be
removed. That reading is overruled by the higher-precedence instruction, and
the branch acted on it (see finding 2).

## What was checked

- Spec `ephemeral/native-binary-exchange.md`; `CLAUDE.md`; worklog; PR #290
  body; the two review prompts; `native-binary-review-run.json`.
- Kit 3.0.0 pinned source `example/web/node_modules/@sveltejs/kit/src/runtime/shared.js`:
  `create_remote_arg_reducers` (line 104) returns `undefined` from `__skrao`
  for anything that is not a plain object, so binary values reach devalue
  itself; `stringify_remote_arg` (261) sorts, `stringify_command_arg` (275)
  does not and uses `stringifyAsync`. The Zig guard in `root.zig:68–74`
  admits `array_buffer`, `typed_array`, `data_view` and still rejects every
  other node kind; `Canonical.reduce` is unchanged. Same delegation as Kit.
- Fixture independently re-derived, not taken from round 01. A scratch script
  outside the repo imported the installed `shared.js` and the devalue 5.9.4
  resolved beside it (Node 24.21.0), built the two graphs from the shape
  stated in the `binary_tests.zig:6–12` comment, and compared all five
  fixture fields. `input`, `query`, `command`, `result`, `envelope`: all MATCH.
- Pins: `build.zig.zon` URL is commit `f4c94b9a…` (the `v5.0.0` tag of
  `tylergannon/devalue`); `go.mod` consumes `devalue/v5 v5.0.0`.
- Runs in this worktree:
  - `zig build test` with a cold cache, Debug: 13/13 pass. ReleaseSafe:
    13/13 pass. The PR body claims ReleaseSafe without an artifact; the claim
    is true.
  - `go vet ./native/`: clean. `just vet`: exit 0.
  - `go test -count=1 -race -run TestZigRemoteCalls -v ./native`: PASS, with
    `binary/query` and `binary/command` both passing over loopback HTTP
    against `skgo.NewRemotes`.
  - `just test`: FAILS in this shell, in `internal/kitpatch` and `cmd/skgo`
    only, every failure being `native pnpm version = "12.10.1"; want 12.9.1`.
    My PATH has Homebrew pnpm 12.10.1; the branch's transcript (recorded with
    the frozen 12.9.1 first in PATH, as the worklog says) and CI both pass.
    Not a defect of this PR; see observation 4.
- Mutations on a detached scratch worktree of the same tree, each reverted:
  - Go handler returns `call.Arg` instead of a fresh graph: both subtests
    fail at `binary_test.go:149` ("Zig response … want upstream"). The
    reply-is-not-an-echo claim is load-bearing. (Incidentally the echoed query
    argument comes back key-sorted, showing Go received Kit's canonical clone.)
  - Guard admits `array_buffer` only: the Zig byte-match test and the
    allocation-failure test fail with `UnsupportedValue`; 11/13.
  - One byte flipped in the fixture's `result` buffer: Go test fails.
  - CLI prints the raw HTTP body instead of `core.receive`'s decoded value:
    both subtests fail. Zig's real response path is what the Go test proves.
- Round 01's two nitpicks are resolved in `3877011`: the binary paragraph now
  lives in `native/core/README.md` (and `build.zig.zon` ships it), and the
  fixture's JS graph shape is written above the Zig fixture struct.

## Findings

### 1. issue — PR evidence is anchored to a commit no branch reaches

PR body: "Validation at `76a6ebd6…`" with two `blob/76a6ebd6…/ephemeral/…`
links. The branch was rebased onto `15b8015` and force-pushed; `git branch -r
--contains 76a6ebd` is empty. GitHub still serves the blobs today (both
links return 200) but they are unreferenced objects, not inspectable
artifacts on the PR's history, and the SHA the body names as proved is not an
ancestor of the PR head. `proof-of-work` requires the proved head SHA and
verified links; `agent-protocol` wants the body usable as the squash message.

Also missing from the body: the post-rebase transcript
`ephemeral/native-binary-rebased-tests.txt` (which is the run that actually
corresponds to the PR head), the round 01 review, this review, and the
review-run JSON files. The body still says "not yet merge-ready".

Fix: restate validation at `3877011` (or whatever head is pushed last), link
`native-binary-rebased-tests.txt`, `native-binary-focused.txt`, the two review
files and the two review-run files by that SHA, and drop the "in progress"
line once round 02 is addressed.

### 2. issue — raw session artifacts were deleted on round 01's advice

Commit `b784ca9` ("trim redundant artifacts") removed
`ephemeral/native-binary-baseline.txt`, `native-binary-initial-build.txt`,
`native-binary-vet.txt` and `native-binary-pr.md`. Under the precedence above,
every raw session artifact belongs under `ephemeral/`; the deletion followed a
lower-precedence reading. The files still exist as objects in this repository
(tree of `c5a1241`, itself now unreachable from any ref) and should be
restored before they are garbage-collected:

```sh
git checkout c5a1241 -- ephemeral/native-binary-baseline.txt \
  ephemeral/native-binary-initial-build.txt \
  ephemeral/native-binary-vet.txt ephemeral/native-binary-pr.md
```

`native-binary-pr.md` is a session draft of the PR body; it is a raw artifact
under the caller's rule even though it duplicates the body. Keep it.

### 3. nitpick — round 02's result file is a zero-byte untracked placeholder

`ephemeral/native-binary-review-run-02.json` is 0 bytes and untracked at
`3877011`. Round 01's `native-binary-review-run.json` was committed with
content. This round's result must land the same way, in a checkpoint commit,
or the pair of prompt and result the caller requires is half missing.

### 4. observation — `just test` is only reproducible with the frozen pnpm first in PATH

Not introduced here and already noted in the worklog, but it is the reason a
reviewer's `just test` can fail on a green branch. The example's
`packageManager` is pinned to 12.9.1 with `onFail: download`; the kitpatch and
`cmd/skgo` tests shell out to whatever `pnpm` is ambient. Nothing to do in
this PR; worth a line in the Justfile recipe or a separate issue.

## Spec coverage

| Requirement | Where | Status |
| --- | --- | --- |
| Loopback HTTP to production dispatcher | `binary_test.go:136–143` | met |
| Go inspects bytes, geometry, identity, shared buffer, distinct empties, cycle | `checkBinaryGraph` | met, anchored on literal byte arrays |
| Independent binary reply | `binary_test.go:133`; mutation A | met |
| Zig's actual response path decodes the reply | `cli.zig:31`; mutation D | met |
| Query and command bytes match Kit 3.0.0 / devalue 5.9.4 | `binary_tests.zig:31–47`; fixture re-derived | met |
| Zig content/identity after response storage released | `binary_tests.zig:52–54` | met |
| Allocator failure coverage | `binary_tests.zig:103–120` | met |
| Bounded to existing consumer; Swift JSON profile unchanged | `abi.zig` untouched; no harness added | met |
| Passing repository checks | CI green at `c5a1241`; pending at `3877011` | pending |
| Independent review | rounds 01 and 02 | this file |
| Merged PR, synchronized main, task cleanup | — | not yet, draft |

## Outcome

material findings remain
