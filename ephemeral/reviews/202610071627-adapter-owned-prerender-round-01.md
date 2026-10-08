# Adversarial review — adapter-owned prerender integration, round 01

Date: 2026-10-07 (local). Reviewer: independent session, read-only except this file.

## Target

- Design: `ephemeral/plans/adapter-owned-prerender.md` (untracked, as on disk at review time).
- Saved experiment: `ephemeral/research/adapter_load_research_test.go` (untracked; the file
  changed on disk during the review — the version reviewed is the one that substitutes
  hook, remote **and** endpoint modules, `len(buildVariants) != 8`, md5
  `c8b119945ef9bf2cc2160c0b6898644b`). The earlier `ephemeral/research/adapter-owned-prerender.patch`
  (committed in 3d112a4) predates the endpoint handling.
- Requested outcome (from the launch prompt and the plan): generated application modules are
  typed throwing stubs; the present prerender load, remote/input and hook bridge behaviour
  moves to the adapter's build-time SSR integration. Planning and review only.

Caller constraints honoured as operating constraints: write only this artifact; no
implementation. No instruction narrowed the subject matter, predicted findings or requested
a verdict, so nothing was ignored.

## Evidence inspected

Repository instructions: `CLAUDE.md`, `ephemeral/sveltekit-current/SKILL.md`,
`ephemeral/worklog/20261007-generated-load-stubs.md`.

Current implementation: `internal/gen/loads.go` (load stub emission, `building`/`buildLoad`
bridge at 102–201, inline `import('@sveltejs/kit')` types at 170–201 and 260),
`internal/gen/emit.go` (remote emission; `prerenderFromGo` 168–170, `$skgoRemoteInputs`
85/355–357), `internal/gen/endpoints.go` (`building`/`skgoPrerenderEndpoint` bridge 106–125),
`internal/gen/prerender_command.go` (`writePrerenderHook`, authored-hook prohibition,
`hasPrerenderWork`), `internal/gen/emit.go` `remoteList`/`writeRemoteList`,
`internal/adapter/skgo-adapter.js` (plugin wiring 55–135: `prerenderOwnerPlugin` with
`apply: 'build'`, `configResolved`, awaited `buildApp` pre hook; `emulate().platform`),
`internal/adapter/skgo-adapter/prerender.js` (`remoteInputs`, `remoteLoad`, `remoteFunction`,
`requestHandle`), `internal/adapter/skgo-adapter/env.js` (goja environment, `consumer:
'server'`, `'skgo:prerender'` throwing substitute for `@skgo/sveltekit-adapter/prerender`),
`internal/gen/prerender_production_fixture_test.go`, `internal/gen/main_test.go`,
`internal/gen/testdata/prerender-production/server_test.go`.

