# Adversarial review: native handler replacement (stage 1c) + retained project inputs (stage 2), round 01

Date: 2026-10-15 11:42 CST
Reviewer: Claude Fable 5.1 (read-only; only this file written)
Target: branch `codex/dev-retirement-plan`, commits `bd7a4c9..6a48434`
(f3deda4 stage 1c contract, 64a363f retained inputs, 5c4884e stackobject sentinel,
56fea21 tests, dccb75a merge, 6732bfd workspace-selection correction, 6a48434 merge).
Authorities: `AGENTS.md`, `ephemeral/projects/dev-server/PLAN.md` (stage 1c list and
stage 2 accepted proof), the two worklogs under `ephemeral/worklog/` dated 20261015.

## Caller instructions considered

- "stop at the bounded definitions of done, integrate to dev, no main release":
  accepted as a solution constraint. Stage 3 and stage 4 work is not expected
  here, but a stage 1c/2 claim that cannot hold without it is still reported.
- "raw provider event flags alone do not establish whether Project.Watch acquired
  the wrong bytes; a separate validator is checking the atomic-save then
  immediate preserved-mtime write through the actual Project/compiler API":
  accepted as a statement about evidence quality, not as a protected area.
  Finding 2 below records exactly what was and was not proved.
- No instruction narrowed the subject matter or requested a verdict.

## Evidence inspected

- Full diff `bd7a4c9..6a48434` per file; surrounding `fetch.go`, `document.go`,
  `document_fetch.go`, `internal/hotreload/{classify,toolchain,artifact,view,loader,native_runtime}.go`,
  `internal/gen/{retained,retained_watch,retained_analysis,cache_source}.go` and tests.
- `go test ./internal/hotreload` (Go 1.27.2 darwin/arm64, CGO_ENABLED=0): all PASS,
  31.8s, `TestGeneratedNativeHandlerContract` 12.06s. The contract exercises
  3000→4200→3780→3360/3361 literal swaps, Set-Cookie, `/order/` refresh, 418
  endpoint, Money transport, held nested `@nested` (old revision 4200 observed, not
  3780), held serialization, deferred chunk, stream, syntax/structural/`go`
  statement/external-caller refusals, forged profile fields, corrupt object,
  authored panic line, production build without goloader/purego.
- `go test ./internal/gen` cold full package: run 1 FAIL at
  `retained_test.go:341` (81.2s); runs 2–4 PASS (21.4s, 69.9s, 75.3s). Isolated
  `TestRetainedProjectFilesystemEvents` x15: PASS.
- New tests `TestRetainedWatcherStartsDuringTemporaryBuildChurn` and
  `TestRetainedWorkspaceSelectionEvents` (explicit/auto): PASS, 4.1s.
- Scratchpad probes (not project code; paths under the session scratchpad):
  `fsrepro/main.go` — fsnotify v1.9.0 kqueue, rename `one.txt.tmp` over `one.txt`,
  then on the first event write "same-time" to the new inode: 300/300 iterations
  produced no further Write event for that inode within 1.5s.
  `ancestors/main.go` and `isnotexist/main.go` — see Finding 3.
- fsnotify v1.9.0 `backend_kqueue.go` lines 529–660 (`sendCreateIfNew`, re-watch
  after Remove).
- Go 1.27.2 `cmd/go/internal/modload/init.go` as cited by the 6732bfd worklog
  correction (not independently re-read; the port's three rules match the
  documented behaviour).

## Findings

### 1. issue — SSR render-time `event.fetch` does not carry the native revision

Plan line 73: "Nested calls/internal fetches borrow the enclosing revision
lifetime." Stage 1c list: "nested internal request completes while publication
waits." The seam added for this is `FetchConfig.CarryContext` (`fetch.go:87–105`),
applied inside `requestFetcher.serve`. The document renderer builds its own
fetcher at `document_fetch.go:79`:

```
fetcher := &requestFetcher{handler: s.fetch, hook: s.handleFetch, base: s.base, prerendered: s.prerendered}
```

No `carryContext` is set, and `SSROptions` exposes none. `document.go:1005–1020`
shows the SSR remote `answer` path using the render context (coherent), while
`Fetch` goes through this bridge (not coherent). A `load` or component that
calls `event.fetch('/api/...')` during SSR therefore reacquires `_skgo_current`
in the nested handler and can observe a newer revision than the page that is
mid-render, which is the exact old/new mixing the plan forbids.

