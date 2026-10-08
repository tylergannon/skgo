# Adversarial review — adapter-owned prerender integration, round 02

Date: 2026-10-07 (local). Reviewer: the same independent session as round 01, read-only except
this file. Round 01: `ephemeral/reviews/202610071627-adapter-owned-prerender-round-01.md`.

## Target

- Design: `ephemeral/plans/adapter-owned-prerender.md` (untracked, current on-disk revision).
- Saved experiment: `ephemeral/research/adapter_load_research_test.go` (md5
  `2fc881e27db9823cb1aa2a88535058d5`) and the new
  `ephemeral/research/adapter_load_research_overlay.json`.
- Worklog delta: `ephemeral/worklog/20261007-generated-load-stubs.md` (three new lines).
- Requested outcome unchanged: generated application modules are typed throwing stubs; the
  prerender load, remote/input, endpoint and hook bridge behaviour belongs to the adapter's
  build-time SSR integration. Planning and review only.

Caller constraints honoured as operating constraints: write only this artifact; no
implementation. No instruction narrowed the subject matter, predicted findings or requested a
verdict.

## Evidence inspected

Everything listed in round 01 was re-read where the plan changed, plus:

- Kit `src/exports/vite/build/remote.js` 1–137: `treeshake_prerendered_remotes` is exactly
  lines 21–137; it looks the remote chunk up by `remote_original_by_hash.get(remote.hash)` and
  `chunk.moduleIds.includes(original_id)`, overwrites each non-dynamic prerender export with
  `prerender('unchecked', () => { throw … })`, removes the default export and re-bundles. Called
  from `src/exports/vite/build/index.js:853`, after prerendering. The plan's new paragraph is
  accurate, and it is a second Kit-side reason the in-memory remote module must sit at the
  original ID (the chunk lookup would otherwise miss and the body would never be stubbed).
- Kit `src/runtime/server/endpoint.js` 14–82: `render_endpoint` method/fallback/HEAD selection
  and the prerender-method guard — as the plan states.
- `internal/adapter/prerender_inputs_build_test.go:127–135, 540–547`: the authored
  `+page.ts` that imports and calls `remoteInputs` in the engine and requires
  `skgo: declared prerender inputs are build-only` — the "retain the existing explicit
  authored-fixture attempt" sentence refers to a real, load-bearing contract.
- `internal/gen/endpoints.go:31` (`endpointMethodOrder` includes `QUERY` and `fallback`), and
  the test's `export const ([A-Z]+|fallback) = fromGo;` regex covers both.

Experiment, run exactly as the plan documents, from the repository root, no repository edits:

```
go test -overlay ephemeral/research/adapter_load_research_overlay.json -tags adapter_prerender_research -count=1 -v ./internal/gen -run '^TestResearchAdapterSuppliesBuildLoads$'
--- PASS: TestResearchAdapterSuppliesBuildLoads (5.23s)
ok  	github.com/tylergannon/skgo/internal/gen	5.454s
```

The relative-path overlay resolves from the repository root as written. The run logged
"7 throwing source loads and 8 hook/remote/endpoint bridge modules" and all six fixture
contracts `--- PASS` with no `--- SKIP`. The test now (lines 189–236) snapshots the whole
`ui/src` tree after its own rewrites and before the build and requires identical paths and
bytes afterward; (242–252) requires `skgoPrerenderLoad`, `skgoPrerenderEndpoint` and
`skgoRequestHandle` in Kit's `.svelte-kit/output/server/**/*.js`; (253–264) requires none of
the four bridge symbols in `build/ssr/**/*.js` or `build/client/**/*.js` from the same build;
(271–278) requires each named contract to have both run and passed. Each of those is anchored
in something the test supplied or in literal Go-produced strings, not in the system under
test. Round 01's kept-output inspection (bridge symbols only under Kit's `ssr` output; Go data
in `build/prerendered/lifecycle/alpha.html`) still describes this build.

## Round 01 findings — status

1. Endpoint bridge omitted from scope — **resolved.** Scope now names server endpoints; "Kit's
   existing seam" cites `endpoint.js`; a `+server` paragraph, acceptance ("endpoint
   methods/fallback"), and the experiment description (four endpoint modules, eight variants)
   all agree with the test.
2. Handoff prescribed methods — **resolved.** "Required build boundary and demonstrated
   approach" now separates Kit-derived constraints (original IDs, root-relative identity,
   physical hook discovery, no service in `configResolved` with its skgo reason) from the
   prototype's choices (load vs transform hook, metadata carrier — "this plan does not require
   extending `skgo.remotes.json`", marker form), and leaves the latter to the builder.
3. Byte-stability overstated — **resolved** (whole-tree snapshot, verified above).
4. Vacuous denial proof — **resolved.** Acceptance now requires positive presence in Kit SSR
   output and absence in client/goja output of the same build, keeps the authored helper-call
   fixture, and says "merely having no generated helper import is not proof of denial".
5. Inline `ActionFailure` import — **resolved** ("named `ActionFailure` type imports in
   TypeScript"; "Load/action coexistence must check those types too").

## Findings

No material findings remain. Two genuine nitpicks:

### 1. Nitpick — `getRequestEvent` is a bridge-only import that the forbidden list does not name

`internal/gen/emit.go:73` adds `getRequestEvent` to the `$app/server` import solely because
`prerenderFromGo` (168–170) calls it. The plan's "Source result" enumerates `building`,
`buildLoad`, `prerenderFromGo`, endpoint `fromGo`, platform assertions, input-producer
closures, bridge imports and inline import expressions, but not this import, and the research
fixture's regex surgery left it in place (`import { getRequestEvent, prerender } from
"$app/server"` in the kept `state.remote.ts`). An implementer who removes exactly the listed
items would leave an unused runtime import whose only purpose was the bridge. Name it, or
phrase the rule as "imports used only by removed bridge code".

### 2. Nitpick — the load-stub self-check in the experiment is weaker than the module-level one

`adapter_load_research_test.go:72` checks the rewritten load stubs for `buildLoad` and
`platform` only, while line 130 checks the other modules for `building` and the adapter
specifier as well. If the import-stripping regex at line 68 ever failed, a dangling
`import { building } from '$app/env'` would survive the check (Vite's TS transform does not
type-check, and the body no longer references it, so the build would still pass). Research
code only; worth aligning if the test is kept as the reference experiment.

## Verified and not contested

Every Kit citation added since round 01 (`build/remote.js:21-137`, `endpoint.js:14-82`) is
accurate for the installed 3.0.0 pin. The documented reproduction command works unmodified.
The plan now correctly treats post-crawl tree-shaking as the reason remote forwarding is proved
by literal remote artifacts rather than by a callback in final server JavaScript. The handoff
leaves the metadata carrier and the Vite hook choice to the builder while keeping the
constraints Kit actually imposes.

## Outcome

only nitpicks remain
