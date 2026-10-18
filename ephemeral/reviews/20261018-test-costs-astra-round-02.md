# Independent review: second test-cost reduction

Target: proposed round-two deletions/simplifications, including the incomplete dirty draft after cc1190d2. Implementation was paused; this is a plan review, not implementation acceptance.

Outcome: **material findings remain**. The proposed direction is sound. I would proceed with the cuts, with the small preservation condition below and a wider sweep of expensive assertions. I found no reason to retain the Swift end-to-end rebuild, repeated language transitions, compiler-rejection permutations, CLI analyzer matrices, or separate scaffold compilation merely because they once caught a bug.

Evidence: repository AGENTS.md; complete current deletion diff; `ephemeral/test-timing/final.jsonl` and `final-time.log`; generator fixture/setup, prerender, ownership, caller-event, retained-project and cache tests; hotreload host, handler, state/lifetime and compiler paths; plugin installation implementation/tests; CLI dispatch/MCP and deleted interface tests; example production-handler, route/data sweeps and streaming tests; native-client tests. No suites were rerun and no implementation was changed. Package/test timings overlap and are not additive savings forecasts. The memory registry search yielded no task-specific evidence used in this review.

## Findings

### 1. Issue — the separate native-state test has one distinct safety assertion that the retained fixtures do not currently cover

`internal/hotreload/native_test.go:409–414` checks that installation refuses a candidate requiring missing host storage, including refusal *before* opening a nonexistent object. Searching the retained handler/lifetime tests found no equivalent; the lifetime branch creates the `missing` candidate but only checks its reclaim classification (`native_test.go:272`). The generated installer actually owns this guard (`native_runtime.go:103`), so this is not a dependency assertion. Removing the guard can allow candidate relocation against absent host symbols rather than selecting host replacement.

Delete the separate state host, but carry this small check onto an already running host before its retained budget is exhausted. It needs no additional candidate compilation: a candidate with a deliberately absent storage symbol and nonexistent object is enough to exercise the ordering. Preserve the existing state/init/type/held-request assertions already present in the handler/lifetime fixtures; do not recreate the state test's whole scenario. This is the only concrete objection I found to the listed cuts.

### 2. Issue — duplicate streaming requests and whole-manifest sweeps leave an avoidable seconds-long floor

`example/stream_test.go:36` and `:163` both request `/stream` and assert pending shell/value order; the latter additionally observes actual streaming and middleware behavior. `:69` and `:234` separately request the identical data URL; the latter adds only header/absence assertions. All four cost 3.60s in the captured timing because the demo deliberately waits up to 3600ms (`example/web/src/routes/stream/page.server.go:22–24`). These are two request results worth of assertions, not four.

Merge the first document test's remaining assertions into the real streaming test; merge the data middleware assertions into the data-order test. Also replace the two full-manifest HTTP sweeps (`example/server_test.go:302`, `:819`, measured 5.19s and 3.61s) with the representative routing distinctions not already covered by focused real-handler tests. They currently re-exercise the slow stream and other application pages to establish generic document/data envelope behavior. Keep registration/manifest completeness checks that inspect metadata directly. Do not shorten the product demo's waits or add a test-only timing mode to save these tests. This is the clearest additional wall-time target outside the proposed compiler cuts.

### 3. Issue — the compiler sweep should include tests compiling against the hand-written fake SDK

`internal/gen/ownership_test.go:28–31` explicitly defines `foreignFixture` as a stand-in for SKGO; `:58` writes a fake SDK and `:337–361` replaces the real module with it. It is useful for scanner inputs, but app builds against this fake API cannot establish compatibility with the real SDK.

The proposed output-ownership cuts should extend to `prerender_test.go:81,110,193,234,270`, `arity_test.go:119,249`, and the dependency-ownership builds at `ownership_test.go:483,569`. In particular the JavaScript-stub test at `prerender_test.go:88` already asserts the emitted JavaScript type, then builds a Go application unrelated to that language distinction. Keep exact emitter/admission assertions, and keep the minimal generated-consumer compilation against the *real* SKGO API where interoperability actually matters. Do not replace each deleted compile with another miniature compile framework. As fake-SDK members become unused, delete them too; that imitation is a significant maintenance cost independent of test runtime.

### 4. Issue — CLI copies survive inside the generator package, and the draft must retain the cheap MCP contract

The plan removes CLI analyzer duplication from `cmd/skgo`, but `internal/gen/check_test.go:119–123,145–175` still builds a CLI and runs pristine/failing complete check commands; `internal/gen/swift_test.go:172–193` also requests that binary. `internal/gen/main_test.go:55–67` consequently links another SKGO executable in a separate test package. This is additional orchestration duplication, not generator-boundary proof. The reported `TestCheckApplication` takes 3.61s and Swift generator/domain coverage 3.99s, though only measurement after editing can attribute savings precisely.

Keep scanner/generator assertions in `internal/gen`; consolidate the actual CLI exit/output witness with the CLI package's existing executable witness. The deleted `cmd/skgo/interface_test.go` need not return, but `serveMCP(io.Reader, io.Writer)` already exists (`cmd/skgo/mcp.go:48`) and allows the promised dispatch, initialization and error/output proof without a child process or analyzer application. The current dirty draft has no such replacement; that is understood to be unfinished, not a completed-change defect. Do not finish the deletion with the entire SKGO-owned MCP protocol untested, and do not rebuild the large equivalence fixture to address it.

## Additional scope decisions

- Use the retained loader for native compiler fixtures. `compile.go:43–53` source-loads the complete dependency graph with `NeedDeps|NeedSyntax|NeedTypesInfo`; production `host.go:562` supplies `p.retained.Load`. The current expensive fixture path is materially different from the shipped path. Keep `compileWithLoader`, which production uses; after migration, remove the otherwise test-only `Compile`/`CompileApplication` wrappers if no callers remain. Avoid creating a second retained test implementation.
- Preserve genuine process-boundary witnesses: native installation and held-request/lifetime behavior, one real plugin ABI/CLI application, generated shutdown, and the existing minimal generated-consumer witness. Their presence does not justify repeated equivalent builds around them.
- The installer builder/probe boundary proposal is appropriate if it invokes real `Install` orchestration and real publication/selection/locking. Testing only `publish` would miss whether a failed build/probe reaches publication; replacing the whole workflow with a mock would miss the claim. The proposal already states the correct boundary.
- The Swift roundtrip deletion reduces generated-Swift integration confidence, but that does not justify retaining a 9.63s rebuild in the ordinary suite when direct emitter tests and native transport boundary tests remain. Do not claim those remaining tests prove the exact removed roundtrip; simply stop making that automated claim.

The next implementation should stay predominantly deletions, assertion consolidation and use of existing production boundaries. No new fixture system, gate, timing runner, build-tag lane or exhaustive replacement matrix is warranted. Re-measure the ordinary suite once after the cuts; the captured timing alone cannot promise a particular final wall time.
