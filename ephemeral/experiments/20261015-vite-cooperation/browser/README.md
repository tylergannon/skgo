# Native Go publication through stock Vite to Kit's browser client

This is a bounded integration experiment, not a proposed complete dev server.
It deliberately uses a Kit route with `ssr=false` and a Vite-produced document
scaffold. It cannot establish Go-rendered page integration or select which
server should own the public development socket. All remote function execution
is Go; generated TypeScript remote bodies throw.

Run from the repository root:

```sh
go test -tags skgo_browser_experiment ./internal/hotreload -run '^TestNativeViteBrowserExperiment$' -count=1 -timeout=25m -v
```

The Go runner prepares a disposable generated application using the installed
patched Kit 3.0.0 and VitePlus pins. It reuses the existing native test's
`orderSource`, generator, `Lower`, `PrepareLoader`, `Compile`, and generated
`SkgoHotInstall`. It builds the native HTTP host once, starts `vp dev`, and
owns their lifetimes. Vite's child process group is killed during cleanup;
its optimizer cache is local to the disposable project. The experiment ends
after twenty minutes or `POST http://127.0.0.1:56142/__experiment/stop`.

Open http://127.0.0.1:56142/orders. Initial Go total is literally 3000. Enter a
draft and increment exactly three times. Click Set shallow state to store literal
`SHALLOW_NATIVE_41` through Kit’s public shallow `goto`. Record document and component IDs.
Then compare:

- `Patch 2400 + refreshAll`: compile one replacement object, install in the
  existing host, notify Vite, and use Kit's public `refreshAll()`.
- `Patch 1800 + invalidateAll`: same native path, different public Kit action.
- `Patch 1200 + document reload`: same native path, document reload comparison.
- `Compile error`: reject an undefined Go identifier before publication.
- `Repair 900 + refreshAll`: install the corrected literal and refresh.
- `POST /__experiment/frontend`: edit the actual Svelte file from
  `FRONTEND_ONE` to `FRONTEND_TWO`; Vite's normal watcher/HMR handles it.

The small actual Vite plugin exposes a loopback publication endpoint and calls
Vite's `server.hot.send`. The Svelte fixture uses `import.meta.hot.on`. Go does
not implement Vite's watcher, module graph, dependency transforms, HMR transport,
or frontend update semantics.

The log records actual native host PID and measured preparation/install times.
The page reports time from publication to completed Kit action plus Svelte tick.
That browser timing excludes candidate preparation and is not a benchmark distribution.
A stable PID is evidence of same-process installation, not a requirement to
preserve arbitrary application state. No ordinary-edit path calls `go build`.

Browser observations belong below after actual operation. An HTTP 200 or the
interactive runner exiting successfully is not browser proof.

## Observed in the actual in-app browser, 2026-10-15

The parent agent operated and inspected the actual Kit page. Native host PID was
94410. Initial total was 3000. The browser held draft `DRAFT_KEEP_41`, count 3,
shallow state `{"proof":"SHALLOW_NATIVE_41"}`, component ID
`e4060c9a-1e6b-4fda-9300-cac2a7f44547`, and document ID
`821dc780-466c-4b91-a778-417de13f12c8`.

- Native patch to 2400 plus `refreshAll()` visibly showed 2400, with every
  recorded state value/identity retained. Candidate preparation was 776.10 ms,
  installation 1.66 ms, and the browser displayed 3 ms publication-to-visible.
- Native patch to 1800 plus `invalidateAll()` visibly showed 1800, again with
  every recorded state value/identity retained. Candidate preparation was 743.66 ms,
  installation 1.61 ms, and the browser displayed 3 ms publication-to-visible.

Both public actions worked in this same-route query fixture. Their source-level
invalidation differences do not establish a visible state-loss difference here.
Kit warns that the fixture's public `replaceState` is deprecated in favor of
`goto(url, {state, shallow: true, replace: true})`; no private client API is used.

- The invalid Go identifier `broken` was rejected after 768.89 ms. There was no
  installation/publication. The page displayed the compile error and retained
  the last good total and healthy state. Repair to literal 900 was prepared in
  798.68 ms and installed in 1.42 ms; the actual page showed 900 after 3 ms,
  with its earlier draft/count/shallow state/component/document retained.
- Editing the Svelte markup literal to `FRONTEND_TWO` produced an ordinary Vite
  HMR update and visibly changed the page. The document ID and shallow state
  survived, but the component became `2767109e-9c51-45b2-8e5c-043a093ebbdf`;
  draft became empty and count became 0. Stock component HMR therefore did
  **not** preserve those component-local values in this fixture.
- After seeding state again, native patch to 1200 plus document reload showed
  1200, empty draft, count 0 and shallow `{}`. New component ID was
  `049d088a-6ebe-4d9c-bfab-077cc6cb02b9`; new document ID was
  `129e755a-c756-4662-837b-10e7d24683eb`. Candidate preparation was 797.31 ms and native
  installation 1.56 ms, still in host PID 94410.

The Svelte result matches the pinned runtime rather than a custom Go HMR policy:
`example/web/node_modules/svelte/src/internal/client/dev/hmr.js:36–51` destroys
the old effect then invokes the replacement component. Its getter/setter
preservation concerns the exposed instance, not restoration of arbitrary local
`$state`. Plugin 7.3.1 enables compiler HMR by default in development
(`src/utils/options.js:210`) and filters unchanged transformed modules
(`src/plugins/hot-update.js`), but changed component JavaScript takes that
replacement path. No extra state-preservation system was added. CSS-only HMR,
other component shapes, server-load errors, query-boundary repair, navigation
races and lost/reconnected publication events were not exercised here.

This experiment establishes one native-install → Vite custom-event → Kit-client
composition, and compares three client actions. It does not prove a file watcher,
SSR module delivery, a chosen public-socket architecture, or a full dev-server
solution. Go edits here are captured source overlays compiled by the real native
pipeline, triggered by explicit experiment controls. The renderer experiment is
separate; these pieces have not yet been joined.

Cleanup completed: the runner exited successfully after the stop request, both
ports 56141/56142 had no listening process, and native host PID 94410 no longer
existed. The saved shallow-state control now uses Kit’s documented `goto`
replacement for deprecated `replaceState`; that small API correction was
source-checked but the browser experiment above was not rerun. The observations
above describe the actually exercised `replaceState` version.

## Timing correction after comparison with prior native experiments

The original `compile` label in run.log measured the entire Toolchain.Compile
call: fresh packages.Load, type checking, classification, lowering, file writes,
then go tool compile. It was not compiler-only time. The runnable harness now
labels this `prepare`; the original log is preserved as recorded.

Earlier native logs already captured compiler times around17–26ms and early
edit-to-HTTP times around19–27ms. Those sources were available in the token
cache before this experiment, but the parent failed to reconcile them before
reporting0.75–0.80s as compilation. The current primitive performs fresh
packages.Load on every edit and does not use the retained analysis envisioned
by PLAN stage2. This browser experiment proved the communication path, not
performance qualification of the intended fast update path.

Historical references: `/Users/tyler/src/skgo-token-cache/experiments/history-native/initial-http-run.log`,
`poc-mappings.log`, and `poc-final.log` (the latter separates its additional
input-fence overhead from the17–18ms compiler invocation). The new tagged
`TestNativeCompileStageTimingExperiment` and `timing.log` isolate the current
stages on the actual generated-host fixture; no production optimization was
introduced for that measurement.
