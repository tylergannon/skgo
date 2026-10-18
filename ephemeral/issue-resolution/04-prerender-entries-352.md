# #352 — Go-owned prerender entries

Complete downloaded issue body and comments: [04-prerender-entries-352.json](04-prerender-entries-352.json).

## Readiness and mission

Ready for implementation; no HITL decision blocks it. A parameterized route whose catalog belongs to Go can declare its entries in Go, with Kit consuming those entries during its normal build. Follow the issue's proposed `skgo.Entries` declaration surface and route-param typing, while leaving implementation choices and installed-pin mapping to the builder. Avoid a committed duplicate JSON catalog, manual preparatory generation gesture, or application work in JavaScript.

The owner explicitly removed the documentation-site migration as a dependency. Do not modify skgo-project or recreate its old markdown catalog workflow. Go-owned stores/catalogs remain the use case.

## Read-only cache evidence

Retrieval followed `/Users/tyler/src/skgo-token-cache/index/README.md` → `index/upstream-docs/topics.md` and `index/promise-audit.md` → `promise-audit-2026-10-17/facts/matrix/prerendering.md`.

- `repos/dev-experience-20261015/frameworks/kit-3.0.0-upstream/documentation/docs/20-core-concepts/40-page-options.md:87-117`: `entries` enumerates parameter sets for a dynamic route and may be exported by `+page.js`, `+page.server.js`, or `+server.js`; it may be async and retrieve a catalog from a store. This is route discovery for prerendering, distinct from remote-function input enumeration.
- The same source, `:79-85`: missing dynamic routes may be supplied by explicit entries or crawling; a fixture intended to prove Go entries must not accidentally discover the same pages by links or a frontend entries list.
- `promise-audit-2026-10-17/facts/matrix/prerendering.md:3-5`: the historical source account already describes the authenticated Go build bridge for loads/remotes/endpoints and real Kit-build/artifact-serving checks. Reuse existing ownership and proof seams; this is not a new prerendering engine. Historical retained statuses and deployed-site checks are not today's implementation proof and do not make a deployed documentation migration part of this issue.

Cache sources are context at their recorded identities. Installed Kit **3.1.0**, verified in `example/web/node_modules/@sveltejs/kit/package.json`, is authoritative. The builder maps its own analyse timing, entry generation/matcher validation, and export precedence before implementation.

## Current facts and settled questions

Installed `src/core/postbuild/analyse.js:132-133` selects `page?.entries ?? endpoint?.entries`; `:200-207` retains endpoint `mod.entries`; `:225-226` selects `leaf.universal?.entries ?? leaf.server?.entries`. Thus a server export is a valid hook point and universal page entries must retain priority. Page/server/endpoint coexistence must follow these semantics rather than invoke every catalog indiscriminately.

`prerender_service.go`, `prerender_request.go`, `internal/gen/prerender_command.go` and `internal/adapter/skgo-adapter/prerender.js` already own build-operation lifecycle and callback transport. `example/server_test.go:193` contains `TestDynamicPrerenderEntriesAreServedFromFiles`. These are current seams, not a prescribed new operation or proof that analyse-phase calls already work.

The issue's questions—whether the service is ready at analyse time and how invalid matcher entries fail—are code/source qualification questions, not user product choices. Resolve them by executing a real pinned build. An entry rejected by its route matcher must fail as Kit does, with the offending route recognizable. A Go enumeration error must fail the actual `vp build` and name the route. Do not silently drop invalid entries or turn a build error into an empty catalog.

README's prerender matrix row and later limitation list still reject Go-load prerendering; source page-option docs also say entries are unchanged/frontend-only. Correct the stale claims within this feature and document the Go declaration accurately. If documentation source changes, render it using the existing authoring contract. No deployed-site refresh is required here.

## Winning and proportional proof

The real example includes a parameterized `[slug]` page with a Go-owned catalog, no alternate TS entries list and no crawl links that could independently produce the same pages. For a fixture-supplied list, every listed HTML and `__data.json` artifact exists and the real production handler serves its contents byte for byte. Assert the supplied page receipts too; comparing two empty/wrong artifacts is insufficient. Check an unlisted catalog control does not appear by accident.

Use ordinary Go tests for declaration/generation rules, callback values, error context, and final artifact serving. A real `vp build` must establish analyse-time availability, precedence, matcher refusal, and route-naming enumeration failure; reuse one successful built fixture/output across its artifact assertions and keep failing-build cases focused. If endpoint declarations are exposed as proposed, qualify their entries at the same boundary. This is build/HTTP behavior, so new browser machinery is unnecessary; a normal look at the resulting example page is useful product inspection, not another runner.

Opus verifies these checks are load-bearing, executes focused proof, and reports only missed goal behavior or reproducible regressions. Reuse captured build output instead of repeatedly running expensive builds after commentary-only review. New catalogs, documentation-site migration, remote-input API changes, measurement pipelines, benchmark suites, and evidence manifests are excluded.

## HITL question from installed-source mapping

The existing generated `RouteParams` is a request view with private fields/accessors. Matched values are converted Go domain values; Kit entries need raw strings and there is no general inverse matcher. Thus the issue's literal `[]RouteParams` proposal is neither publicly constructible nor a complete raw-entry representation.

Pending API choice: generate an exported per-route `EntryParams` struct with raw string fields (optional values as pointers), retaining typed route field names; or accept raw `map[string]string` records, giving up generated route field typing. The common build bridge/lifecycle work can proceed while this surface choice is answered.

HITL answer: user selected **Generated EntryParams struct (recommended)**. Generate exported raw entry fields separately from request `RouteParams`; preserve Kit's raw matcher-input semantics.
