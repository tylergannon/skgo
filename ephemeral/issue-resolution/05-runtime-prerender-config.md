# Group 5: Shared runtime/prerender configuration

## #278 Carry application HandleFetch and HandleError configuration into prerendering

https://github.com/tylergannon/skgo/issues/278

RequestEvent final review found runtime/build divergence: prerender_request.go creates requestFetcher without its runtime HandleFetch hook, and PrerenderServiceOptions exposes neither HandleFetch nor HandleError. The generated build entry binds the selected request middleware but does not import the runtime fetch/error configuration. Applications relying on fetch URL rewrites, stamped response headers, or error shaping can therefore produce different static content. The generated Kit handleFetch currently relays cookies only. Add a small explicit/shared configuration path and real served/build parity contracts. No in-tree prerender page currently exercises the affected runtime hooks; accepted nonblocking finding2 in ephemeral/reviews/20261007-request-event-stage6-opus.md.

<!-- agent-session-group-20261018 -->
## Coordinated agent session: Prerender application configuration

Work together: #278, #275.

Application fetch/error hooks and default serialized-response-header filtering have an explicit reusable configuration path shared by served rendering and the generated prerender service. The same application configuration produces matching build-time and runtime behavior without duplicating registration wiring. Keep ownership coherent across both issues; do not claim runtime hook support closes the build-time gap.

Use the installed pinned Kit implementation as the specification. The session is complete only when both issues' retained acceptance criteria are satisfied; partial delivery does not close the other issue.

### Comment by tylergannon at 2026-10-18T16:58:02Z

**Triage — 2026-10-18**

