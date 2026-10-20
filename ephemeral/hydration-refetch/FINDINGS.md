# Hydration refetch flake: /items/[id]

Scenario: remote.feature "A universal load shares its remote query with the
component at hydration". CI run 39350545988 attempt 1 saw
`GET /_app/remote/4rw8ae/getItem` at hydration; rerun passed.

## Status

Resolved as Kit client timing; scenario retargeted to the retained fixture.

### Payload (skgo prod binary, curl /items/93)

`__sveltekit_x.data = {q:{"4cga8b/whoami/":{v:...},"4rw8ae/getItem/WyI5MyJd":{v:{...}}}}`
placed between `await import(app)` and `kit.start(...)` -- the same shape and
position as Kit's renderer (kit src/runtime/server/page/render.js:510-526,
collect_remote_data in src/runtime/server/remote-functions.js:411). Key equals
Kit's create_remote_key (src/runtime/shared.js:346) for the payload the client
sent in CI (`?payload=WyI5MyJd`). CI trace shows the same key in the document.

### Kit client mechanism (3.1.0)

- `_start` copies `data.q` into `query_responses` (client.js:466-473).
- First `new Query(key)` consumes and deletes the entry (query/instance.svelte.js:78-87).
  At hydration the first consumer is the universal load's proxy.
- The entry lives in `query_map` only while proxies reference it: `cache.ref`
  registers each proxy with a FinalizationRegistry (cache.svelte.js:44-46,106-109);
  `deref` evicts on the next tick when count hits 0 (cache.svelte.js:130-137).
- `pin_while_resolving` pins only until the awaited promise settles
  (shared.svelte.js:65-77); `pin_in_effect` is a no-op outside an effect (load).
- `+page.ts` returns only the value, so the load's proxy is unreachable once the
  load returns. If GC + the finalizer task run before the component's
  `getItem(params.id)` constructs its proxy, the entry is evicted, the new Query
  finds no `query_responses` entry, and it fetches.

Local baseline: 40/40 pass with `--repeat-each 40` (idle machine, 1 worker).

### Reproduction (forced GC)

Chromium with `--js-flags=--expose-gc`, init script calling `gc()` on every
task (MessageChannel loop, 3 s). Same built binary, real step definitions:

- `/items/93` (old scenario): 20/20 fail with exactly the CI error
  `remote requests: GET /_app/remote/4rw8ae/getItem`.
- `/items/93?retain=1` (load returns the proxy in page data): 20/20 pass.
- Without forced GC: old scenario 40/40 pass locally.

Conclusion: Kit client timing, not skgo bytes. Kit's renderer emits one
`q[id/payload] = {v}` per key no matter how many server consumers there were;
the client hands it to whichever Query is constructed first (the load, at
hydration) and nothing pins that Query for the component once the load's
proxy is unreachable. A Kit server would produce the same request.

Change: the hydration scenario now opens `/items/93?retain=1`.

### Measured (same built binary, port 8361)

| condition | old `/items/93` | new `?retain=1` |
|---|---|---|
| idle, `--repeat-each 40` | 40/40 pass | 40/40 pass |
| 10 CPU burners, 8 workers, 150 each | 150/150 pass | 150/150 pass |
| allocation-pressure init script (no explicit gc), 30 each | 24 pass / 6 fail (CI error) | 30/30 pass |
| `gc()` every task, 20 each | 0/20 pass (CI error) | 20/20 pass |

Mutation: binary built with the `q` key mis-spelled (document.go `record`
call, `id+"/x"+payload`): new scenario 5/5 fail
(`GET .../getItem, GET .../whoami`). Source reverted.

Full `just e2e prod` with the change: 180 passed.

Probe harness (init-script GC loop, temp feature, temp config) was scratch and
is deleted; nothing of it is committed.
