# Retained preparation under whole-application correctness

**The fast retained behavior survives the real application correctness path.** On the same generated application, warm whole-application preparation took **168.108ms and150.521ms**, plus **3.786ms and3.792ms actual native installation**, versus **831.933–880.496ms** for existing CompileApplication. Actual handler literals1000/2000/3000 came from one native PID57278. The older root-only137–138ms exact number did not reproduce on this richer, correctly admitted app; no check was removed to hit it.

This is an experimental acquisition seam, not a production implementation or a universal latency bound. Source base was aaa53d3; platform Go1.27.2 darwin/arm64 with CGO_ENABLED=0. The production CompileApplication classifier, lowering, native compiler, object hashing and dependency archive hashing were retained. The actual generated native host was built using production **HostView**, pinning its toolchain and dependency identity, rather than legacy Lower's permissive host profile.

## Same-app comparison and costs

The fixture is the existing generated-handler contract: real remotes, loads, endpoint, action, deferred values, named refresh, cookies, authored panic mapping, internal fetch and existing pooled goja SSR. Production generation and pinned Kit build are unchanged. Source checking covered8 application roots,265 graph packages and4 affected reverse-import caller packages. All application roots remain typed; every affected caller becomes an explicit source root, including local replacements outside ./....

| Acquisition | Preparation samples ms | Actual install ms | Interpretation |
|---|---|---|---|
| Existing full CompileApplication |880.496,831.933,845.895|4.178,3.929,3.959|Full recursive dependency syntax every edit|
| Exact request routed through View |870.356,879.489,886.717|3.864,4.031,3.900|3 fallback loads; no retained benefit|
| Whole-app retained acquisition |548.236 cold,168.108,150.521|3.900,3.786,3.792|2 initial metadata loads, then4 metadata hits; no fallback|

[Raw samples/stages and full contract PASS](run.log). Three native supplied values per mode were installed and required literal real-handler responses. The same app/host/compiler/importcfg were used throughout this ordered comparison; these are bounded observations, not a statistical platform benchmark.

Warm stages:

| Stage | Two observations ms |
|---|---|
| Untyped source-shape fence |0.577,0.634|
| Captured inputs + retained whole-app analysis |42.833,26.922|
| Typed caller fence |0.820,0.822|
| Lowering |1.932,1.922|
| Candidate writes |0.549,0.538|
| Stock native compiler |33.868,32.728|
| Object/profile plus full dependency archive identity |87.504,86.929|

Archive identity remains the largest warm stage; it was deliberately retained. The analysis stage includes recursive mutable-input acquisition, structural selection fingerprints and8 typed roots, so it must not be described as the earlier root-only1–2ms analysis. The difference from the historical137ms experiment cannot be attributed solely to caller checking because its fixture/capture scope also differed.

## What was safely reused and invalidated

`loader_test.go.txt` retains discovery metadata and a bounded goload Session/View. It captures the main application and local replacement inputs, including non-Go/embed inputs. Whole graph metadata determines the complete reverse-import caller closure. All application roots and that closure are source-checked together, while unchanged dependencies retain compiler-export type graphs. Production Classify then runs unchanged over the returned graph. Ordinary body changes reuse discovery and compatible type information, but changed roots and affected callers are rechecked.

- Current full NeedDeps+NeedSyntax/TypesInfo requests are unsupported by goload retention: simply wrapping production's exact request fell back3/3. The experiment changes acquisition shape while preserving the classifier's coverage.
- File/package/import/build/control membership changes force discovery refresh. Warm existing caller-body edits are type-checked and uninstrumented calls select CompleteBuild before candidate.o.
- Target dependency source, source membership, and embedded input bytes/membership changes select **CompleteBuild**. This refuses compiling a replacement against a host whose dependency archives may no longer match. It does not implement structural restart/rebuild itself.
- Existing HostView admission still checks actual toolchain/profile/object/dependency identity. No dependency identity memoization or omitted reread was used.

