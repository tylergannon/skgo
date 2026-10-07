# Prerender predicates: simplest correct design

**Status:** Agreed. Codex and Claude Fable 5.1 both endorse the existing-owner relay with message-carried replies and terminal failure; Claude's final check (turn 3) added the worker-to-owner failure notice and the renderer-default parity requirement below. Design only: no build has run this, and the definition of done is the proof. The branch is rebased onto `origin/main` at `c6906b7`, including codegen #266; generated Go goes in each package's `skgo_gen.go`. The [RequestEvent plan](20261006-request-event.md) remains the feature plan.

## Decision

Use the threads and service already running. Kit prerenders in a worker; the adapter's main thread owns the Go service and remains available while awaiting that worker. The generated hook forwards custom predicate calls through that owner. Kit's defaults run locally when no callback is configured.

```mermaid
sequenceDiagram
    participant K as Existing Kit prerender worker
    participant A as Existing adapter main thread
    participant G as Existing Go service
    K->>A: Request handle, predicate, arguments, completion cell
    Note over K: Synchronous bounded wait
    A->>G: Invoke retained Go callback
    G-->>A: Boolean or error
    A-->>K: Post reply, mark answered, notify
    Note over K: Read reply synchronously; return boolean or throw
```

There is no extra worker, process per call, asset enumerator, fetch wrapper, decision cache, or speculative callback evaluation. Both predicates use the same small transport. JavaScript transports arguments and results; application decisions stay in Go.

## Why a callback rather than an eager table

Kit requires an immediate boolean from `preload` and `filterSerializedResponseHeaders`; a Promise is truthy and therefore incorrect. More subtly, callback timing can affect the result. Consider a hook whose preload closure reads a request-local `assetsReady` boolean, initially false. An awaited page load sets it to true. Both Kit and SKGo's current runtime evaluate preload after the load, so a font is admitted. A table computed before resolve records false.

Claude proposed precomputing decisions and independently searched for an asynchronous hook seam after every relevant load/render but before Kit's synchronous calls, including error rendering. We found no such seam. Refreshing tables after Go callbacks would still miss state changed by later deferred work. A whole-manifest table also asks about assets Kit never asks about and duplicates its path and branch rules.

Runtime SSR already replays header-filter decisions computed at fetch time. That is useful for stable predicates but does not establish a public purity restriction. Kit also calls the filter later during serialization, and during synchronous header reads inside a universal load. A runtime deviation is a separate correctness issue to investigate, not a reason to constrain the prerender API.

## Minimal relay

The existing main-thread build owner establishes a uniquely named BroadcastChannel before publishing build connection information. Kit's single prerender worker opens that channel through the generated hook. The same owner closes it during its existing cleanup; the channel must not keep the process alive by itself.

For a non-nil callback, the worker posts the logical request handle, predicate kind, Kit's actual arguments, and a one-word shared completion cell. It waits synchronously with the existing callback timeout. The owner asynchronously invokes the retained Go callback under that request's context while the Go hook is suspended in resolve, posts a reply containing a boolean or the full error, then sets the cell to answered and notifies.

Node's `receiveMessageOnPort(channel)` reads the reply synchronously after the wait. `Atomics.wait` returns immediately with `not-equal` if the owner answered before waiting began; `ok` covers an ordinary notification. No polling or worker event-loop callback is needed to receive the reply. The reply must be present and have exactly one valid boolean result or error; a missing or malformed reply fails the build rather than becoming false.

One completion cell may be reused by this worker because its synchronous predicate calls cannot overlap, **provided every bridge failure is terminal**. On timeout, owner/service error, or malformed reply, record a sticky failed state before throwing. Every subsequent predicate call immediately throws that retained failure without resetting the cell or posting another request. The owner uses the existing build-failure/cancellation path. This prevents a late completion from waking a later call even if Kit catches the first error while rendering an error page or an application prerender policy would otherwise continue. Do not add retries or a recovery protocol. Ordinary false results are successful replies.

A worker-side failure must also reach the owner. A predicate thrown inside `render_response` is caught by Kit's `respond` and becomes a 500 page (`handle_fatal_error`), and `config.kit.prerender.handleHttpError` may be `'warn'`, `'ignore'` or a function, under which Kit's prerender *completes* with that page written. So on timeout or malformed reply the worker posts a failure notice on the same channel before throwing, and the owner's existing `fail()` marks the build failed, which `installPrerenderFailureBoundary` turns into a rejected `buildApp` regardless of Kit's HTTP error policy. An owner-observed failure (service error, dead service) takes that path directly. Sticky failure in the worker prevents a wrong answer; the notice prevents a green build around it.

