# Adversarial review: native handler replacement (stage 1c) + retained project inputs (stage 2), round 02

Date: 2026-10-15 12:12 CST
Reviewer: Claude Fable 5.1 (read-only; only this file written)
Target: branch `codex/dev-retirement-plan` at `b6f5e25`
(round 01 reviewed `6a48434`; this round adds 7811883, 2ce3537, 57a62eb, b6f5e25:
render-fetch revision carry, fsnotify 1.9.0→1.10.1, arm-before-read watch
registration, two new watcher tests, one new document fetch unit test).
Authorities: `AGENTS.md`, `ephemeral/projects/dev-server/PLAN.md`, worklogs
`ephemeral/worklog/20261015-native-handler-contract.md`,
`202610151115-retained-project-inputs.md`, `20261015-parallel-native-inputs.md`.

## Caller instructions considered

No instruction narrowed subject matter or requested a verdict. The pointer to
`ephemeral/native-inputs-just-test.log` was used as one more data point, not as
a substitute for runs made here.

## Evidence inspected

- Full diff `6a48434..b6f5e25` for every file; surrounding `fetch.go`,
  `document_fetch.go`, `internal/gen/retained_watch.go` event loop (lines
  170–260), fsnotify v1.10.1 `backend_kqueue.go` (`addWatch` 351–, `readEvents`
  495–552, `watchDirectoryFiles` 583–, `dirChange` 622–, `sendCreateIfNew` 656–).
- `go vet ./... ./example/...`: clean.
- `go test . -count=1`: ok (2.1s). `go test ./internal/hotreload -count=1`: ok (31.5s).
- `go test ./internal/gen -count=1`, two sequential cold runs on this Mac:
  run 1 ok (123.1s); run 2 FAIL (90.8s) in `TestRetainedAtomicThenPreservedMtimeCompilerInputs`
  (see finding 1). The user-supplied `ephemeral/native-inputs-just-test.log`
  shows a full `just test` green with `internal/gen` at 122.3s and
  `internal/hotreload` at 79.3s.
- Scratchpad probe `anc110/main.go` (fsnotify 1.10.1): adding every ancestor
  of `/Users/tyler/src/skgo/example` up to `/` succeeds; 6 directory watches
  cost about 447 descriptors on this machine.

## Round 01 findings, status at b6f5e25

- Finding 1 (SSR render-time `event.fetch` did not carry the native revision):
  closed. `document_fetch.go:80–85` inherits only the enclosing fetcher's
  `carryContext`; `document_fetch_test.go:163` proves event/locals/private
  values do not leak while the host value does; `contract_test.go:346–355`
  holds a real Kit universal-load `fetch` inside a goja render across an
  install and requires `Order total 3780` beside `Fetched endpoint:3780`, then
  `endpoint-v2:3360` on the next render. The worklog records that the test
  produced a mixed document before the fix, so it is load-bearing.
- Finding 2 (lost write after atomic replace): the mechanism is closed by
  source analysis. fsnotify's kqueue `readEvents` calls `w.remove(name)` before
  `sendEvent` (lines 500–507), so when `run()` receives the Remove and calls
  `armFile` → `addWatch` (`retained_watch.go:112–165`), `addWatch` finds no
  entry, Lstats the replacement inode and registers it before `Observe` reads.
  fsnotify's later `sendCreateIfNew` then dedups on the path. A write after
  the registration produces an event; a write before the read is captured by
  the read. The original `retained_test.go:341` assertion passed in both cold
  runs here and in the supplied `just test`. Finding 1 below is the new failure
  in the same workflow.
- Finding 3 (`GOWORK=auto` watcher could not start past a dangling symlink in
  `$HOME`): closed. fsnotify 1.10.1 tolerates missing symlink targets in
  `watchDirectoryFiles`; `retained_watch.go:64` uses `errors.Is`;
  `TestRetainedAutoWorkspaceDanglingAncestor` runs the real `OpenProject`/`Watch`
  in an isolated process with `GOWORK=auto` and a dangling sibling, then proves
  a farther `go.work` enters the private compiler workspace with both modules.

## Findings

### 1. issue — a benign provider error during an atomic save is forwarded as a project error, and `just test` is still nondeterministic

