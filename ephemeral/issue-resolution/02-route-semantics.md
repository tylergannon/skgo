# #163 and #159 — route semantics

Complete downloaded issue bodies and comments: [#163](02-route-semantics-163.json), [#159](02-route-semantics-159.json).

## Readiness and mission

Ready to implement; no HITL design choice blocks this group. Real page/endpoint routes must honor Kit's trailing-slash options and select the same route for absent optional parameters and accepted/rejected matcher values in native development and production. Give adapter-specific `config` an explicit supported result or a clear build-time refusal. #163 explicitly authorizes that refusal; it does not require inventing deployment options. #159 is qualification and wording repair: paired Go matchers already exist.

## Token-cache annotations

Read-only retrieval followed `/Users/tyler/src/skgo-token-cache/index/README.md` → `index/upstream-docs/topics.md` / `index/upstream-docs/kit-advanced.md`, plus `index/promise-audit.md` → `promise-audit-2026-10-17/facts/matrix/param-matchers.md`.

- `repos/dev-experience-20261015/frameworks/kit-3.0.0-upstream/documentation/docs/20-core-concepts/40-page-options.md:161-174`: slash values are `never` (default), `always`, and `ignore`; the option also controls prerender output naming. Do not assume the existing default redirect establishes all values.
- The same source, `:176-222`: route `config` is adapter-specific; nested configuration merges at the top level only. This answers the apparent product-design question: there is no generic platform option set to invent for skgo. Current installed Kit semantics, including inheritance and page/endpoint compatibility, remain the builder's specification.
- `repos/dev-experience-20261015/frameworks/kit-3.0.0-upstream/documentation/docs/30-advanced/10-advanced-routing.md:66-70`: optional captures may be absent; optional parameters cannot follow a rest parameter.
- The same source, `:72-157`: Kit 3 matchers return the accepted/parsed value or `undefined`; rejection lets Kit try the next route in sort order before 404. Browser and server must agree. A rejection proof that only expects 404 can miss a broken fallback route.
- `promise-audit-2026-10-17/facts/matrix/param-matchers.md:5-19`: skgo already declares frontend matchers in `params.ts` with paired `func(string) (T, bool)` Go functions; missing Go registrations are refused. JavaScript/Go agreement is the application's contract, not arbitrary JavaScript translation. The audit's historical status records are not a current runtime verdict.

These cached upstream sources are tagged 3.0.0 context. Installed `example/web/node_modules/@sveltejs/kit` is verified **3.1.0** and governs implementation; the builder must map its own relevant source independently.

## Current repository facts

`request_event.go:32-34` defines `ParamMatcher` as the generated adapter for ordinary Go matcher functions. `example/web/src/params.go` supplies a paired `Order` matcher and named `OrderNumber` result. `TestSharedRoutingContracts` covers absent optional captures with rejecting matchers across document/data/remote/hook carriers; `TestTypedLoadMatcherRouting` covers typed values, omission, fallback and optional chains. `TestEndpointMatcherRejectionFallback` positively distinguishes matched and fallback endpoint receipts. These are existing contract seams, not newly executed evidence.

`README.md:228` still says JavaScript matcher functions are not run by Go without explaining the paired implementation; `:238` limits slash support to the default redirect. Repair the matcher wording and update slash wording only to the behavior this group actually establishes. The old README reference to `example/e2e/features/routes.feature` does not resolve in this checkout; use the actual current scenario locations rather than perpetuating that link.

## Winning and proportional proof

Use one representative real routing application across development and a built production server. Its supplied literal receipts must distinguish omission from an accepted value, matched route from fallback route, and a rejected URL with no remaining candidate from either success case. Qualify all three slash values with canonical/noncanonical requests, route selection and the exact Kit redirect behavior, including option inheritance and page/endpoint distinctions where pinned Kit makes them relevant. Demonstrate adapter `config` handling with a concrete accepted fixture or actionable build-time rejection that identifies the unsupported route/configuration.

Status, Location, query retention, response body and build refusal are ordinary Go tests against real handlers/build paths. Kit client navigation needs focused browser scenarios for optional omission and accepted/rejected matcher selection; assert visible positive receipts and the expected route/URL, not only absence or lack of errors. Run those focused scenarios in development and production. Reuse the same production build/output across related assertions; do not build once per case. Inspect the resulting application normally once. Opus then checks the tests are load-bearing, runs focused proof, and reports goal failures or reproducible regressions without adding features or another proof framework.

## Questions resolved and exclusions

- **New matcher engine?** No: paired Go matching already exists; the retained issue is proof and truthful documentation.
- **Which slash behavior?** Kit supplies the values, constraints and precedence. Map the installed source; this is mirroring, not a design interview.
- **Must arbitrary adapter config work?** No: clear build-time refusal is expressly allowed by #163.
- **Scope:** route semantics and their required qualification/documentation only. No reroute support (#167), prerender entry API (#352), validation hook redesign (#166), new runners, evidence manifests, benchmark campaign or repeated full CI loop.
