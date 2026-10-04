decision: Issue 105's client-navigation load check asserts the returned literal values and one document only. A request count is not a Kit parity contract when the load returns only an awaited result and drops the proxy.

decision: Keep an exact one-request check only for the separate retained-reference case: the load returns its query proxy, the component awaits that same proxy, and the response is held long enough to exercise a shared in-flight request.

finding: The installed stable Kit source was verified as `@sveltejs/kit` 3.0.0 at `/Users/tyler/Codex/2026-10-02/task-18/skgo-js-dependencies/example/web/node_modules/@sveltejs/kit`. `src/runtime/client/remote-functions/query/proxy.js` registers each proxy with `cache.ref`; its `then` getter delegates to `pin_while_resolving`. `shared.svelte.js` manually references the cache entry only until the promise settles. `cache.svelte.js` decrements proxy references in the finalizer and evicts after a tick when the count is zero. A live proxy reference therefore protects the cache entry; awaiting and returning only a resolved value does not establish that it will stay protected for a later consumer.

evidence_limit: The retained experiment receipts at `/Users/tyler/Codex/2026-10-03/task-7/skgo-project/ephemeral/experiments/remote-query-cache/` measure Kit 3.0.0-next.28, not stable 3.0.0. Stable source confirms the cache lifecycle relevant to the test correction, but these receipts do not prove a forced-GC double request on stable Kit.

friction: This worktree had no installed `node_modules`, and `go generate` initially tried to write Go's cache under the restricted user cache. `just install` succeeded with isolated Mise paths; setting `GOCACHE=/tmp/query-oracle-go-cache` allowed the intended `just generate` workflow to complete.

finding: `just dev` needs the generated frontend build manifest in a fresh worktree. The first attempt failed before serving; running the documented `just build` first made the normal dev server work. Both browser legs then passed on the task-owned ports 18341/18342.