Checked against current [`main` at `920b6aca`](https://github.com/tylergannon/skgo/tree/920b6aca738a65d6c046a67f4db2373596fafd03). Classification: `bounded-hard`.

Still applicable: PrerenderServiceOptions in prerender_request.go exposes BindRequest, Endpoints, Matchers and FilterSerializedResponseHeaders, but no HandleFetch or HandleError. Runtime and native development have those hooks, so their existence does not close the build-time gap. A shared explicit configuration path and literal served-versus-prerendered behavior provide a clear endpoint.

<!-- skgo-issue-triage-20261018-920b6aca -->
## #275 Make renderer header-filter defaults straightforward to share with prerendering

https://github.com/tylergannon/skgo/issues/275

RequestEvent stage 5 supports PrerenderServiceOptions.FilterSerializedResponseHeaders and an authored prerenderPackage entry. The default generated build entry cannot discover a filter supplied only to runtime SSROptions, so served and prerendered hydration headers can differ unless applications explicitly share configuration. The authored entry duplicates generated registration wiring. Provide a small shared configuration path or clearly documented hook-based ownership without migration scaffolding. Demonstrate real served/build parity and preserve transport, matchers, boundary and address behavior. Nonblocking finding 2 in ephemeral/reviews/20261007-request-event-stage5-opus.md; current starter/example set no renderer filter.

<!-- agent-session-group-20261018 -->
## Coordinated agent session: Prerender application configuration

Work together: #278, #275.

Application fetch/error hooks and default serialized-response-header filtering have an explicit reusable configuration path shared by served rendering and the generated prerender service. The same application configuration produces matching build-time and runtime behavior without duplicating registration wiring. Keep ownership coherent across both issues; do not claim runtime hook support closes the build-time gap.

Use the installed pinned Kit implementation as the specification. The session is complete only when both issues' retained acceptance criteria are satisfied; partial delivery does not close the other issue.

### Comment by tylergannon at 2026-10-18T16:57:45Z

**Triage — 2026-10-18**

Checked against current [`main` at `920b6aca`](https://github.com/tylergannon/skgo/tree/920b6aca738a65d6c046a67f4db2373596fafd03). Classification: `bounded-hard`.

The explicit workaround still exists: PrerenderServiceOptions has FilterSerializedResponseHeaders, and internal/gen/prerender_resolve_options_test.go exercises a custom prerenderPackage sharing that filter. This is not missing filter support; the remaining issue is ergonomic ownership/discovery for the default generated build entry. Keep the change limited to shared defaults or a clearly supported configuration path, with served/build parity, and coordinate with #278.

<!-- skgo-issue-triage-20261018-920b6aca -->

## Initial orchestration assessment

Goal: one explicit reusable application configuration path feeds served rendering and the DEFAULT generated prerender service, carrying application HandleFetch, HandleError, and serialized-response-header defaults without an authored entry duplicating generated registration. Preserve existing transport, matchers, request boundary and client-address behavior.

Proof: actual real build versus served runtime literal parity for fetch rewrite/stamped content, public error-hook fields and filtered serialized headers. The default generated entry must consume shared configuration; merely adding runtime fields or using the old custom-main workaround does not prove this group. Existing per-request header override semantics and registration facilities remain covered. Use ordinary Go build/handler contracts.

Potential design question: what small Go-owned explicit configuration boundary best mirrors Kit's application hook/default ownership while fitting the existing generator configuration? Map pinned Kit and cache/current generator before deciding; no new registration framework or migration machinery. Both issues already authorize a small shared path, so routine API details are implementation judgment unless research uncovers a material product choice.

## Token-cache annotation: both issues executable

Research entered through the [cache index](/Users/tyler/src/skgo-token-cache/index/README.md), its [hook ownership route](/Users/tyler/src/skgo-token-cache/index/upstream-docs/topics.md:57) and [resolve-option reference](/Users/tyler/src/skgo-token-cache/index/upstream-docs/kit-reference.md:47). Cached [handle documentation](/Users/tyler/src/skgo-token-cache/repos/dev-experience-20261015/frameworks/kit-3.0.0-upstream/documentation/docs/30-advanced/20-hooks.md:19) explicitly covers runtime and prerender requests; cached [handleFetch documentation](/Users/tyler/src/skgo-token-cache/repos/dev-experience-20261015/frameworks/kit-3.0.0-upstream/documentation/docs/30-advanced/20-hooks.md:105) likewise includes prerendering. These older docs explain the shared ownership; current authority remains the installed Kit 3.1.0 source below.

- **Kit owns application hooks once, not independently by serving mode.** Installed [server initialization](/Users/tyler/.codex/worktrees/e413/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/server/instance.js:52) gets the application's hook module and installs handle, handleError, handleFetch and transport. [create_fetch](/Users/tyler/.codex/worktrees/e413/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/server/fetch.js:31) invokes that handleFetch before its normal fetch continuation; [error conversion](/Users/tyler/.codex/worktrees/e413/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/server/errors.js:21) invokes that handleError and merges its public fields, except already-handled errors. Redirects remain outside error conversion. The same ownership applies while Kit builds.
- **Header serialization is a resolve option with per-request ownership.** Installed [respond defaults](/Users/tyler/.codex/worktrees/e413/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/server/respond.js:54) filter every header by default; [resolve](/Users/tyler/.codex/worktrees/e413/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/server/respond.js:590) accepts the handle-selected predicate. SKGO already adds a renderer default in [SSROptions](/Users/tyler/.codex/worktrees/e413/skgo/document.go:103); its [document options](/Users/tyler/.codex/worktrees/e413/skgo/document.go:795) prefer an explicit request predicate. The [prerender fallback](/Users/tyler/.codex/worktrees/e413/skgo/prerender_request.go:279) uses the service default only if the request chose none. Share that existing default without reversing precedence, combining predicates, or broadening serialized headers.
- **The default generated entry is the missing discovery boundary.** Current [prerender command generator](/Users/tyler/.codex/worktrees/e413/skgo/internal/gen/prerender_command.go:10) directly wires generated loads/remotes, conditional transport, endpoints, matchers, the selected request boundary, and an intentionally unavailable build client address. Current [declared configuration](/Users/tyler/.codex/worktrees/e413/skgo/internal/gen/declared_config.go:39) explicitly selects Go locals/hook package/symbol; it does not discover arbitrary runtime constructor locals. A filter or hook captured only inside NewHandler cannot be inferred safely. A small explicit Go selection is within the authorized issue scope; its API spelling and validation are builder judgment.
- **The workaround proves available semantics, not the desired ownership.** Existing [shared-renderer-default test](/Users/tyler/.codex/worktrees/e413/skgo/internal/gen/prerender_resolve_options_test.go:56) requires real prerendered `x-default: default-literal` and a served filter check, but uses an [authored buildservice main](/Users/tyler/.codex/worktrees/e413/skgo/internal/gen/testdata/prerender-production/buildservice/main.go:12) repeating generated registrations. That does not satisfy either issue's default-generated-entry requirement.
- **Hook wiring must reach the real operation, not merely the option struct.** Current [PrerenderServiceOptions](/Users/tyler/.codex/worktrees/e413/skgo/prerender_request.go:21) has the filter but no fetch/error hooks; [generated build hooks](/Users/tyler/.codex/worktrees/e413/skgo/internal/adapter/skgo-adapter/prerender-modules.js:119) delegate handle/fetch into the bridge. Build [requestFetch](/Users/tyler/.codex/worktrees/e413/skgo/internal/adapter/skgo-adapter/prerender.js:889) currently relays cookies, and load/remote operations execute their Go callbacks through the build service. The new shared configuration must be observed in real build results for the chosen fetch/error cases, including hook-only public fields and safe private-error handling. Production hook fixes from group 3 do not by themselves close this build gap.

Question resolved: Kit settles hook ownership and resolve-option semantics; the repository already has explicit Go package/symbol selection and the underlying default/override behavior. **#278 and #275 are executable together now; no HITL product decision is missing.** Choose one small explicit reusable Go configuration path rather than infer runtime values or author another registration entry. Real build/served literal parity must run through the default generated command, and existing transport/matcher/request-boundary/address/override contracts must remain valid. This does not authorize migration machinery, a new registry framework, repeated full-suite runs, or changing #272's separately established build-fetch capability boundary.
