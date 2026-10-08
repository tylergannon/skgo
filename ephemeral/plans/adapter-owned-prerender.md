# Adapter-owned prerender integration

## Developer outcome

Generated application server modules contain typed throwing stubs, not build
bridges. A developer reading a generated load or remote function sees its Kit
declaration, Go-derived types, and a body that throws `skgo: implemented in Go`.
Build forwarding, input producers, platform callbacks, and request lifecycle
integration belong to the adapter's build integration. This request is planning
and independent review; it does not authorize implementing or releasing the
production change.

The scope includes server loads, prerender remote functions with and without
declared inputs, server endpoints, and the generated server hook. Extracting bridge helpers into
imports in application modules does not satisfy this outcome. Generated Go
bindings, Go wire codecs, and the build-only Go service remain necessary.

## Kit's existing seam

The authority is this worktree's frozen installed `@sveltejs/kit@3.0.0` at
`example/web/node_modules/@sveltejs/kit`, following
`ephemeral/sveltekit-current/SKILL.md`.

Kit includes `adapter.vite.plugins.pre` before its own compiler/remote plugins
in `src/exports/vite/index.js:608-675`. Its build sequence in
`src/exports/vite/build/index.js` builds `builder.environments.ssr` at line 449,
prerenders at line 794, and calls `adapt` at line 909. The final `adapt` callback
is too late to supply prerender load behavior; the adapter's Vite hooks are not.

Kit's `src/runtime/server/page/load_data.js:22-192` constructs the load event,
tracks its accesses, calls `node.server.load`, and validates its result. Keep
that implementation. Kit's `src/exports/vite/plugins/remote.js:97-154` registers
remote functions and computes their IDs from their original root-relative
module path. Keep that implementation and module identity too. Its native
`src/runtime/app/server/remote/prerender.js` owns validation, caching, remote
dependencies, and result serialization; forwarding replaces only the Go body.
After crawling, Kit's `src/exports/vite/build/remote.js:21-137` replaces
non-dynamic prerender bodies with throwing declarations and tree-shakes their
callbacks. Preserve this step and its original-module identity lookup. Remote
bridge presence must be established by its actual build results or before that
step, not by requiring a callback in the final server JavaScript.
Kit's `src/runtime/server/endpoint.js:14-82` selects the endpoint handler by
HTTP method, handles HEAD/fallback, and invokes it. Keep those native semantics;
the adapter supplies each declared method's build-only Go-forwarding body.

## Required build boundary and demonstrated approach

The demonstrated adapter plugin has `apply: 'build'` restricted to
`environment.name === 'ssr'`. Here `ssr` is Kit's Node build environment, distinct
from SKGo's embedded `goja` environment and from development server execution.
The experiment supplies module content through an in-memory load hook before
Kit's own transformations. This proves a feasible route, not a mandated choice
between Vite load and transform hooks. The builder should choose the smallest
native build integration after its own mapping. The required boundary is that
bridge behavior belongs only to Kit's build SSR execution and application source
contains throwing declarations. Client, development, and goja execution retain
their current native runtime integrations. No Kit patch or request-time Go
service execution is needed for this change.

The builder chooses the smallest representation and carrier for build declaration
information; this plan does not require extending `skgo.remotes.json`. The
information must come from the actual Go declarations, not copies of TypeScript
source or executable JavaScript strings in metadata. Preserve current
remote/load/action/endpoint identity checks and Kit's own APIs.
Build integration needs the actual generated exports: load source identity for
diagnostics; action names in a module with loads/actions; and the kind, exported
name, argument presence, and declared-input presence of every remote in an
affected module, including modules mixing prerender and other remote kinds.
Preserve each endpoint's original module path and exact declared method exports,
including QUERY and fallback, using the generator's existing endpoint declarations.
Map the existing scanner and emitters independently when choosing how that
declaration information reaches the build.

For a server load, the build function uses the existing adapter load bridge
and translates returned redirects/errors with Kit's own functions. Preserve
transported values, deferred results, parent data, headers, cookies, typed
parameters, Go request hooks, and the existing failure diagnostics.

For remote modules, preserve every native wrapper and its no-argument versus
argument-taking call shape. The prerender body forwards through the existing Go
remote bridge; declared inputs are a lazy callback through the existing Go input
bridge. Other bodies remain throwing stubs. Do not proxy/re-export a second
`.remote` module under a different ID: Kit must initialize exactly the original
exports with exactly the original IDs and kinds.

For `+server` modules, the build method bodies use the existing endpoint
bridge. Application-source exports are plain typed throwing handlers. Preserve
Kit's method analysis and dispatch, Go hook/request isolation, endpoint fetches
during prerendering, static endpoint artifacts, and normal runtime Go dispatch.

Kit's `src/core/sync/write_server.js:68-110` discovers server hooks from physical
source entries. The experiment used an empty `hooks.server.ts` marker and
supplied its hook exports only during the build. That form is sufficient, not
mandatory: the builder can choose the minimal marker/type declarations Kit
needs. Keep the existing prohibition on authored server hook paths. Application
source has no prerender imports or build branch.

The existing Go service owner still starts/publishes in the awaited buildApp pre
hook, and still owns timeout, terminal failure, cancellation, and cleanup. No
service startup in configResolved: generation/formatting also load Vite config.
Module replacement happens during compilation without invoking the service.
Missing or malformed declaration metadata is an explicit build error, rather
than a fallthrough to a stub or a guessed source path. Match canonical IDs
relative to Vite's resolved root; preserve source identities for diagnostics.

