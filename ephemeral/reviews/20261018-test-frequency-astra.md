# Astra review: SKGO test cost and frequency

**My recommendation is to make `just test` the inexpensive, normally cached command people use constantly, route Linux CI to Bob, and run a small set of real compiler/process proofs when their implementation changes and before releases. Delete redundant setup and negligible assertions before deciding what belongs in slower qualification.** A slow tier is useful; it must not become a storage area for every expensive test ever written.

The user has already decided to remove redundant post-merge CI reruns and ordinary `-count=1`, and has selected Bob for Linux tests and any other practical CPU workload that can be offloaded. Those decisions take precedence over the older instruction against separate lanes. This is advice only: no implementation, infrastructure changes, or test-suite executions were performed.

Review target: worktree `/Users/tyler/.codex/worktrees/f4fc/skgo`, commit `a7de5528`, following merged PR #386. Outcome: **material findings remain** in the current frequency/cost arrangement and in an incomplete implementation of the proposed move to qualification. The direction is sound; the qualifications below make it concrete.

## What is measured, and what is inferred

The supplied `ephemeral/test-timing/round3/final-time.log` records a warm, deliberately uncached test execution: **19.25 seconds elapsed and 71.54 CPU-seconds** (44.78 user + 26.76 system). This is not the time of an ordinary cached repeat. The supplied package accounting attributes 18.93 CPU-seconds to hotreload, 15.83 to generation, 4.32 to adapter, 4.30 to kitpatch, 3.61 to CLI tests, 3.48 to devserver, 11.08 to other tests, and 9.99 to the Go driver/build work. These CPU costs overlap in elapsed time; their removal cannot be translated directly into wall-time savings.

The existing JSONL independently shows expensive individual executions: native handler contract 6.58s, captured native HTTP host 4.77s, native lifetime 2.97s, typed-load generated consumer 2.44s, language-application generation 2.22s, and kitpatch's native merge/idempotence test 2.74s. Two example streaming tests each take 3.60s because they use deliberately delayed application loads. These figures describe this run, not a promised future budget.

The parent agent inspected live infrastructure during this review. Its reported evidence is:

- Routine CI run `39173446014`, job `117811135514`, used GitHub-hosted compute. Its test step took approximately 143s, including 115.658s reported for the native package. Its logs show Go and mise cache hits, not universally cold caches.
- Three self-hosted runners are online: `bob-skgo-release-1`, `bob-skgo-release-2` (Linux X64), and `amritanandamayi-skgo` (macOS ARM64), all carrying `skgo-release`.
- Bob already has persistent runner workspaces and shared Go/module/tool caches. Live inspection found approximately 15GB of Go build cache, 3GB of modules, and 69MB of Zig cache. Both runner containers were running.
- The Actions cache inventory contains Go and mise entries, with no Zig-named cache entries. The observed setup-go cache hit was not saved again after the job.

I independently verified the repository configuration supporting those reports: `ci.yml:21,69,90` selects hosted runners; `qualification.yml:16` selects self-hosted Linux; `bob-runners.compose.yml:18-36` mounts persistent caches and workspaces; `bob-runner.Dockerfile:12` sets the persistent `GOCACHE`.

**The supported explanation is that routine CI bypasses Bob's already-warm infrastructure.** On the hosted jobs, dependency-keyed immutable Go cache hits also cannot incorporate newly built project objects on that same key, and no explicit Zig cache transport is configured. Those are credible contributors to cost. The evidence does not isolate how much of the 115.658s was cold Zig compilation, libc/archive work, host speed, or other effects. `cache: false` on self-hosted setup-go disables Actions cache transport; it does not disable the local Go cache.

## Findings

### 1. Issue — the same expensive proof is scheduled more often than its value warrants

`ci.yml:6-8,20` runs same-repository push CI, including pushes to main and tags. Its ordinary `just test` includes the native loader tests, while `ci.yml:84-107` runs two of them again in a Linux/macOS matrix. The tests themselves already enforce no-cgo conditions (`internal/hotreload/native_test.go:40-44`, `contract_test.go:28-32`), so the additional Linux job is not buying a new no-cgo configuration. The native Zig exchange also runs in the ordinary Linux job and the macOS `./native ./internal/swiftgen` job (`ci.yml:82`). Release qualification again runs `just vet` and `just test` after rebuilding the candidate (`qualification.yml:47-56`).

**Change:** stop ordinary CI on main and release tags. Keep one ordinary check per development revision, retaining the existing avoidance of duplicate same-repository push/PR jobs. Run native loader qualification once per required platform when loader/compiler/generator/runtime inputs change, and at release; run the Swift consumer on macOS when Swift generation changes and at release. A second Linux native invocation on the same source/toolchain contributes no additional evidence. Keep fresh execution of selected qualification witnesses, not a forced rerun of every ordinary assertion.

