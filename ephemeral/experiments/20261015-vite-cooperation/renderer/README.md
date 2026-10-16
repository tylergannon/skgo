# Renderer delivery experiment

Run from the repository root:

```sh
go test ./ephemeral/experiments/20261015-vite-cooperation/renderer -run TestRendererDelivery -v -count=1
```

Requires the existing `example/web` frozen dependency installation and `vp`. The Go test creates an isolated fixture beneath this directory, starts Vite with `vp dev`, drives the adapter-boundary plugin over HTTP, verifies literal rendered output, and terminates the process group. It changes no production source. The index is untouched.

## What actually ran

The fixture imports an ordinary JavaScript dependency from a Svelte page inside a Svelte layout. Kit 3.0.0 and Svelte 5.57.1 compile their real root/Props/rendering closure through the existing `gojaEnvironment`; SKGO's existing `ssr.New` and two-slot pool execute the resulting artifact. This is **not a stand-in render function**. The small fixture omits loads, remotes, asset handling and browser hydration.

Observed representative run (2026-10-15, this machine; individual observations, not performance guarantees):

| Operation | Vite work plus local HTTP round trip | New renderer pool | First render |
| --- | ---: | ---: | ---: |
| Initial complete Kit build | 750 ms | 5.8 ms | 0.22 ms |
| Existing page component edit; retained builder, goja environment only | 44 ms | 4.1 ms | 0.28 ms |
| Imported dependency edit; same retained builder | 50 ms | 3.9 ms | 0.53 ms |
| Another dependency edit; complete Kit build again | 685 ms | 3.9 ms | 0.44 ms |

All four renders checked the expected changed heading text. The emitted renderer was 123,897 bytes. Across earlier diagnostic runs the environment-only rebuild observations ranged roughly 42–143 ms under variable concurrent machine load. No Go rebuild/link occurs in these renderer changes.

`gojaEnvironment.build` retains the same ViteBuilder/configuration and invokes Vite's environment build again, followed by its existing Rolldown fold. This does **not** demonstrate incremental reuse of Vite's development module graph; no claim about internal build cache retention is made. The experiment fixes a two-component node table. New/deleted routes, route ordering, universal hooks, remote identities and changed configuration were not tested and need refreshed metadata before this could serve a complete app. Full and partial builds are a useful executable comparison, not an architectural selection.

## Upstream ModuleRunner comparison

A separate probe bundled the pinned upstream `vite/module-runner`, without porting or editing it. A Vite-transformed real `message.js` module was returned through the documented invocation-shaped transport. The default IIFE fails goja parsing at the native `import(filepath)` in `ESModulesEvaluator.runExternalModule`. Standard pinned Rolldown CJS output with `dynamicImportInCjs:false` removes that syntax; the unchanged runner then imports the Vite-transformed module and returns `Dependency C` successfully. No CommonJS `require` implementation is installed: this probe has no externalized modules. This shows a plausible runtime capability, not complete renderer delivery.

The next bounded probe reused the existing adapter plugin in a Vite development `goja` environment, retaining its initialized node/hook sources. It used the original runner and an HTTP transport forwarding module requests to Vite's `fetchModule`; Go does not resolve modules or maintain a module graph. The actual adapter entry began importing the real Svelte server graph. It stopped before rendering:

```
TypeError: Invalid base URL
    at posixPathToFileHref
    at directRequest
```

The immediately preceding module request was `/@id/__x00__skgo:skgo:esm-env`, the adapter's NUL-prefixed virtual module. Upstream runner constructs a file URL from module identity (`module-runner.js:226`); the current engine supplies `goja_nodejs` URL. Diagnostic instrumentation captured the exact single constructor argument: `file:///\u0000skgo:skgo:esm-env` (an actual NUL, no separate base argument). `goja_nodejs` at the pinned `1f56ff5bcf14` calls Go `net/url.Parse` directly (`url/url.go:167`), which rejects the control character; its one-argument constructor calls that parser with `isBase=true` (`url/url.go:390`), explaining the misleading “base” wording. A pinned-toolchain Node comparison accepted the same input and returned `file:///%00skgo:skgo:esm-env`. Thus the demonstrated blocker is a specific existing URL implementation compatibility defect, not missing runner/module-graph machinery. A small existing-boundary correction may be sufficient for this edge; further graph edges remain untested. This result does not establish that upstream runner requires Node or that this path is impossible. At the parent's request, one final bounded retry normalized NUL to `%00` solely in the experiment's file-URL constructor. That passed this edge and traversed further through the actual Svelte graph. The next request was `@oxc-project/runtime/helpers/awaitAsyncGenerator`, inserted by existing adapter lowering; external-module evaluation then failed with `ReferenceError: require is not defined`. The runner's standard CJS-lowered external-import path reaches `require`, which the production Go engine intentionally does not provide. The attempt stopped after about 144 ms of module traversal. No external loader, module graph rewrite, or additional compatibility machinery was built. Whether the helper can remain inside Vite's transformed/inlined graph is untested, so this is an unresolved integration edge, not a declaration that the runner is impossible. **Actual Kit rendering and update latency through ModuleRunner remain unproven.**

## Scope and glue

`vite.config.js` is only an experiment adapter/configuration boundary. It uses existing adapter exports, ViteBuilder, DevEnvironment/fetchModule, and pinned Rolldown. `renderer_test.go` owns orchestration, fixture edits, pool creation and assertions. Temporary transport/bootstrap code for the runner lives in those fixtures; it is not a proposed product API. The test temporarily chooses production compilation while invoking build/runtime probes because mixing a dev-compiled component with the existing adapter's `DEV=false` renderer produces a Svelte render-context mismatch. It restores the dev process environment afterwards; a production integration should avoid relying on such shared mutable process configuration.

The timer excludes watcher debounce and browser delivery. No socket handoff, Go native patch, browser HMR, concurrent renderer replacement, failed-build retention, route addition, or application-sized performance claim is made here. Native Go live patching remains required and independent of these frontend renderer experiments.

The Go test returning success means the successful artifact paths and simple runner import passed their assertions. The full-graph probe deliberately records a blocking result without failing the enclosing exploratory experiment; it has **not** passed Kit rendering qualification.