## Source result

TypeScript modules retain ordinary top-level type imports and their exact
Go-derived return shapes. Their bodies only throw; no `building`, `buildLoad`,
`prerenderFromGo`, endpoint `fromGo`, platform assertions, input-producer closures,
bridge imports, or inline TypeScript import expressions remain in generated
application source. Remove imports used only by removed bridge code, including
`getRequestEvent`; a dangling unused bridge import does not satisfy the cleanup.
Action declarations retain their exact success/failure
union types, using named `ActionFailure` type imports in TypeScript rather than
inline import expressions. Load/action coexistence must check those types too.
JavaScript modules retain the JSDoc Kit requires for types; JSDoc import type
references are not TypeScript inline import expressions. Kit declaration code
such as `query(...)` and `prerender(...)` remains because it describes the native
API and browser types, with throwing bodies and no build callback options.

Generation must replace existing SKGo-owned generated bridges with these stubs,
handle both language modes and language switches, and preserve authored files.
No compatibility shim, new acceptance runner, sidecar, or new application I/O
implementation is part of this change.

## Existing boundary experiment

`ephemeral/research/adapter_load_research_test.go` is the saved Go test. From the
repository root, Go's standard overlay maps it into the generator test package
without changing the implementation tree:

```text
go test -overlay ephemeral/research/adapter_load_research_overlay.json -tags adapter_prerender_research -count=1 -v ./internal/gen -run '^TestResearchAdapterSuppliesBuildLoads$'
```

Its build tag excludes the saved artifact from ordinary repository tests. The earlier, narrower experiment
is preserved in `ephemeral/research/adapter-owned-prerender.patch`; the saved Go
test is the current experiment and additionally removes endpoint bridges.

The test stages an isolated application, generates it, then substitutes seven
source loads with typed throwing stubs, an empty hook marker, three remote
source modules without build bridges, and four modules of throwing endpoint
exports. A build-only adapter plugin supplies
bridge-bearing modules in memory. It runs a real Kit build, requires the whole
`src/` tree's paths and bytes to be unchanged afterward, and runs six existing literal
Go handler contracts for pages/endpoints, redirects, renderer resolve options,
remote artifact misses, stored errors, and transform failures. This experiment
passed against the current pinned dependencies with seven load stubs and eight
hook/remote/endpoint build variants. It requires all six selected contracts to
execute and pass without skips, rather than treating exit code zero alone as
proof. It requires the retained load/endpoint/request bridge symbols in compiled
Kit server JavaScript and no bridge symbols in client or goja JavaScript from
that same build. The literal remote artifact/result contracts establish remote
forwarding; Kit removes non-dynamic remote callbacks after crawling.

This demonstrates the build boundary, not a finished production implementation.
In particular, the prototype keeps old hook/remote/endpoint module text in its temporary
Vite build config; the product must synthesize those modules from declaration
metadata. The prototype's regex edits only prepare an isolated fixture; they
are not the proposed production transformation. Its load proxy preserves
original exports, but the product must explicitly cover load/action coexistence.
It does not establish JavaScript/JSDoc behavior, development behavior, the whole
browser suite, or final package distribution.

## Acceptance for implementation

Use ordinary Go tests and existing real application builds. Generated output
contracts must require the literal types/native declarations and throwing
bodies as well as the absence of bridge code, so an empty file cannot pass.
Cover both languages, language switching, load/action coexistence, endpoint methods/fallback, mixed remote
kinds, no-argument and argument-taking prerenders, and typed/transported inputs.
Require generated metadata to agree with compiled native exports and IDs.

Exercise the production adapter on freshly generated plain stubs without the
research test's source edits or copied build-module text. Reuse one real build's
artifacts across the existing literal handler contracts for prerender behavior,
request lifecycle, error/redirect diagnostics, parent data, transports, deferred
results and inputs. Add the smallest missing contract to an existing fixture
where necessary. Missing dependencies or skipped tests are failures.

Prove the plugin does not supply bridges to client, dev, or goja execution.
Require positive bridge presence where Kit retains it in build SSR output and
absence in client/goja output of that same build, together with literal runtime
rendering and remote artifact/results. Account for Kit's post-crawl remote
tree-shaking described above. Retain the existing explicit authored-fixture attempt to
import/call a build helper in the engine and require its expected failure
message; merely having no generated helper import is not proof of denial.
Confirm application source bytes stay
unchanged during builds; run generation again to check stable output. The
published adapter must contain every new internal module and match the embedded
adapter fingerprint under its existing package contracts.

An independent validating agent should check that the chosen tests actually
fail if the required behavior is removed, then run them. Final integration uses
the project's ordinary `just test`, the browser scenarios in both modes whose
claims require Kit's client, and opening the changed example as a visitor. Do
not add HTTP-only browser scenarios or a new command that declares completion.

## Implementer handoff

Deliver the developer outcome above across the generator and adapter as one
coherent change. Kit's installed pin is the specification. The builder owns
both sides of the declaration metadata seam and removal of existing generated
application bridges; its independent validator owns load-bearing verification.
All agents work in the designated SKGo worktree. Choose implementation details
from the pinned source and existing code, and preserve the established Go
service lifecycle. The boundary experiment is supporting evidence, not a
template the production code must copy.