This removes redundant post-merge CI while preserving release qualification of the actual candidate. The latter has a distinct purpose and is permitted by the user's stated preference for occasional slow checks.

### 2. Issue — “move it to existing qualification” is not yet an executable coverage plan

The existing qualification workflow is **main-only and Linux-only** (`qualification.yml:15-16`). It has no pre-merge relevant-change trigger and no Swift or macOS native job. The real Vite development integration tests are already opt-in through `//go:build skgo_dev_integration` (`internal/devserver/session_integration_test.go:1`), and that tag is not invoked by these workflows. Ordinary Go compilation does not prove those tests ran.

There is a second prerequisite: `example/server_test.go:33-48` constructs the real handler using the embedded frontend build. Deleting `just build` from CI while keeping those tests does not yield cheap full checks; it leaves missing or stale inputs.

**Change:** explicitly wire retained slow proofs into qualification before removing their existing execution. Allow relevant-change qualification against the PR revision, plus the release candidate. Include the required native platforms and the selected development-session tests where the change crosses the Vite/Go boundary. Use ordinary Go test invocations and a small, explicit selection; no new gate framework or evidence-manifest system is needed.

For the built example, keep one candidate build and share it between handler tests and production browser tests. If the routine check must work without a frontend build, move only the tests that consume that real build to qualification, keeping standalone handler contracts in the ordinary suite. Do not label a command “full” while silently omitting its prerequisites or tests. Do not claim complete development browser qualification from the focused development tests.

### 3. Issue — several remaining tests still pay for a whole application to inspect emitted text

Examples include no-argument remotes (`internal/gen/arity_test.go:23-115`), Swift output (`swift_test.go:16-41`), prerender stubs (`prerender_test.go:29-103`), and the shared endpoint fixture (`endpoints_test.go:38-85`, also consumed by `actions_test.go:10-16`). They run full generation and then inspect strings or file existence. The endpoint file already demonstrates the cheaper pattern: call `writeEndpointStubs` with explicit declarations and compare the emitted source (`endpoints_test.go:94-159`).

Kitpatch also repeatedly invokes real package-manager config merging for YAML shapes (`internal/kitpatch/kitpatch_test.go:225-283`) and repeatedly reconstructs stock Kit queue bytes via reverse patching (`703-759`). SKGO must preserve authored patch paths and refuse ambiguous input; it need not repeatedly qualify pnpm's general YAML processing. A test with many syntax variants is not automatically valuable merely because all its assertions pass.

**Change:** move emitter assertions to the emitter, retaining independent literal expectations for SKGO-owned choices. Delete subset checks already covered by stronger emitter contracts. For kitpatch, retain one real VitePlus/pnpm configure-and-verify witness covering preservation and idempotence, check SKGO-owned preflight/refusal decisions in process using the existing `Options.Run` seam, and remove redundant native runs for ordinary YAML syntax. Recover immutable stock queue input once per package execution and copy it into mutable fixtures.

Keep a representative full-generation consumer where compilation and actual dispatch add evidence that a source assertion cannot. The typed-load dependency consumer (`load_params_test.go:11-14,66-83`) is such a case: generated typed accessors must affect the real response. The language-switch test also exercises generation ownership across two revisions (`language_application_test.go:30-64`); preserve that transition while trimming duplicated stub-text assertions. There is no reason to retain every separate whole-generation fixture.

### 4. Issue — the Zig protocol test builds an ABI archive it never uses

`native/remote_test.go:23` invokes `zig build install`. The default install builds both `skgo-remote` and `skgo_native_core` (`native/core/build.zig:18-24`), including libc-linked ABI code and bundled compiler runtime. The Go test only executes `zig-out/bin/skgo-remote` (`remote_test.go:28,91`). Building the archive is neither an assertion about its ABI nor a prerequisite of the tested executable.

**Change:** expose a CLI-only Zig build step and use it here. Compile that CLI once and keep the existing many literal HTTP assertions against Go's real remote handler. Those assertions prove an SKGO-owned interoperability boundary and belong in relevant native qualification. They do not justify compiling the unused archive. Build the archive where an actual archive consumer requires it; do not invent another test merely to preserve today's accidental work.

### 5. Issue — caching policy currently spends around the warm infrastructure and cannot substitute for proof selection

`Justfile:71` explicitly disables test-result caching for every invocation. Removing that is already authorized. Bob's warm build/module caches already exist; configuring a new cache service or new runner fleet would solve the wrong problem.

**Change:** select Bob's current Linux runner labels for trusted repository work and use its persistent local Go cache. Keep Actions Go cache transport disabled there. Retain the shared Zig global cache and, if needed, place the checkout-local Zig cache under the existing persistent cache mount so checkout cleanup cannot erase it. Keep build outputs and mutable fixtures job-local: sharing a compiler cache is different from having simultaneous jobs overwrite one binary or application directory. Two Bob slots do not require a scheduler redesign.