[Independent validation](../20261015-retained-validation/REPORT.md) ran the full actual HostView/handler contract plus structural import/constant/type refusals, a warmed caller gaining a forbidden target call, dependency constant/type/import/newGo/embed changes, and a sibling local replacement module outside ./.... A deliberate target-only caller mutation admitted the forbidden external caller; a deliberate omitted dependency guard failed the intended rejection assertion. Positive restored cases passed. Those controls establish load-bearing checks, not just source review.

A further [actual archive experiment](archive-run.log) copied a real context dependency archive privately, added supplied bytes (shared build cache untouched), compiled successfully using that importcfg, and let real writeArtifact compute the changed archive identity. Actual HostView installation rejected the candidate with profile mismatch; its real handler independently remained literal3000. This joins actual archive bytes to host admission rather than only forging a dependency string.

## Actual browser join

The same qualified HostView contract app was joined to a real Vite custom-event connection and Kit client's refreshAll. Native compilation remained CompileApplication through the qualified whole-app acquisition seam. The browser used this app's production Kit assets, not the older legacy-Lower fixture; this isolates the Go speed answer and does not claim a complete development-frontend HMR composition.

Actual source saves produced4101→5202→6303, then malformed source retained6303 and repair produced7407. Warm successful preparation was176.658ms,165.195ms and161.491ms, with3.861ms,3.714ms and3.761ms install in unchanged PID60931. The initial cold browser preparation was793.424ms; concurrent process/initialization costs mean it is not interchangeable with isolated548ms cold observation. Times end at native preparation/installation, not visible paint. Source polling adds up to75ms, and browser refresh/network/paint were not independently timed.

[Browser raw native stages](browser-run.log), [independent browser observations](../20261015-retained-browser/OBSERVATIONS.md). Luna inspected the actual rendered application and screenshots: hydration true, supplied literals visible, draft/counter/document marker preserved through updates and error/repair. The generic browser error label is not a complete useful authored compiler diagnostic. The browser fixture stopped naturally and returned PASS; all owned processes/port56511 were cleaned.

## Scheduler and other boundaries

The acquisition benchmark works on supplied captured sources and does not own publication scheduling. The browser watcher compiles synchronously, rereads its watched source before installation, and discards a candidate if superseded during preparation. **It does not use ProjectBuild.Commit; a read-to-install race remains.** Independent existing Project stale-generation tests passed separately; that unused gate must not be promoted into joined obsolete-native-publication proof. Multi-file atomic snapshots and concurrent edit/admission remain separate integration work.

Mutable external source trees must be captured coherently; versioned module/toolchain inputs remain immutable for the retained View lifetime. The fixture is representative of the existing handler contract and one replaceable package, not every possible application graph, platform, session length or Go edit. Incompatible changes are correctly refused/rebuild-selected, but no new structural rebuild supervisor was built. Arbitrary initialized Go state continuity is not required or claimed.

## Reproduction and preserved failures

[Commands](COMMANDS.txt), overlays, exact loader/compiler/contract/browser sources, raw outputs and browser authored/actual HostView source snapshots are preserved here and in token-cache `experiments/retained-qualification-20261015/`. No production files were modified.

- `failure-first-metadata-refresh-and-restore.log`: metadata NeedExportFile refreshed discovery each edit; sample fixture also failed original expected4200 because its constant body had not been restored. Original literal contract caught that harness error.
- `mode-pingpong.log`: full original contract PASS, but inconsistent NeedEmbedFiles metadata modes caused2 metadata reloads/edit and534–557ms preparation. Matching metadata supersets corrected reuse; this run is not the final speed result.
- `run.log`: final equivalent comparison and original full generated-handler contract PASS.
- `archive-run.log`: actual changed dependency archive compile + real HostView refusal PASS.
- `browser-run.log`: actual source-save/native/Kit-client error/repair join, naturally stopped PASS.