Pinned Kit (`example/web/node_modules/@sveltejs/kit`, `package.json` version `3.0.0`, pnpm
patch `skgo-kit-3.0.0-queue.patch` touching only `src/core/postbuild/queue.js`): every line
citation in the plan was checked and holds —
`src/exports/vite/index.js` 614–616 (adapter `pre` plugins precede Kit's plugins) and 267–277
(`root = resolve_root(config)`); `src/exports/vite/build/index.js` 449
(`builder.build(builder.environments.ssr)`), 794 (`prerender(...)`), 909 (`adapt(...)`);
`src/exports/vite/plugins/remote.js` 97–154 (`transform` filtered by `remote_module_pattern`,
`hash(path.relative(root, id))`, self-import by basename, chunk emission keyed by hash);
`src/exports/vite/utils.js` 144/158 (`remote_module_pattern = /[/.]remote\.[^/]+$/`);
`src/core/sync/write_server.js` 68–110 (`resolve_entry(config.files.hooks.server, …)`);
`src/runtime/server/page/load_data.js` 22–36; `src/runtime/app/server/remote/prerender.js`
1–130; `src/core/postbuild/prerender.js` 176 (`adapter.emulate()`);
`src/core/postbuild/analyse.js` 150–178 (remote export analysis, `validate_server_exports`);
`src/utils/exports.js` 76–98; `src/exports/vite/build/build_server.js` 168–173.

Experiment, reproduced without editing the repository, via `go -overlay`:

```
go vet  -overlay <scratch>/overlay.json -tags adapter_prerender_research ./internal/gen   # exit 0
go test -overlay <scratch>/overlay.json -tags adapter_prerender_research -count=1 -v ./internal/gen -run '^TestResearchAdapterSuppliesBuildLoads$'
--- PASS: TestResearchAdapterSuppliesBuildLoads (5.14s)   # all six fixture contracts "--- PASS", no "--- SKIP"
```

A scratch copy of the same test (renamed, plus a `cp -R` of `ui/build`, `ui/.svelte-kit`,
`ui/src`, `vite.config.ts`, `skgo.remotes.json` into the scratchpad before the contracts run)
also passed (5.48s). Inspection of the kept output:

- Bridge symbols (`skgoPrerenderLoad|skgoPrerenderRemote|skgoPrerenderEndpoint|skgoRequestHandle|requestHandle|sveltekit-adapter/prerender`)
  occur in 12 files, all under `.svelte-kit/output/server/` (Kit's `ssr` environment);
  **zero** occurrences under `build/ssr` (goja bundle) or `build/client`. The plan's
  environment-scoping claim is demonstrated, not just asserted.
- `build/prerendered/lifecycle/alpha.html` contains `Go after hook: /lifecycle/alpha
  cookies:scoped%20raw deleted cookie-forwarding:root` and two `remote-13` occurrences —
  Go-produced load, remote and hook effects landed in prerendered output.
- Source stubs on disk after the build: `hooks.server.ts` is `export {};`;
  `+page.server.ts` is `import type { RequestEvent } …; export const load = async (event:
  RequestEvent): Promise<{ ready: string }> => { throw … }`; `+server.ts` is per-method
  `RequestHandler` throwers; `.remote.ts` prerender bodies are `unimplemented()` with no
  `inputs` option.
- The compiled `_page.server.ts.js` entry calls `event.platform.skgoPrerenderLoad("src/routes/lifecycle/[slug]/+page.server.ts", …)` — the root-relative identity rule works.
- No `+page.server.ts` in the fixture exports `actions`; load/action coexistence is not
  exercised (the plan says so).
- The test's byte-stability check covers the 15 files it rewrote out of 76 under `src/`.

## Findings

### 1. Issue — `+server.ts` endpoint bridge is outside the plan's scope, contradicting its own developer outcome and its saved test

Evidence. The plan's first sentence: "Generated application server modules contain typed
throwing stubs, not build bridges." Its scope sentence: "The scope includes server loads,
prerender remote functions with and without declared inputs, and the generated server hook."
`internal/gen/endpoints.go:106–125` emits `import { building } from '$app/env'` and a
`fromGo` body reading `event.platform.skgoPrerenderEndpoint` into every generated
`+server.ts`/`+server.js` — the same build-only-behaviour-in-runtime-source the worklog
correction rejects ("The user rejects build-only behavior in generated runtime source files").
The saved test now *does* replace endpoints (`entry.Name() != "+server.ts"` branch,
`len(buildVariants) != 8` "one hook, three remote, and four endpoint build variants"), and the
adapter already carries `remoteEndpoint`/`skgoPrerenderEndpoint`. The plan's description of
the experiment ("an empty hook marker, and three remote source modules without build bridges";
"the prototype keeps old hook/remote module text") is now stale against the file it names.

Impact. An implementer following the scope sentence can ship `+server.ts` still importing
`$app/env` and branching on `building`, declare the outcome met, and pass an acceptance
section whose "absence of bridge code" contract is only defined for the three listed kinds.
Either include endpoints in scope (the experiment shows the same mechanism works for them and
`TestProductionPrerenderedEndpointsAndPages` already covers `/lifecycle-api` and
`/lifecycle-cookies/*`) or state the exclusion and why. Then fix the experiment description.

### 2. Issue — the handoff prescribes methods the repository instructions reserve for the builder

Evidence. `CLAUDE.md` "Delegate missions, not methods": "Sprint documents obey the same rule:
mission, acceptance, ownership. No file lists, no prescribed designs, no step-by-step." The
plan's "Selected integration" and "Source result" sections fix: the hook (`apply: 'build'`,
`environment.name === 'ssr'`, in-memory supply "at the original module IDs"), the metadata
carrier ("Extend the generator's existing build metadata" — i.e. `skgo.remotes.json`, a file
the Go binary also reads at startup per `emit.go` `remoteList`), the hook marker form ("An
empty generated `hooks.server.ts` or `.js` marker"), "No service startup in configResolved",
and the exact identity rule. Several of these are Kit-anchored constraints with their reason
given (plugin order, `write_server.js` discovery, remote ID computation) and belong in a
handoff. Others are prototype-derived choices stated as mandates without a Kit reason: the
metadata living in `skgo.remotes.json` rather than a build-only file, the marker being empty
rather than carrying type-only content, and the "in memory at the original module IDs" shape
versus any other way of reaching the same compiled output. The closing "Implementer handoff"
then says "Choose implementation details from the pinned source and existing code", which the
earlier sections have already foreclosed.

Impact. The instruction exists because prescriptions "cap an agent's quality at the
dispatcher's understanding and transmit the dispatcher's unverified assumptions as fact";
here the assumptions are from a single prototype that did not exercise JavaScript mode,
load/action coexistence or dev. Separate the Kit-derived constraints (keep, with citations)
from the prototype's choices (demote to "the experiment did X; the builder decides").

### 3. Nitpick — "requires every source module's bytes to be unchanged afterward" overstates the experiment

The test compares only the files it rewrote (`stubs`: 7 loads + 8 variants = 15 of 76 files
under `src/`) against its own writes. That anchor is independent of the build and fine for
what it claims, but the plan's sentence promises more than the test checks. Either widen the
check to a hash of the whole `src/` tree taken before the build, or soften the sentence.

### 4. Nitpick — the engine's build-helper denial becomes a vacuous proof once nothing imports the helper

Acceptance asks to prove the plugin does not supply bridges to goja "including the existing
deliberate build-helper denial in the engine" (`env.js` `'skgo:prerender'` →
`remoteInputs() { throw … }`). After the change no generated application module imports
`@skgo/sveltekit-adapter/prerender`, so a contract anchored on that substitute cannot fail for
any generator regression — it is the "X is absent" shape `CLAUDE.md` warns about. The kept
build already gives the load-bearing form: assert the bridge symbols are present in Kit's
`ssr` output and absent from `build/ssr` and `build/client` of the same build. Keep the
denial as a guard; do not count it as proof.

### 5. Nitpick — the inline-import rule in "Source result" also hits the action stubs, which the plan never names

`loads.go:260` emits `import('@sveltejs/kit').ActionFailure<…>` inline into TypeScript
`actions` signatures, and `loads.go:170–201` the same pattern for `RequestEvent`. The
"Source result" rule ("no … inline TypeScript import expressions remain in generated
application source") covers both, and the worklog correction is the origin, but only loads are
discussed. Name actions so the "load/action coexistence" contract checks their types too; the
JavaScript `@param {import(…)}` JSDoc exemption is already stated correctly.

## Verified and not contested

Every Kit citation in the plan is accurate for the installed 3.0.0 pin. The "do not re-export
a second `.remote` module under a different ID" rule is correct and important:
`remote_module_pattern = /[/.]remote\.[^/]+$/` would match `x.remote.ts?skgo-original`, so
the load proxy's `?skgo-original` trick would register a second remote hash. The `ssr`
environment name, adapter-pre plugin precedence, physical-file hook discovery, and
root-relative identity were each confirmed in source and in the kept build output. The Go
service lifecycle (`buildApp` pre, `buildEnd`, `closeBundle`) is unchanged by the plan.

## Outcome

material findings remain