If another rendering worker is introduced later, it needs its own reply channel/cell; this design relies on the verified current one-prerender-worker topology, not global concurrency guesses. The owner must publish a failure reply before cleanup could block waiting for worker exit. Normal service rejection should wake the waiter; a missing or hung owner is bounded by the wait timeout.

Nil callbacks omit the corresponding Kit option and produce no predicate requests. Sequence composition remains in Go. Callbacks execute when Kit asks, without caching or extra calls. The rendering worker pauses during each custom callback, including other renders on its event loop. That cost is real but unmeasured; there is no basis yet for calling it expensive.

## Evidence and limits

- Installed Kit is 3.0.0. `src/utils/fork.js` creates the prerender Worker and awaits its exit using a Promise. `src/exports/vite/build/index.js` awaits prerender. The adapter's `createPrerenderOwner` is main-thread-only and its `buildApp` hook prepares/publishes the service first.
- Kit `src/runtime/server/page/index.js` awaits load data before `render_response`; `page/render.js` invokes preload after component rendering. SKGo `document.go:deliver` calls `renderPlan` before `assemble`, where `document_assemble.go` evaluates preload.
- Kit `page/load_data.js` evaluates header predicates synchronously during header reads; `page/serialize_data.js` evaluates them during serialization. `load_response_header_not_serialized` throws an error; it does not merely warn.
- Claude ran the message-and-notification primitive on Node 24.21.0 with simulated asynchronous responses: true, false, and a full error each returned correctly. This is supporting exploration, not a repository test or actual Kit/Go integration proof.
- The served page composes the hook's `ResolveOptions` over a renderer-level default: `document.go:1097-1105` falls back to `SSROptions.FilterSerializedResponseHeaders` when the hook sets none. The prerender service constructs no renderer, so "non-nil callback" at build time must mean the same composed result, or a page would filter headers when served and not when prerendered. The RequestEvent plan owns where that default comes from at build time; this design only requires that the precedence be identical.
- The actual suspended-request dispatch, Vite/Kit worker integration, terminal failure behavior, and cleanup remain to be demonstrated by implementation. Same-origin Go endpoint fetches during prerender are also an existing gap to address in the broader request-lifecycle work.

## Definition of done for implementation

Use ordinary integration tests around a real Kit build, with literal fixture expectations:

- Custom preload admits a known font and rejects a known JS chunk's preload while leaving the script/import behavior intact. The load-mutated request-local flag example also admits its font.
- A custom header filter admits one fixture header and rejects another in hydration data. Reading a rejected header during a universal load produces Kit's error. Include state changed between fetch and serialization to prevent an eager table passing accidentally.
- Nil callbacks produce no predicate transport calls. A filter set only on `SSROptions`, with no hook filter, filters the prerendered hydration data exactly as it filters the served page.
- With `handleHttpError: 'ignore'`, a predicate timeout still fails the build; the output directory is not left holding an error page as a success.
- True, false, full-error, and answer-before-wait paths work. A killed service yields its failure promptly; a hung owner/callback fails within the existing timeout. After timeout, a late reply cannot make any later call succeed or return the wrong value; the build is terminally failed.
- The build releases its existing owner/channel and logical requests on success and failure. Keep the RequestEvent plan's full request-lifetime contract, including deferred work and response-body completion.

Compare stable-predicate output with served output as useful parity evidence, while taking Kit as the authority where the current runtime differs. Do not expand this design task into repairing the runtime filter or implementing RequestEvent. No real-build implementation proof has been claimed here.

## Discussion history

The user requested alternating commits until both authors endorsed simplicity and correctness. Initial proposals remain in Git history; the arguments are retained separately:

1. Codex proposed reusing the main-thread owner instead of creating another worker.
2. [Claude turn 1](../reviews/20261006-predicate-contest-claude-01.md) proposed async decision tables.
3. [Codex turn 2](../reviews/20261006-predicate-contest-codex-02.md) supplied the load-mutated closure counterexample and refreshed the codegen baseline.
4. [Claude turn 2](../reviews/20261006-predicate-contest-claude-02.md) withdrew the tables, endorsed the relay, and demonstrated message-carried replies.
5. [Codex turn 3](../reviews/20261006-predicate-contest-codex-03.md) accepted that refinement with terminal failure semantics and consolidated the design.
6. [Claude turn 3](../reviews/20261006-predicate-contest-claude-03.md) endorsed the consolidated design, added the worker-to-owner failure notice (Kit's error policy can otherwise finish a build around a sticky failure) and the renderer-default filter parity requirement.
7. Codex final acceptance: I checked Claude's additions against `installPrerenderFailureBoundary` and `documentOptions` and accept both. We both endorse this design's simplicity and correctness; no material disagreement remains. The implementation definition of done above remains outstanding.
