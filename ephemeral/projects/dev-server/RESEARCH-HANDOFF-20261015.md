# SKGO development experience research handoff — 2026-10-15

## Read this before continuing

Read [GOAL.md](GOAL.md) first. The user explicitly requested the four architectural responsibilities be written down. We do not yet know the solution; broad competing-strategy research remains required.

The user requested this handoff immediately because context compaction is imminent. Read the `agent-protocol` skill after compaction. This document records the session's current authority, coaching, research and unfinished work. It is not a new implementation plan or a declaration that the research is complete.

**Current goal: run `skgo dev`, open a browser, save Svelte or Go, and see the change quickly across the whole application, with useful diagnostics and recovery. Fast live Go patching is REQUIRED for ordinary edits. Preserving initialized Go application memory is NOT required. Restart/reinitialization is an acceptable fallback, not a replacement for the fast path. Preserve browser drafts and healthy page state where practical.**

The session is in research, source collection, candidate-experiment and semantic-index preparation. No development-server implementation or new proof-of-concept runtime has been built or executed. Do not jump to implementation after compaction or mistake an unexecuted candidate note for an approved architecture.

## User coaching and corrections — preserve the substance

1. The user began with issue [#324](https://github.com/tylergannon/skgo/issues/324), explicitly warning that its agent-written requirements may be wrong. Read PLAN.md and the manager conversation as context; the overriding aim is a best-in-class web application development server properly integrated with Vite HMR and browsers. The issue's boundaries are not binding when they obstruct that goal.
2. The user strongly objects to tasks that are FAR TOO SMALL. Bootstrap, validation and merge overhead mean the eventual implementation should take a worthwhile stride. Expand beyond issue #324 when useful. A ModuleRunner micro-spike or disconnected primitive must not become the final deliverable just because it is easy to finish.
3. Explain the developer feature first. `get_runner()` is Kit's internal helper, not a user-facing feature, SKGO feature or reason to bury the discussion in internals.
4. The design boundary matters enormously in agentic development: selecting the wrong shape can cause an order of magnitude more code, or even more work without solving the problem. The user does not want agents dispatched to fight uphill on an unexamined architecture. Find the elegant composition of existing machinery before implementation dispatch.
5. The user explicitly specifies TWO cooperating servers: a Vite dev server doing frontend transforms/watching/module graph/HMR/browser transport, and a Go application server handling application requests and existing pooled-goja rendering. SKGO development supervision observes Go changes and applies native live patches, restarting only as fallback. Research minimal cooperation between these servers. Supervisor responsibility is not a mandate for another public service. Routing/transport details must serve this architecture, not replace it.
6. Goals and strategies must never be confused. The goal is save a file and see the change across Svelte/Go. A specific runner, transport, listener topology, loader, gopls port, cache or patch is only a candidate strategy. Repeatedly return to the user-visible goal rather than promoting a proposed mechanism into the mission.
7. Research is explicitly authorized and requested before development. The user does not think the answer has been found yet and expects premature implementation to fail. Gather source/docs and prior art broadly, create lightweight potential-experiment ideas, do more focused research rounds, then build a problem-focused semantic index. Cheap experiments will follow that groundwork. Avoid heavyweight plans and complete speculative architectures.
8. Use Luna subagents for internet/GitHub prior-art research and independent concept generation. Use **Sol 6.1** for semantic-index main and readers; user also allowed Opus, but explicitly excluded Fable and Astra for the index. We selected `gpt-6-luna` and `gpt-6.1-sol` respectively. Do not silently use the parent model for index authorship.
9. **State-preservation correction:** the user challenges how much initialized server state needs preservation. Normal web applications are request dispatchers; rebuilding/reloading the HTTP server and losing its memory is normal. The user said: "fuck initialized state" and "If we can't re-initialize and then go back and start responding to HTTP requests again, then we're just not good at what we're trying to do." Do not require arbitrary stateful daemons/goroutines to survive a hot swap.
10. Preserving daemon-like state across arbitrary replacement would be ideal but is enormously difficult and not needed here. The user cites Gimbal, a major SKGO application: it deliberately moved workflow runs into separately spawned processes so the HTTP server can be rebuilt/reloaded repeatedly without carrying their state. Treat that as user-supplied design context; this session has not independently inspected Gimbal.
11. The user wants practical heuristics and bounded solutions, not an academic pursuit of perfect lifecycle proof. They compare this to yesterday's code-retirement discussion and suggest keeping old code briefly/in an LRU then retiring it. Preserve the intent to avoid perfectionism. Do not misstate this as a proof that LRU eviction alone makes still-referenced native code safe to unload. Bounded retention and occasional host recycling can keep general unloadability off the critical path while preserving required fast live patching between recycles. Process exit provides ordinary reclamation. Do not start another general lifetime-analysis project.
12. The assistant correctly recognized overconstraining state preservation, but then made a serious second error by declaring "restart-first, live patching optional." The user explicitly rejected that interpretation. **Live patching is mandatory; initialized-state preservation is not.** Do not conflate those independent requirements. Browser continuity is also independent of Go memory.
13. The user explains why live patching is the point of this work: the team already had rebuild/restart, abandoned it after prior work found a roughly six-to-ten-second full rebuild/relink floor, and has spent the past week developing the faster path. Full host relinking on every save makes development unusable. Preserve the existing native replacement foundation and research how to integrate it cheaply with the supervisor/Vite/browser. This latency history is user-supplied in this session, not a new measurement.
14. Permission to restart/reinitialize when needed is a practical fallback and a relaxation of memory continuity, not permission to drop patching or claim performant Go development is impossible. Do not repeat the already-rejected rebuild-on-every-save strategy.

15. The user explicitly challenges taking Vite responsibilities into Go. Vite must retain transforms, its module graph, invalidation and HMR. The candidate is its own JavaScript runner inside goja with minimal host facilities and existing transport. Do not prescribe a Go rewrite as fallback. If direct reuse encounters a concrete incompatibility, reconsider the integration boundary before duplicating upstream algorithms. Older PLAN/adoption semantic-port instructions are context, not a current mandate.

16. The user reiterates that the two-server architecture was explicit from the start and worries the research is drifting into reimplementing Vite. The assistant acknowledges centering the runner was misleading. Keep runner reuse as a possible internal detail only. Research Vite-produced live output → existing Go renderer and installed Go patch → Vite/browser notification, with minimal glue. Broad information in the cache is welcome; breadth of research is not authorization to broaden implementation or replace Vite responsibilities.

17. The user explicitly rejects reducing the research to one question or a favored set of seams. Investigate a broad set of plausible strategies and combinations, preserve competing maybes, and choose experiments after the research. The current candidate notes are incomplete. A five-minute in-thread heartbeat named `Keep SKGO research aligned` (id `keep-skgo-research-aligned`) is active to remind the assistant to distinguish goals/architectural requirements from tactics and maintain research breadth.
18. The user explicitly asked to save the four architecture bullets and keep them foremost. They are recorded in GOAL.md beside this handoff and in the research cache. There is no single mapped solution yet.

The user requested durable handoff before compaction, then explicitly corrected the assistant's erroneous restart-first interpretation. This document now incorporates that correction. Resume research/index work with REQUIRED fast live patching and OPTIONAL initialized-state preservation; restart is fallback.

## Workspace, cache and authority

- Worktree: `/Users/tyler/.codex/worktrees/6b4a/skgo`.
- Branch: `codex/vite-dev-integration`.
- Source foundation: `bd7a4c9e3b3a476c8f0eac74fb3da4ed6f0be781` on `dev` at session start.
- Existing session checkpoint: `ab57556` (`docs: capture Vite development integration constraints`), only a worklog. More handoff/worklog changes may be uncommitted while this note is written; inspect `git status` rather than assume.
- Tracked plan: `ephemeral/projects/dev-server/PLAN.md`. It is prior context and source routes, not authority over the user's distinction between required fast patching and relaxed state preservation. PLAN.md has NOT been rewritten during this research.
- Issue #324: `Design and implement live SvelteKit rendering through Vite and pooled goja (stage 3)`. Read through `gh issue view 324 --repo tylergannon/skgo --json title,body,comments,state`; no issue edits were made.
- Manager chat: `01a13cc3-9c78-7753-9c7f-eeab10684cd8`, title `Break plan design paralysis (2)`. User supplied host `remote-control:env_e_6a4d4d99990c8322b549a2c0ba9fecef`; tool resolves it to a local thread. Tool `read_thread` accepts at most ten turns. Filter out command/reasoning items; unfiltered output is enormous.
- Manager's confirmed delivery instruction: integrate validated checkpoints directly into `dev`; no intermediate PR/squash ceremony. Keep unfinished dev-server work off `main` and do not trigger releases. Final promotion to `main` happens after qualification.
- **Token cache:** `/Users/tyler/src/skgo-token-cache` (not a Git repository).
- **Index:** `/Users/tyler/src/skgo-token-cache/index/README.md`.
- **Current research brief:** `/Users/tyler/src/skgo-token-cache/research/dev-experience-20261015/BRIEF.md` (updated to required fast patching; state preservation relaxed).
- Cache `TASK.md` points to that brief and now states required fast patching, with restart fallback. Old task text preserved as `TASK.before-dev-experience-20261015.md`.
- New source subtree: `repos/dev-experience-20261015/`; new web captures: `web/dev-experience-20261015/`; reports: `research/dev-experience-20261015/`; candidate ideas: **cache-root `potential-experiments/`**, not under the research directory.
- Current full tracked SKGO snapshot: cache `repos/dev-experience-20261015/skgo-dev-bd7a4c9/`, from `git archive bd7a4c9`; provenance in `research/dev-experience-20261015/skgo-source.md`.

## Constraints that still apply

Go owns application responses and application I/O; page rendering remains in the existing pooled pure-Go goja runtimes. Node/Vite tooling is allowed and necessary during development. No Node application-rendering sidecar or cgo requirement. Production still uses one Go binary, Node at build time only. Preserve normal Kit/Svelte frontend behavior and use VitePlus (`vp`) for frontend environment operations. Do not implement SKGO-owned tooling as new JavaScript harnesses. The adapter/framework boundary legitimately contains JavaScript.

All application endpoints remain Go. Importing generated throwing JS stubs to inspect metadata is not the same as executing application function bodies or application I/O. This distinction is important and prevented a potentially unnecessary replacement metadata system.

## What was learned from sources

### Vite/Kit/goja connection

- Installed dependencies were provisioned with `vp install` in `example/web`; lockfile stayed unchanged. Kit **3.0.0** with SKGO queue patch, Svelte **5.57.1**, Svelte Vite plugin **7.3.1**, VitePlus Core **1.0.0** containing Vite **8.3.1**.
- Vite provides custom development environments, module-fetch transport, HotChannel, ModuleRunner and an injectable evaluator. Running upstream ModuleRunner in goja is a candidate, NOT demonstrated here. The user subsequently challenged moving Vite responsibilities into Go. No Go port is an assumed fallback: an incompatibility should prompt reconsideration of the integration boundary before duplicating upstream behavior.
- There are distinct Vite server module graphs and per-runtime evaluated-module caches. The user's goal does not require retaining arbitrary internal renderer cache state; coherent output matters.
- Kit `src/runner.js:get_runner()` checks Vite's `isRunnableDevEnvironment`. Earlier assistant wording "Node-runnable requirement" was too broad. Vite documents Runnable as same-runtime live-JS-value transfer; in the usual Vite setup that is Node. It is not intrinsically a dependency on Node APIs. Whether thin functions/proxies/metadata satisfy actual Kit call sites remains an experiment.
- Kit's dev handler imports/configures its server entry and calls `respond`. Its adapter `getRequest`/`setResponse` hooks convert incoming/outgoing HTTP objects; they do not themselves substitute application dispatch. Need to research/reuse a correct tooling/application seam while retaining static precedence, matcher validation, paths, assets and HMR.
- **Important simplification:** Kit's dev remote client transform imports a module and reads `value?.__?.type`. SKGO generates remote modules with throwing callback bodies passed to Kit factories. Source inspection shows query/command factories attach metadata/wrappers without invoking those callbacks. Therefore stock Kit metadata introspection in Node is plausibly compatible with the ownership rules. Baseline probe should reuse it; a custom remote metadata catalog is only a contingency if actual behavior falsifies that path.
- Vite by default dispatches HMR across environments in parallel. Sending an event is not proof a renderer is ready. Investigate the least glue needed to notify the browser after a live patch is installed or a fallback host becomes ready; do not make a general transactional publication framework a requirement.
- Existing adapter `gojaEnvironment()` is build-only (`apply: 'build'`). It is current production infrastructure, not an already-working dev runtime integration.

### Browser behavior

- Kit publicly exports `refreshAll()`; current implementation sets force invalidation and calls `_invalidate(false)`, refreshes active queries/loads/live resources, and restores `page.state`/shallow state. Vite supplies custom event `send`/`on` and `import.meta.hot.on`. These APIs exist; the unproven piece is coordinated supervisor/Vite/Kit wiring and recovery.
- Historical patched-Kit Chrome experiment already demonstrated healthy Go load/query refresh preserving a controlled unsent draft, local counter, shallow state and component/document identities. Do not repeat that as if it had never been tested, and do not promote it to automatic integrated-dev-server proof.
- That experiment showed a failed query boundary still visible after repaired `refreshAll()` resolved. A fulfilled refresh promise is not proof of visible recovery.
- Failed-load repair in that experiment used a browser document reload; it did NOT test a retained public refresh control afterward. Therefore it does not prove reload is necessary or that refreshAll cannot recover a failed load.
- Strict `csr=false` pages contain no script/client to receive in-page notifications. Document navigation/reload is their normal update path; do not silently inject a client and claim unchanged semantics.
- Prior browser evidence: cache `experiments/dev-preflight-20261014/skgo/ephemeral/dev-preflight/kit-refresh/README.md`, with exact limits/captures. It used an explicit control and a real Go handler, not an automatic supervisor/Vite integration.

### Prior art and simplicity limits

- Go: existing cached Air/wgo/fresh/got-reload/hotswap/interpreter/native sources were retained. New source includes olivere/vite, Velocity/framework and installer, templ, Wails v3, and Wails-pinned atterpac/refresh. Full details and pins in `go-prior-art.md` and its `SOURCES.md`.
- olivere/vite is a small asset/tag/manifest helper; it doesn't supervise Vite or Go.
- Wails combines Go/Vite and readiness; refresh has serialized/coalesced supervision, blocking preparation before stopping its primary command, and preserves last-good primary on preparation failure. This may supply supervision and fallback-restart mechanisms; it does not supply the required Go live-patching fast path. Assess direct reuse without substituting rebuild/restart for patching.
- templ distinguishes template text that can be read/reloaded at runtime from Go changes requiring restart, and uses full-page SSE reload. Do not mistake its browser refresh for Kit data/state retention.
- Velocity's actual serve loop stops before build and ignores rebuild errors; its launcher's last-good CLI artifact behavior is a different mechanism. README claims alone would have hidden that distinction.
- Rust Axum hotreload has a small stable state/listener plus swappable handler-generation design. **Caveat:** `Arc<Generation>` is held until `router.call(req).await` yields Response, not visibly through later streamed body consumption. Comments claiming "until last response sent" do not establish streaming-safe retirement. The report was corrected; no streaming tests were found in that checkout.
- cargo-leptos separates CSS/client update paths from server process termination/restart. That can be a useful precedent, not an inferior result merely because Go memory would reset.
- Dioxus/Subsecond is genuine function patching but depends on custom ThinLink/compiler/runtime machinery and has layout/state limits. It is optional background research, not the baseline.
- .NET separates managed compatible deltas, rude-edit restarts, persistent browser services, baseline epochs and replay. Actual complexity includes agents/named pipes/prior deltas. One runtime-rude-edit test is ignored upstream; report records that. Transfer boundaries, not CLR machinery, into Go design.
- JVM HotSwapAgent has runtime/framework/plugin and license constraints; not direct Go reuse.
- Cloudflare's Vite integration demonstrates alternate-runtime transport with actual startup buffering, worker orchestration and restart behavior. Do not copy its large workerd/Miniflare machinery wholesale. It establishes available seams, not a Goja integration proof.

## Source/doc collection completed so far

See `research/dev-experience-20261015/framework-sources.md` and `framework-supplement.md` for exact provenance. Added full tagged upstream repo/docs trees for Kit 3.0.0, Svelte 5.57.1, Vite 8.3.1, VitePlus 1.0.0, Go 1.27.2, Svelte Vite plugin 7.3.1; goja_nodejs, source-map consumer/spec/tests. Existing x/tools v0.52.0 includes full gopls implementation and roughly 100 doc files. Existing exact Kit patched package and VitePlus published bundle remain in cache.

- Readable upstream Vite 8.3.1 tag is distinct from the VitePlus fork/bundled artifact. Do not assume byte identity from the version number.
- Exact cached/installed `module-runner.js` SHA-256 matched: `22f865735c612f2a38df1ee1610366d1b76eff4d62c29eb55a39d933e26740b7`.
- Exact cached/installed patched Kit `src/core/postbuild/queue.js` SHA-256 matched: `400bf34544e2b3c5512e489eb5a0ca99a7483f0b6d733f70cab9e81e28f3a0f5`.
- Those hashes check the named files, not the entire packages.
- The collected goja_nodejs version is already a direct SKGO dependency; its older internal goja requirement alone is not a conflict because Go module selection uses the app graph.
- Upstream source was fetched/read, not installed/executed. New SKGO tests, browser tests and POCs have NOT run.

## Candidate notes currently present

Cache-root `potential-experiments/` contains:

- `README.md` — provisional routing and candidate-only status.
- `supervisor-listener-owner.md` — Go-front vs Vite-front topology comparison.
- `supervisor-host-handoff.md` — candidate readiness/overlap vs stop-first; exclusive-resource and response-lifetime concerns.
- `supervisor-browser-lifecycle.md` — reuse known Kit/Vite APIs for browser notification; parent corrected mistaken "unknown API" wording.
- `browser-refresh-and-recovery.md` — three probes covering automatic updates, diagnostics/runtime recovery, reconnect/navigation/no-CSR.
- `vite-goja-module-transport.md` — upstream runner/module transport candidate, no internal cache-state preservation requirement.
- `vite-http-front-door.md` — Kit-specific dispatch seam, links the separate listener probe.
- `vite-kit-remote-metadata.md` — revised to test stock metadata introspection first; custom catalog contingency only.

**Reconcile these with the latest distinction: fast live patching is required; arbitrary initialized-state retention is not.** Patch probes must demonstrate the fast path and avoid full host relinking for ordinary edits. Fallback-restart probes may reset memory. Same-PID/initializer assertions can diagnose whether a patch actually occurred, but preserving arbitrary app state is not a universal product promise. Do not make background daemon-state restoration a requirement or promote restart-only probes into substitutes for patching.

Several candidate citations use shorthand `frameworks/...` or `example/web/node_modules/...`; the Sol readers must normalize them to source paths actually inside the cache. Existing aliases do not prove portable citation resolution.

## Agents and model assignments at handoff

Native collaboration tools were used; completion messages arrive automatically. No detached CLI agent run or external workflow watcher is active. Inspect `collaboration.list_agents` after compaction for live state; names below can be reused with followup_task.

Completed Luna assignments:

- `/root/source_collection`: official full source/docs collection and provenance.
- `/root/go_prior_art`: initial and expanded Go prior art, corrected old native evidence vs current source.
- `/root/compiled_prior_art`: Rust/.NET/JVM/Cloudflare prior art, then streaming/lifecycle/Node-scope corrections.
- `/root/concepts_supervision`: three supervisor concept files.
- `/root/concepts_browser`: browser/recovery concepts; fixed output path and failed-load overclaim.
- `/root/concepts_vite_runtime`: three Vite concepts; fixed Node-only assumption, safe generated-stub introspection and overstrong renderer-state requirement. Its completion preceded the state-preservation discussion and the explicit correction that live patching remains required.

**Sol 6.1 index update completed:** `/root/semantic_index`, with two Sol source readers.

- Entrypoint `index/README.md` links GOAL first and current `dev-experience.md`. Five source-grounded leaves cover current foundation, frameworks/runtime, Go prior art, compiled prior art, and candidate strategies/experiments.
- The three broadened research maps are integrated as composable, unranked and non-exhaustive alternatives. Current source, historical observations, upstream facilities and unexecuted compositions remain distinct.
- Structural checks reported 23/23 Markdown nodes reachable, 29/29 query targets reachable, 216 new literal source ranges valid, zero missing/outside-cache links. These are structural facts, not answer fidelity or runtime proof.
- Metrics helper reported 22 leaves/617 citations/0 orphans; housekeeping made physical depth 3 while Markdown depth is 1. The eval helper's 29/29 existence-based score is NOT genuine retrieval success. Its estimated reads were 22.34, so a fresh reader rather than metric optimization checks actual usability.
- State was compacted to avoid recursive prior-state nesting. No candidate was executed.
- Fresh Sol 6.1 reader `/root/fresh_index_retrieval` completed five realistic questions from the entrypoint, inspecting cited source slices. Result: `index/.semantic-index/fresh-retrieval-20261015.md`. All five answer areas were retrieved; no broken paths or misleading claims in inspected slices. About 39 file/slice reads across five substantial questions; large concatenated reads caused truncation and a targeted reread. Detailed independent signaling and .NET/JVM/Quarkus sources were outside this bounded check. This is sufficient retrieval qualification, not exhaustive source or runtime proof.

**Broadened Luna research completed:**

- `/root/broad_framework_strategies`: independent framework cooperation map, direct/proxied assets, middleware boundaries, browser updates and alternative renderer freshness mechanisms. Added Vite Ruby and adjacent-source/doc evidence. Fixed the mistaken inference that JS-hosted Vite middleware itself implies Node application SSR.
- `/root/broad_compiled_strategies`: multiple compiled-server combinations, further source-backed Quarkus study, corrected .NET/Vite source pointers. Quarkus provides a distinct compatible instrumentation/app-runtime restart precedent, not a portable Go implementation.
- `/root/independent_strategy_discovery`: alternative update signals (Vite plugin HTTP, post-publication watched marker, polling, Go event channel) and pinned vite-plugin-full-reload source. These are optional mechanisms, not architectural requirements.

All three maps live in cache `potential-experiments/`, linked from its README and the semantic index. Source/doc subtrees retain provenance. They expand the earlier set; no claim that all possible strategies are exhausted is valid.

## Next work

1. STOP index work. The user explicitly says once completeness criteria are met, index building MUST STOP. The fresh source-grounded retrieval check passed its five questions and no consequential miss requires repair. Do not polish leaves, rerun benchmarks, optimize metrics or pursue academic perfection.
2. Report the available research/index and remaining empirical unknowns. The user requested broad alternatives before choosing experiments. No solution or experimental sequence has been selected; compare the maybes before any implementation commitment.
3. Continue exploration if meaningful gaps emerge. Preserve GOAL's four responsibilities and required fast native patching; arbitrary initialized-state preservation is not required. Never convert a candidate runner, transport, topology, lifecycle policy or prior-art mechanism into an architectural requirement.

The requested five-minute in-thread alignment reminder is active as `keep-skgo-research-aligned`. It checks separation of goals/architecture from tactics and breadth of research. It is not an implementation plan.

Suggested retrieval questions (required fast patching, optional state preservation; parent had held these out from index builders):

- Which existing SKGO mechanisms compile and install changed Go bodies without relinking the full host, and which supervisor/Vite connection is still missing?
- Which prior art can keep serving last-good code on patch/build failure and provide fallback restart without making rebuild-on-every-save the baseline?
- Can stock Kit inspect generated remote stubs safely, and where does actual page dispatch still require a different integration?
- Which exact cached source matches installed Kit/VitePlus, as distinct from readable upstream source and current online docs?
- How does Vite invalidation reach an unchanged importer and what remains unproved for pooled goja?
- What does healthy Kit refresh preserve, and what can remain broken despite a resolved promise?
- Does Axum's generation reference cover streamed response consumption or just handler-future completion?
- Which process/browser responsibilities can persist across an application restart without promising Go memory preservation?
- What are the cheapest experiments that could distinguish minimal integration shapes, and which observations already exist?

## Tool/workflow friction to avoid repeating

- `read_thread(turnLimit)` maximum is 10, not 12. Filter its enormous nested output down to human messages; don't dump reasoning/commands.
- In zsh, `path` is tied to PATH. A loop variable named `path` broke command lookup. Use task-specific variable names.
- VitePlus distribution paths are `node_modules/vite/dist/vite/node/...`, not `dist/node/...`.
- Kit patched queue lives at `src/core/postbuild/queue.js`, not `src/utils/queue.js`.
- apply_patch rejects deleting and adding the same file in one patch; use an update.
- Cache itself is not a Git checkout; subdirectories may contain upstream checkouts. Do not attempt to commit or reset the whole cache.
- Luna browser worker initially wrote under research/.../potential-experiments; it was moved to cache-root potential-experiments and the accidental empty directory removed.
- Do not edit upstream sources or execute their setup scripts merely because they were downloaded. Preserve pins/provenance and negative findings.

## Memory use and disclosure

Memory quick pass was used for prior SKGO migration context and semantic-index evaluation pitfalls. Current source/thread evidence superseded stale memory on native qualification. If the eventual final response relies on this memory, include the required memory citation block: `MEMORY.md:25-39` (rollout `01a13cd4-cff4-7e91-a3d4-b1fdff6f10a3`) and `MEMORY.md:1068-1078` (rollout `01a0f069-9512-7910-a8e8-47b7fe8b0880`) are relevant. No user memory files were modified; this handoff is repository/cache working material.

## Latest execution checkpoint

Broad research rounds, Sol index integration and the bounded fresh retrieval check are complete for the current snapshot. Index work is STOPPED at the user's explicit instruction because completeness criteria are met. The solution remains unselected and candidate integrations unexecuted. No application implementation, PR, merge, release or issue mutation occurred. GOAL.md records the four responsibilities verbatim in substance and takes precedence over earlier tactical planning.

## Solution-selection work now authorized and underway

The user asked what is needed from them to try selecting a solution, challenging further waiting. Parent answered that no additional direction is needed and began bounded executable experiments using the completed research. Index work remains STOPPED. No architecture is selected before observing results.

Read-only Luna maps `renderer_experiment_map` and `native_browser_experiment_map` identified reusable current adapter/SSR/native test APIs and historical real-browser fixtures. Two implementation workers now own disjoint experimental paths under `ephemeral/experiments/20261015-vite-cooperation/`: `renderer_delivery_experiments` owns `renderer/`; `native_browser_experiments` owns `browser/` and optionally new `internal/hotreload/browser_experiment_test.go` to reuse package-private fixture APIs. They may not modify production APIs/existing test files. Go orchestration, VitePlus, actual Kit/native code, no fake application values, no Vite rewrite. Parent owns actual browser inspection and final comparative judgment. Workers will report runnable probes, measured results and concrete blockers; losing candidates should not be rescued with large compatibility systems.

The research's alternative families remain available. Comparing feasible mechanisms does not make them requirements. The intended next outcome is an evidence-backed solution recommendation, with uncertainty and unproved areas explicit, not another research inventory or a complete product implementation.


## Executed comparisons and recommendation, 2026-10-15

No further user direction was needed. The parent operated the actual in-app browser against a generated native Go host and stock pinned Vite/Kit. Experimental sources and detailed observations are under `ephemeral/experiments/20261015-vite-cooperation/`; the native harness is `internal/hotreload/browser_experiment_test.go`, behind `skgo_browser_experiment`. Production implementation remains unchanged.

Native patches changed the real Go query 3000 → 2400 → 1800 → 900 → 1200 in one host PID. Candidate preparation (including fresh package analysis) measured 744–799 ms and installation 1.4–1.7 ms on this tiny fixture. Vite custom events followed successful installation; Kit refreshAll and invalidateAll both displayed changed values after approximately 3 ms and retained the exact draft, counter, shallow state and component/document identities. These are individual observations and exclude watcher scheduling; they are not application-sized save-to-paint guarantees. Compile rejection left total1800 and healthy browser state; repair showed900. Document reload showed1200 and reset state/identities. Svelte markup HMR visibly changed FRONTEND_ONE to FRONTEND_TWO, retained document and shallow state, but recreated the component and reset local draft/count. This matches pinned Svelte behavior; do not promise that stock Svelte HMR preserves arbitrary component-local state.

Renderer comparison used actual Kit root/layout/page plus imported dependency and existing pooled goja. Full Kit build and retained ViteBuilder goja-environment rebuild both passed changed-literal rendering checks. Environment-only rebuild observations were roughly42–143 ms across diagnostic runs, with about4–7 ms new pool initialization; this is a tiny static two-node fixture. It does not prove sharing the live dev module graph, dynamic route metadata, CSS/assets/hydration coherence, concurrent pool replacement or whole-app performance.

The unchanged upstream ModuleRunner executes in goja after standard pinned Rolldown CJS lowering removes native dynamic-import syntax. A real transformed module imported successfully. Actual Kit graph traversal first exposed a goja_nodejs URL incompatibility for a NUL-prefixed virtual module file URL. One experiment-only Node-equivalent percent-encoding passed that edge; traversal then reached an externalized OXC async-generator helper and failed because no require implementation exists. No custom module graph, evaluator or general compatibility subsystem was built. Inlining that helper through Vite remains an untested possibility; runner approach is neither proved nor ruled out.

**Recommendation to carry into the full application proof:** compose existing native Go replacement with Vite-owned browser transport and Kit's public healthy-data refresh; retain the last good application on compile/build rejection. Use Vite-produced renderer bundles and fresh goja pools as the currently working comparison baseline. Ordinary frontend edits rebuild no Go binary; Go edits use native patches. Full rebuild/restart and browser document reload remain explicit fallback paths, not the ordinary edit strategy. This is an evidence-backed candidate recommendation, not an added architectural requirement. Keep the upstream runner and direct/proxied asset topologies as alternatives if application-scale results favor them. Do not infer public socket placement from the CSR experiment.

**Remaining join:** browser fixture deliberately used ssr=false with a Vite-produced scaffold; renderer fixture was separate. No complete skgo dev watcher/supervisor, Go-rendered dev-document/bootstrap integration, automatic structural metadata refresh, reconnect/race handling, failed query/load boundary recovery or full-app latency has been proved. The next discriminating work is a coherent full-app edit loop joining these already-tested components, with Go-rendered initial HTML, stock Vite HMR, real Go edit, failed-edit repair and fallback observed in one browser session. Preserve these boundaries in any implementation claim. Index work remains STOPPED. Native browser experiment processes were stopped and ports cleared.

Independent verifier ran the exact renderer experiment once after files stabilized. All four independently supplied component/dependency literal assertions passed; omitting the update build would fail them. Its retained rebuild measurements were43.4/55.1ms, full builds727.7/756.0ms, pool initialization4.0–5.9ms. Simple ModuleRunner import passed; full graph still reported absent require after145ms. Overall test PASS does not qualify that explicitly blocked probe.


## Performance correction: do not accept the integration fixture as the fast-path baseline

User challenged0.8s compilation against yesterday's faster results. They were right to challenge it. Historical native logs were present in the cache: initial HTTP fixture17–26ms compiler /18–27ms edit-to-response; later mapping fixture19–23ms edit-to-HTTP; final fenced fixture17–18ms compiler with226–242ms separate input verification. Parent had used earlier browser work but did not reconcile native performance evidence before reporting the new figure and recommending the composition.

New browser timer labeled the entire Toolchain.Compile operation as compilation. It includes a fresh packages.Load with NeedDeps+NeedSyntax+NeedTypesInfo, reloading dependency analysis per edit, plus classification/lowering before stock compilation. A staged rerun measured one iteration at856.9ms packages.Load versus32.0ms stock compiler, about2ms other preparation and1.9ms installation. The compiler is not exhibiting an0.8s floor. Current primitive lacks the retained-analysis connection that PLAN stage2 already calls for. Corrected runnable browser labels to prepare; original log remains unchanged. The integration experiment remains evidence that native installation can cooperate with Vite and Kit; its preparation latency is an identified inefficiency, not accepted performance. No reason to re-open semantic indexing or invent a new caching framework. Reuse and compare prior retained-input work before selecting implementation.


## User correction: archive, index, and actually use experiment evidence

User explicitly requires ALL experimental data in the token cache, indexed and used, and immediate parallel delegation. Prior instruction to stop index polishing does not excuse leaving newly created evidence unarchived/unindexed. Targeted incremental indexing of new evidence is required; stop after retrieval works. Sol6.1 owns index changes, native experiment worker owns corpus materialization and browser raw observations, renderer worker owns exact raw renderer/independent verification stdout recovery. Corpus destination: /Users/tyler/src/skgo-token-cache/experiments/vite-cooperation-20261015. Parent must retrieve and use old/new performance evidence before further recommendations. No new compatibility/caching implementation is authorized by merely discovering this gap.

The bounded control retained the same overlay but omitted NeedDeps: still260 packages and1486 syntax files, with only3 in the changed root. Pinned go/packages v0.52.0 treats any overlay as invalidating dependency export data. Baseline Load664–857ms vs compiler27–32ms; control Load535–593ms, compiler27–28ms. All six control candidates were installed in one host and exact HTTP1000/2000/3000 values asserted. Changing one flag is not the fix. Reuse retained-analysis work instead of accepting whole-graph reload on each edit. Complete raw runs and source are in the new cache corpus.
