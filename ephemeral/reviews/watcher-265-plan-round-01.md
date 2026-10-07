# Adversarial review: fix plan for #265 — round 01

Written 2026-10-06 22:27 PDT. Read-only review; nothing outside this file was
changed.

## Target

`ephemeral/plans/watcher-265-fix.md` (untracked in worktree `watcher-265`, on
top of `eec9fd4`), reviewed against:

- GitHub issue tylergannon/skgo#265, "Development watcher drops rapid
  restored-content edits and serves stale modules" (open).
- `ephemeral/research/watcher-265/` — `instrumentation.patch`,
  `research_265_test.go.txt`, and the six captured logs.
- The pinned installed sources: `@sveltejs/kit` 3.0.0 under
  `example/web/node_modules`, and the chokidar 3.6.0 bundled in
  `@voidzero-dev/vite-plus-core@1.0.0` (`dist/vite/node/chunks/node.js`).
- `CLAUDE.md` and the `agent-protocol` skill.

The launch prompt set only valid operating constraints (read-only, artifact
path, "plan not implementation"); no narrowing of scope was requested, so none
was ignored.

## Evidence inspected

- `internal/adapter/skgo-adapter/env.js:1038-1134` — `gojaDevUnchangedFiles`:
  the `config` hook that returns `server.watch.ignored`, the `watcher.emit`
  wrapper with its byte-digest `known` map, the template-partial-save guard,
  and the `transform` `remember`. Also `:905-932`, the `/__skgo_dev/changed`
  cursor endpoint.
- `internal/adapter/skgo-adapter.js:116` — the plugin is registered in the
  adapter's `vite.plugins.post`.
- `internal/adapter/dev_watcher_test.go` — the EventEmitter partial-save
  contract (emits directly on a bare `EventEmitter`; chokidar is not in it).
- `example/devrender/devrender_test.go` — `TestMain` (one `vp dev` per
  package over a verbatim copy of `example/web`, `copyWebRoot`), the
  metadata-touch, invalid-UTF-8, build-emit, source-edit and dependency-edit
  tests, and the `changeLog`/`namedChanges`/`settleChanges` helpers.
- `internal/vite/dev.go:208-228` — Go's `Changed(since)` consumer.
- `example/web/vite.config.ts` — no `server.watch` today.
- Vite bundle: `mergeConfigRecursively` (`node.js:5619-5674`),
  `runConfigHook` (`:43077-43095`), `resolveChokidarOptions` (`:21316-21335`),
  watcher construction and `watch === null` → `createNoopWatcher`
  (`:29995-30010`).
- Bundled chokidar: option normalisation (`:16489-16495`), `_emit` with the
  `awaitWriteFinish` pending-write branch preceding the 50 ms change throttle
  (`:16647-16700`), `_throttle` (`:16717-16745`), `_awaitWriteFinish`
  (`:16756-16787`), `_remove`'s `cancelWait` (`:16866-16868`).
- Kit: `src/exports/vite/index.js:398-404` (kit's own `server.watch.ignored`
  in its `config` hook) and `:614-677` (adapter `post` plugins are appended
  after kit's plugins); `src/exports/vite/dev/index.js:195-262` (kit's
  `add`/`unlink`/`change`/`all` watcher listeners).
- `.github/workflows/ci.yml` — `just test` on `ubuntu-latest` runs
  `./example/...`, so `example/devrender` already executes on Linux CI.
- Research logs: `nodefs.log` and `independent-baseline.log` show
  `awaitWriteFinish:false, useFsEvents:false` failing all three 10 ms trials
  (`delivered_changes=1`, served `probe = 2`, disk `probe = 1`) and passing
  the 150 ms control; `stable-writes.log` and `independent-stable-writes.log`
  show `{stabilityThreshold:50,pollInterval:10}` passing all four plus the
  metadata-touch test; `macos-fsevents.log` shows the default FSEvents backend
  passing all four trials **without** `awaitWriteFinish`.

## What the plan gets right

The mechanism claim checks out against the bundled source: with
`awaitWriteFinish` set, `_emit` either folds a second event into the pending
write (`node.js:16649-16652`) or routes `add`/`change` through
`_awaitWriteFinish` and returns (`:16670-16683`) before ever reaching
`_throttle(EV_CHANGE, path, 50)` (`:16685`). The deferred emit reads the file
as it is when stability is reached, so the adapter's digest filter and Vite's
transform both see the settled bytes; a coalesced edit-and-restore correctly
produces no change-log entry and no stale cache. The research logs support
each row of the issue's table. Putting the default in the adapter rather than
the example is the right layer. Keeping `_throttle` untouched and not adding a
second watcher is correct.

## Findings

### 1. Issue — "Preserve `server.watch: null`" cannot be mapped from the adapter's position, and the plan's other preservation cases need an explicit mechanism the plan does not name

The plan requires that a user's `server.watch: null`, `awaitWriteFinish: false`,
`true`, and configured objects all survive, and that the new default apply
"only when the option is unspecified." Mapping Vite's merge from the pin shows
two distinct problems:

- `mergeConfigRecursively` (`node.js:5627-5641`) assigns the override whenever
  the existing value is `null`/`undefined` **or** when either side is a
  non-object (`merged[key] = value`, `:5672`). So a plugin `config` hook that
  simply returns `{ server: { watch: { awaitWriteFinish: {...} } } }` replaces
  a user's explicit `false` or `true` with the object. Preservation is only
  possible by reading the incoming `config.server?.watch` inside the hook and
  returning nothing when the key is present. The plan says "preserve", not
  how; the existing `ignored` hook (`env.js:1064-1067`) is the pattern the
  implementer will copy, and it does not inspect anything.
- `server.watch: null` is already gone before the adapter's hook runs. Kit's
  own `config` hook returns `server.watch.ignored` unconditionally
  (`kit/src/exports/vite/index.js:398-404`), kit's plugins precede the
  adapter's `post` plugins (`:614-677`), and `runConfigHook` merges each
  hook's result in order (`node.js:43084-43092`). By the time
  `gojaDevUnchangedFiles.config` sees `config.server.watch`, it is
  `{ ignored: [...] }`, not `null`. The adapter cannot distinguish "user
  wrote `null`" from "user wrote nothing". The plan's step-1 contract for
  `null` therefore either cannot be written honestly against real Vite
  resolution (the plan insists on real resolution) or will be written against
  the plugin in isolation, which the plan forbids.

Impact: step 1 as written contains a contract that cannot pass by any adapter
change, and the remaining preservation cases will silently fail if the hook
is written the way the existing hook is. The plan should drop the `null`
case (it is kit's behaviour to note, not skgo's to guarantee — the existing
`ignored` exclusion already has exactly this property) and state that the
hook must read the incoming config and only contribute `awaitWriteFinish` when
`config.server?.watch?.awaitWriteFinish === undefined`.

### 2. Issue — The permanent regression is not load-bearing under the backend the devrender fixture actually uses on macOS, and the plan supplies no mechanism for the fixture to choose one

`macos-fsevents.log` shows the defect does not reproduce under the default
FSEvents backend: with `awaitWriteFinish:false` all four trials pass. The
research only failed after the instrumentation patch forced
`useFsEvents:false` in `example/web/vite.config.ts`. The devrender fixture
starts one `vp dev` per package over a byte-for-byte copy of `example/web`
(`devrender_test.go:60-100`, `copyWebRoot`); there is no hook by which a test
can alter the watcher backend, inject a tracing plugin, or add a barrier
endpoint.

The plan's step 3 demands the regression "fail reliably with the new default
removed" using "a test-fixture barrier or recorded event timing", and says
backend selection and instrumentation must be "confined to the copied test
fixture". Both halves presuppose a way to put code into the vite process that
serves the fixture. None exists, and the plan does not say to build one, nor
does it decide the consequential question: if the fixture forces
`useFsEvents:false` for its single `vp dev`, then every other devrender test
on a developer's Mac also runs under `fs.watch` rather than FSEvents; if it
does not, the regression passes on macOS with the fix removed, which
`CLAUDE.md` names as the first thing a validator must check ("would each one
actually fail if the feature were broken or removed"). A second `vp dev` for
one test roughly doubles the package's wall time.

Impact: as written, step 3 is unimplementable without a design decision the
plan defers to the builder while also constraining it ("production must not
acquire research endpoints"). The plan should name the injection mechanism
(e.g. the copy gets its own `vite.config.ts` wrapping the example's, written
by `TestMain`) and state explicitly which backend the package-wide server runs
under on each platform and therefore where the regression is load-bearing.

### 3. Nitpick — Step 4's conditional is already decided by the plan's own evidence

"Add a focused real-server partial-save assertion if the existing
EventEmitter contract cannot establish stability-path recovery." The existing
contract (`internal/adapter/dev_watcher_test.go`) emits `change` directly on a
bare `EventEmitter`; chokidar, `awaitWriteFinish` and the stability path are
not in it and cannot be. The condition is false by inspection, so the plan
should simply say whether the real-server partial-save assertion is in scope.
Note that under `awaitWriteFinish` a truncated intermediate written within the
threshold is never seen by kit's validator at all, so a real-server assertion
whose only content is "no error overlay appeared" is the absence-only `Then`
that `CLAUDE.md` warns about; it needs a positive observation (the settled
template served, with its marker) to be load-bearing.

### 4. Nitpick — The cost of the default is stated but not bounded

Every `add` and `change` on every platform — including macOS FSEvents, where
the defect never reproduced — now waits at least `stabilityThreshold` plus
one `pollInterval` and runs an `fs.stat` poll loop per pending file
(`node.js:16760-16777`). Kit's manifest regeneration on route `add`/`unlink`
(`dev/index.js:231-233`) and its `change` handling inherit the delay. The
plan calls this "a short stability delay" and defers HMR demonstration to
delivery; it should state the per-edit latency it is accepting so the
validator has a number to compare the "page updating to the changed marker"
demonstration against, and note that 50/10 was exercised only under
`useFsEvents:false` (`stable-writes.log`, `independent-stable-writes.log`),
never under FSEvents or `usePolling`.

## Outcome

`material findings remain`
