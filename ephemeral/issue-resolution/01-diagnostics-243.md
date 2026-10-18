# #243 — useful development diagnostics

Full issue body and every downloaded comment: [01-diagnostics-243.json](01-diagnostics-243.json). The historical reproduction is v0.16.3; the 2026-10-18 owner comment explicitly requires a new real-session reproduction on the native bridge.

## Readiness

Ready for implementation. No HITL product decision remains. The issue already fixes the goal (authored Go error and source frame, recovery, development only), and the repository fixes the architecture (native Vite/Kit development rendering; Go application callbacks). The builder must map the installed Kit 3.1.0 error behavior before choosing the delivery mechanism. Do not assume the historical opaque HTML response is itself a violation: Kit deliberately sanitizes unknown public error bodies while reporting the underlying exception through its diagnostics.

## Token-cache annotations

Research followed `/Users/tyler/src/skgo-token-cache/index/README.md` → `index/dev-native-kit-uds.md`, `index/dev-framework-runtime.md`, and `index/upstream-docs/topics.md`; the cache was read-only.

- `index/dev-native-kit-uds.md:3-10` identifies native Kit development SSR, Go application behavior, private Unix sockets and the public Go proxy as the selected architecture. Older goja development/source-map proposals are historical and must not drive this repair.
- `repos/dev-experience-20261015/frameworks/kit-3.0.0-upstream/documentation/docs/30-advanced/25-errors.md:88-105` distinguishes unknown errors' private message/stack from the generic public error body and identifies `handleError` as the error-reporting boundary. This is historical upstream 3.0.0 documentation, useful context rather than authority over the installed 3.1.0 implementation.
- `experiments/product-structure-20261015/OBSERVATIONS.md:3-5` records a real development command where malformed Go code showed an authored file/line and repair cleared the Vite overlay. It demonstrates a prior compile-error observation; it does **not** establish returned-load-error or runtime-panic diagnostics under today's native bridge. Do not expand this issue into another HMR architecture or state-preservation project.
- `index/dev-framework-runtime.md`, section “Svelte HMR and error repair boundaries”, points to old Kit source noting that an emitted error event does not establish that the overlay was visible. Inspect the actual developer experience rather than claiming browser success from transport alone.

## Current repository facts

Installed `example/web/node_modules/@sveltejs/kit/package.json` reports **3.1.0**, as required by `ephemeral/sveltekit-current/SKILL.md`.

`tracing_native.go:269-287` defines a kind/message/status diagnostic and makes it conditional on tracing being enabled. There is no authored source frame in that payload. `internal/adapter/skgo-adapter/dev-bridge.js:220-253` rethrows native errors while preserving the already handled Go public body; its Go-error hook returns that body without logging the underlying error. These are concrete seams to investigate, not a prescribed repair. `load.go:472-515` recovers load panics and captures a Go stack; preserving ordinary recovery remains required. The owner triage describes a plausible current gap, but source inspection alone is not a current reproduction.

## Executable goal and bounded proof

A developer using the real current `skgo dev` command with tracing **disabled** can diagnose both an ordinary returned Go page-load error and a panic: each exposes its supplied unique literal and a useful authored route `.go` filename/line in the normal development diagnostic channel. Generated `internal/skgo/links/...` identity alone is insufficient. The failing requests return 500 and do not execute the page's success markup. A subsequent healthy request returns a fixture-supplied literal receipt through the same development session. Repair the authored failure and observe the route become usable again through normal Kit/Vite behavior.

Use ordinary Go handler tests for the diagnostic/recovery contract, anchored in supplied literals and authored fixture locations. For what only the development integration proves, start a real session and inspect its developer-visible diagnostics and recovery; `internal/devserver/session_integration_test.go` is an existing opt-in integration seam, not a mandate for a new runner. Retain a focused production non-disclosure check so the change does not expose the authored literal or stack in production responses. Reuse focused checks after fixes; one broad regression run belongs at the appropriate integration boundary, not after every review comment.

Opus validation must verify those assertions would fail if the diagnostic or recovery were removed, run the focused proof, and report only goal failures or reproducible regressions. Browser overlay redesign, new diagnostic products, production disclosure, general HMR/state preservation, performance work and proof machinery are outside this issue.

## Design questions considered

- **Which runtime owns development rendering?** Answered by the repository architecture exception and the cache's current-direction route: Vite/Kit.
- **Must this also change production error handling?** No. The issue explicitly excludes production disclosure; preserve Kit's public error semantics.
- **Does tracing need to be enabled to get development diagnostics?** No. Ordinary developer errors must remain diagnosable without an observability opt-in; the current dependency is part of the suspected gap.
- **Must the generic public 500 page contain the raw diagnostic?** The goal is useful developer-visible authored diagnostics. Map pinned Kit before implementation and preserve its public-body/private-diagnostic distinction; a new error-page design is unnecessary.
