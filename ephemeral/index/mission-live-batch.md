# Mission: live and batch queries during render (#61)

Capability when done: a page whose render touches a `query.live` gets the
first yielded value in its markup, as kit's SSR does, and a page whose render
touches `query.batch` queries has them batched to Go in one host call and
answered as one, as kit does. Proof: `live.feature` and `batch.feature` in
`example/e2e` with screenshots of the rendered state, Go tests of the batch
path, existing suite green in both modes. Closes #61.

## Where the work stands

About 80% exists, uncommitted, in the worktree
`/Users/tyler/src/skgo/.claude/worktrees/mission-2-live-batch`
(branch `mission-2-live-batch`, based on `3708fd8`; main has since moved to
`bcf8b5d`, which changed the polytype backend and generated files, so
`git rebase origin/main` first and re-run `just generate` if a generated file
conflicts). The previous builder had the baseline suite green in both modes,
found and fixed the poisoned-runtime hazard, and was running `go test` when
stopped. Nothing is pushed. `git -C <wt> diff --stat` and `git status` show
the whole delta (22 modified, 10 new):

- Go: `remote.go` (+76, batch dispatch), new `remote_batch.go` +
  `remote_batch_test.go`, `refresh.go`, `document.go`,
  `internal/gen/scan.go` (`kindBatch`), `internal/gen/emit.go`, gen tests,
  `internal/ssr/globals.go` (+70), `internal/ssr/ssr.go` (+63: macrotask
  drain and per-render queue reset), `ssr_test.go`.
- Adapter: `internal/adapter/skgo-adapter.js` (+68: live iterator and batch
  host protocol inside the `$app/server` substitute, `SSR_APP_SERVER` at
  :989) and its copy `example/web/skgo-adapter.js`.
- Example: new `routes/live/`, `routes/batch/`, `todos/+page.svelte` and
  `todos.remote.go` changes, `+layout.svelte` nav, `businesslogic/store.go`
  Snapshot broadcast, `generated/*`.
- Suite: `features/live.feature`, `features/batch.feature`, `steps/live.ts`,
  `steps/batch.ts`, `steps/app.ts`.
- `.gitignore` and `ephemeral/screenshots` untracking: already done on main
  by the index PR; drop that part on rebase.

Finish it: rebase, build, run `live` and `batch` features until green, break
each behaviour once to see its scenario fail, `go test ./...` in both modules,
full suite once per mode, PR.

## Kit as the specification

- `runtime/app/server/remote/query.js`:515 `create_live_query_resource` is
  what a live query returns during SSR: it awaits the generator's first
  value. :186 and :203 are the two call sites. :248 is `batch`.
- `runtime/server/remote-functions.js`:35 `create_live_query_response` is the
  SSE wire the browser consumes after hydration (skgo: `remote_live.go`);
  :208 `query_batch` is the batch POST wire.
- Browser side: `runtime/app/remote/`, `runtime/client/remote-functions/`,
  `runtime/client/sse.js`. The client keeps an argument-keyed query cache;
  the payload sent to Go must be kit's cache key, not the validated argument.
- Timers: goja has none. `query.batch` in kit collects within a microtask
  turn; the worktree's macrotask drain in `ssr.go` is how that was mirrored.

## Files skgo-side you will touch

`remote.go`, `remote_batch.go`, `remote_live.go`, `document.go` (`answer`:792
is the host callback that answers remote calls made during render),
`internal/ssr/ssr.go`, `internal/ssr/globals.go`, `internal/gen/scan.go`,
`internal/gen/emit.go`, `internal/adapter/skgo-adapter.js` (`SSR_APP_SERVER`
:989-1083 only; the adapter rebuild is another mission running concurrently
and will port your delta, so keep it inside that block).

## Collisions

Mission "showcase" edits `routes/+page.svelte` and `+layout.svelte` nav
concurrently; mission "adapter" rewrites `internal/adapter` and touches
`internal/ssr`. Rebase onto main before opening the PR; conflicts in
`+layout.svelte` are link lines.