The contract test does not cover this: the stage 1c page (`contract_test.go`)
does not fetch during render, and the held `@nested` case goes remote → `event.Fetch`
endpoint, which is the `fetch.go` path. Impact: a stage 1c claim ("nested internal
request completes while publication waits") holds for endpoints and remotes but
not for SSR, and nothing would fail if it regressed. Bounded fix is in scope of
stage 1c: thread `CarryContext` into the document fetcher and add the SSR-fetch
case to the contract.

### 2. issue — stage 2 contract is nondeterministic under `just test`; the narrow window is real but its product effect is not yet proved

`internal/gen/retained_test.go:337–341` performs an atomic rename, then an
in-place write with `os.Chtimes` restoring the old mtime, and waits for
"same-time" (`wait("same-time")`, fatal "file events did not acquire same-time").
One of four cold full-package runs failed there; isolated runs pass.

Evidence boundary. What is proved: fsnotify v1.9.0 on kqueue, after a Remove
for a replaced inode, sends the event and then Lstat + re-adds the watch for the
new inode (`backend_kqueue.go:642 sendCreateIfNew` and the Remove handling above
it). A write to the new inode that lands before the re-add produces no Write
event (300/300 in the scratchpad repro with an immediate write). `retained_watch.go`
calls `Observe` synchronously per event and performs no re-read after a Remove,
so if the provider drops that Write, `Project.Watch` never acquires the bytes
until some later event on the same path. What is NOT proved: that the failed test
run was this window rather than scheduling pressure from 24+ concurrent
toolchain tests, and that a real editor save sequence (atomic replace followed
by a second write within microseconds, with preserved mtime) occurs in practice.
The separate validator the caller mentions is the right instrument for that.

Impact as it stands: `AGENTS.md` requires `just test` green in seconds, and a
test that fails one run in four on the author's machine will fail CI; a flaky
contract gets deleted rather than fixed. Either the watcher reconciles the path
after a Remove/Rename (one stat-and-reread, which the plan's "atomic saves ...
preserved-mtime writes" acceptance already motivates), or the test must not
depend on the provider delivering a Write inside the re-watch window. Classified
issue, not critical, because the lost update is bounded by the next event on the
path and the contract otherwise passes.

### 3. issue — `GOWORK=auto` with no selected `go.work` watches every ancestor up to `/Users`, and a dangling symlink in `$HOME` makes `Project.Watch` fail

6732bfd restricted watch scope only for `off` and for an explicitly selected
file. For the default (`auto`) with no `go.work` found, `cache_source.go:139–165`
still walks from the host to the filesystem root (or `GOROOT`) and appends every
`go.work` candidate; `retained_watch.go:58–66` then adds `filepath.Dir(control)`
for each, skipping only the volume root. For an app at `~/src/foo/app` that
means kqueue watches on `~/src/foo`, `~/src`, `~` and `/Users`. fsnotify's kqueue
backend opens every entry of a watched directory (`watchDirectoryFiles`,
`backend_kqueue.go:570`) and returns an error for any entry whose Lstat fails.

Reproduction (scratchpad `isnotexist/main.go`, fsnotify v1.9.0):

```
w.Add("/Users/tyler")
err="/Users/tyler/.tmux.conf": no such file or directory
os.IsNotExist=false   errors.Is(err, os.ErrNotExist)=true
```

`retained_watch.go:64` filters with `os.IsNotExist(err)`, which does not unwrap
`fmt.Errorf("%q: %w")`, so `Watch` returns the error and the project has no
watcher at all. This is the same failure class the churn test reproduced for
`off`, left in place for the mode every developer who has not set `GOWORK` runs
in. Boundary: reproduced against fsnotify directly with the real `$HOME`; not
reproduced through `OpenProject`/`Watch` because the fixtures force
`GOWORK=off` or create a `go.work` that stops the walk. The example app in this
checkout is unaffected because `/Users/tyler/src/skgo/go.work` is selected.
Impact: stage 2's "rename/removal via events" and "newly created ancestor
go.work changes selection" are claimed for a configuration whose watcher cannot
start on a typical macOS home directory. The plan's intent (detect a new
ancestor `go.work`) does not require watching `$HOME` with per-entry kqueue
descriptors; stopping the walk at the module root's parent or at the Go module
cache/home boundary, and using `errors.Is`, would bound it.

### 4. nitpick — `Classify` rejects a `go` statement anywhere in the root package, including unchanged code

`classify.go:59–65` walks the whole package and returns `CompleteBuild`
("asynchronous execution at ...") for any `*ast.GoStmt`, not only in functions
whose bodies changed. The plan allows refusing asynchronous execution "at this
stage", so this is bounded, but an app with one pre-existing goroutine in an
unrelated file never gets a native swap, and the diagnostic points at code the
developer did not touch. Worth a note in the diagnostic text.

### 5. nitpick — CI pins `go-version: 1.27.2` while `go.mod` says `go 1.27.1`

`.github/workflows/ci.yml:31` and `:104` pin 1.27.2 for the general test job
(line 77 still uses `go-version-file`). `Toolchain` requires exactly 1.27.2
(`toolchain.go`), so the pin is needed for the native job, but the general job
now tests a toolchain that `go.mod` does not declare. Harmless today; it will
silently drift when either is bumped alone.

## Not findings (checked)

- `dependencyIdentity` (`artifact.go:50`) hashes every archive in importcfg per
  compile; measured contract cost is acceptable (12s including builds).
- No non-test consumer wires `gen.CompilerInputs` into `hotreload.CompileApplication`;
  the plan assigns that to stage 4.
- Identity check skipped for hosts not built through `HostView` is documented
  as the legacy path.
- `gen` package cold runtime 70–81s against "seconds": pre-existing toolchain
  tests dominate; not introduced by this range.

## Outcome

material findings remain