Use Bob beyond unit tests: frontend dependency preparation, example generation/builds, Linux browser qualification, Go vet and documentation-source checks, and Linux release build work all fit the existing capacity. Release qualification and Linux amd64 packaging already use it. The two current containers have 8 CPU/16GB limits each; keep the working setup and measure contention before adjusting it. The local-test `GOMAXPROCS=2` / `-p=4` choices should not become unexplained permanent CI limits on an under-used host. Raise concurrency only when measured elapsed time improves without making the two jobs fight for memory.

Keep actual Darwin/Swift qualification and signing on macOS, and native Linux ARM64 archive/plugin execution on ARM64. The release builder enables cgo and executes its built CLI (`cmd/skgo-release/main.go:102,119-137`); an amd64 cross-build does not replace that platform proof. npm publication currently explicitly requires the GitHub-hosted trusted-publisher identity (`release.yml:217-223`), so keep those final publish steps there. The `@skgo/sv` build can be prepared on Bob and handed to publication if its exact candidate package is preserved; do not build it again in the publishing job. The Pages workflow only uploads and deploys static `docs` (`pages.yml:25-34`), so moving it has negligible CPU value compared with moving builds and browsers.

The current CI condition also accepts fork pull requests. Do not mechanically change its runner label and thereby send arbitrary fork code to the persistent LAN runner; retain a hosted fork path or restrict Bob jobs to trusted repository pushes. This is a consequence of the existing trigger, not a reason to postpone routing normal Linux work to Bob.

Finally, cached Go test success is not reliable freshness proof for files read only by child Node/Zig/compiler processes. Keep explicit uncached execution for the selected cross-language/process qualification witnesses. Normal Go caching remains appropriate for ordinary Go contracts; no general-purpose content-hashing framework is warranted.

## Recommended operating balance

| Work | Frequency and proof |
|---|---|
| Handler contracts, declaration admission, emitter output, command parsing | Normal cached Go tests during development and one ordinary CI check per revision. Run a focused package while editing; use `just test` freely. |
| Generated native replacement, captured host, generated typed-load consumer | Qualification when generator/loader/compiler/runtime inputs change, and before release. One generated fixture per distinct capability; reuse immutable compiler setup. |
| Zig/Go remote exchange and Swift generated-model consumer | Relevant native changes and releases, on the required actual platforms. One CLI build and one Swift consumer build per input set. |
| Adapter callbacks and process ownership | Keep cheap Node seams in ordinary tests when they directly exercise shipped adapter code. Real Vite/session qualification only for relevant boundary changes and release needs. “Uses Node” is not itself grounds for moving a useful subsecond test. |
| Built example handler tests and production browser suite | One candidate frontend build, many assertions over that build. Browser tests retain the claims that require Kit's client. |
| Packaged CLI and external plugin | Exercise the actual release archive as the existing release test does (`cmd/skgo-release/archive_test.go:82-125`). Keep an equivalent local plugin compilation only when changed plugin code needs pre-release proof; it need not run on every ordinary edit. |

“Build once” should mean once for each source revision, target and relevant compiler mode. Linux and macOS artifacts, race and ordinary Go objects, and before/after reload candidates are legitimately different. Rebuilding the same immutable executable for another assertion is waste. Reusing an artifact from a different revision and calling it current proof is also wrong.

One later, bounded release optimization is also visible: `cmd/skgo-release/main.go:94-105` gives every packaged release a fresh module cache, even for unchanged third-party dependencies. Its reason is exact SKGO tag freshness, and it separately checks `Origin.Hash` against the qualified SHA (`116-117`). Preserve that identity check and fresh candidate source while reusing immutable dependency downloads if this becomes a measured cost. Avoid broad deletion from Bob's shared module cache, especially while its other runner is active. This is follow-up release cleanup, not a prerequisite for fixing ordinary test frequency.

The simplest implementation order is: remove routine forced reruns and redundant triggers; route trusted Linux checks to Bob; stop the unused Zig archive build; simplify the identified whole-generation and repeated package-manager tests; then wire the small retained qualification set and remove its ordinary duplicates. Measure the resulting cold and warm executions on Bob before assigning a speed target. The evidence supports substantial avoidable work, but does not yet support an exact future runtime.

Evidence inspected: repository AGENTS.md; agent-protocol and adversarial-review skills; current Justfile and CI/qualification/release workflows; Bob runner compose/Dockerfile; generation, kitpatch, native, hotreload, adapter, CLI, Swift, example-handler and streaming tests cited above; supplied round3 timing output; parent-agent live GitHub and Bob inspection. Prior memory was used only to recover the established preference against blanket full-browser completion rituals (`MEMORY.md:4259`); all current workflow and topology statements were checked against this checkout or explicitly attributed live inspection. No Kit behavior change is proposed, and no dependency behavior was inferred from unpinned documentation.
