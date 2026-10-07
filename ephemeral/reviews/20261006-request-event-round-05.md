# Adversarial review — RequestEvent plan, round 05

**Target:** `ephemeral/plans/20261006-request-event.md` (untracked, worktree `bad1`, baseline
`916ccdc`), revised after round 04.
**Kind:** design plan; implementation deferred until #262 lands. No code under review.
**Operating constraints honoured:** read-only except this artifact. No caller narrowing detected.

## Evidence inspected

Everything from rounds 01–04; the full plan re-read, and a diff of the plan against the copy
taken at the start of round 03 to confirm every change since. New for this round: Kit
`src/utils/fork.js:1–60` and `src/core/postbuild/prerender.js:14,26` (prerendering runs in a
`worker_threads.Worker`, env-forwarded, exiting via `process.exit()`), skgo
`internal/adapter/skgo-adapter/prerender.js:10–14,76,322,452–456,563–564` (owner is main-thread
only; the service URL/secret reach the prerender worker through `process.env`),
`cmd/skgo/main.go:8–10,194–199` and `internal/newapp/gofiles/internal/skgo/config.go.tmpl:4`
(the `//go:generate` directive is the one source of generation flags).

## Round-04 findings — status

1. Synchronous predicates → **resolved**: contract 4 now distinguishes the awaited
   `transformPageChunk` from the two synchronous boolean predicates, specifies a long-lived
   worker with a shared-memory wait, forbids per-call process spawning, requires bounded failure,
   and anchors acceptance in literal presence/absence (so Promise truthiness cannot pass). Checked
   against the pin: `Atomics.wait` is permitted on Node worker threads, Kit's rendering thread is
   a worker (`fork.js:56`), and nested workers plus `SharedArrayBuffer` are same-process, so the
   mechanism is feasible as described.
2. No-buffering transport prescription → **resolved** (contract 3 now defers buffering to the
   implementer and keeps the observable contracts).
3. Nitpicks (begin returns on entering resolve; unmatched requests without `platform`; goja
   bundle does not load server hooks) → incorporated.

## Findings

No material findings remain. The public type shapes, locals ownership and binding instant,
layout domain, entry-point coverage, hook selection, prerender lifecycle contracts, in-tree
update scope, exclusions and the #262 gate are explicit, consistent with the pinned Kit source,
and stated as acceptance rather than method. Three genuine nitpicks:

### Nitpicks

- **Contract 4, worker ownership wording.** The text says "one long-lived adapter-owned Node
  worker per build … the build owner terminates the worker on completion/cancellation". The
  adapter's owner exists only on the main thread (`prerender.js:322 if (!isMainThread) return
  null`), while the rendering thread that must block is Kit's prerender `Worker`
  (`fork.js:56`), which ends itself with `process.exit()` (`fork.js:42`). The predicate worker
  therefore has to be created lazily *inside* Kit's prerender worker by the adapter's prerender
  helper, sharing memory with that thread, and it dies with that thread; the main-thread owner
  only owns the Go service. One sentence saying so avoids an implementer wiring the worker to the
  wrong thread and then discovering the `SharedArrayBuffer` cannot reach it.
- **Contract 4, known cost.** A blocking wait on Kit's prerender worker stalls every in-flight
  render on that loop, including the async `/load` bridge, for the duration of each predicate
  round trip (`config.kit.prerender.concurrency` > 1 is serialized at those points). The plan
  already accepts the per-call cost; stating this specific consequence prevents a later "fix"
  that moves predicates off-thread and reintroduces the Promise-truthiness bug.
- **§1, configuration source.** `--locals-package`, `--locals-type`, `--hook-package` and
  `--hook-symbol` are read from CLI flags (`main.go:194`), and the scaffold's only persistent
  carrier is the `//go:generate` directive (`config.go.tmpl:4`). Say that `check`, the MCP
  surface and any e2e invocation obtain them from that same directive (or receive identical
  flags), so a second caller cannot generate against a different locals/hook selection.

## Outcome

`only nitpicks remain`