`internal/gen/retained_watch.go:181–185` forwards every value on fsnotify's
`Errors` channel to the project owner and then treats it as an overflow,
reconciling all module roots. fsnotify 1.10.1 emits a non-fatal error whenever
its own directory re-enumeration races a file that has since been renamed:
`dirChange` (`backend_kqueue.go:622–650`) reads the directory, finds the
temporary file, and `sendCreateIfNew` → `internalWatch` → `addWatch` → `os.Lstat`
fails with ENOENT because the save already renamed `x.tmp` over `x`. Only
EACCES/EPERM are suppressed there; ENOENT reaches `Errors`.

Reproduced here in the second cold run of the package, in the test added by
b6f5e25 for exactly this workflow:

```
--- FAIL: TestRetainedAtomicThenPreservedMtimeCompilerInputs (0.20s)
    retained_watch_test.go:310: lstat .../wire/assets/one.txt.tmp: no such file or directory
```

Line 310 is `waitRetainedFiles` fataling on the first value from the project's
error channel. Frequency observed at HEAD: 1 failure in 3 cold `internal/gen`
runs (two here, one in the supplied `just test` log). AGENTS.md: a flaky
`just test` is a failure, and a brittle contract is deleted rather than fixed.

Product impact beyond the test: every editor atomic save can surface a
spurious error to whatever owns the project in stage 4 and trigger a full
reconcile of every module root, which is the slow path the retained design
exists to avoid. The provider's ENOENT for a path that no longer exists is not
"event completeness unknown"; the watcher should distinguish fsnotify's
overflow/queue errors (which do need reconciliation) from a missing-path error
whose path is already covered by the Remove/Rename it will also deliver.
The boundary of this evidence: the failure is the error-forwarding path, not
stale bytes; no run produced wrong captured bytes or a wrong compiled literal.

### 2. nitpick — `GOWORK=auto` with no selected workspace watches every ancestor including `$HOME`, `/Users` and `/`

`cache_source.go:139–165` walks to the filesystem root and b6f5e25 removed the
volume-root skip in `retained_watch.go` ("preserve the filesystem-root
candidate"). The plan does not mention ancestor `go.work` detection at all
(no match for "ancestor" or "go.work" in `PLAN.md`); the behaviour mirrors
cmd/go's search. Measured cost is modest (about 447 descriptors for the six
directories above the example app) and unrelated events are dropped at
`retained_watch.go:203–219` before any read. It still means a dev server holds
a kqueue descriptor for every entry in the user's home directory and `/`. Worth
stating as a deliberate decision in the worklog, or stopping at the nearest
directory that cmd/go could actually select.

### 3. nitpick — the generated contract host shares `hold` between goroutines without synchronisation

`contract_test.go:555` makes `hold` a package-level variable written by the
command loop (`start`/`release`) and read inside the SSR `HandleFetch` closure
on request goroutines. The generated program is not run under the race
detector, so this is invisible today, but it is the kind of fixture race that
later shows up as a flake in the one test that proves stage 1c.

### 4. nitpick — `Classify` refuses a `go` statement anywhere in the root package, including untouched files

`classify.go:59–65`. Within the plan's "at this stage" allowance; the
diagnostic should say the statement is pre-existing when it is.

### 5. nitpick — CI pins Go 1.27.2 for the general test job while `go.mod` says `go 1.27.1`

`.github/workflows/ci.yml:31,104` vs `go.mod:3`. Harmless now; will drift
silently when one is bumped alone.

## Not findings (checked)

- Stage 1c contract: all listed behaviours exercised by one generated host in
  `TestGeneratedNativeHandlerContract`, including the new held render fetch;
  passes here and in the supplied `just test`.
- Stage 2 accepted proof: atomic saves, preserved-mtime writes, populated
  directory creation, rename/removal via events, hold/release ordering, local
  dependency/embed invalidation, `[n=Order]` matcher, bounded repeated edits,
  explicit/auto/off workspace selection: all present and passing when the
  provider does not emit the error in finding 1.
- `go.sum` and `example/go.sum` updated consistently for fsnotify 1.10.1;
  builds and vet clean.
- No stage 3/4 work was started; no main release.

## Outcome

material findings remain
