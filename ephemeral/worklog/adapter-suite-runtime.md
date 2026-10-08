friction: Main CI at 89208d3 measured adapter 146.318s and the ordinary full test step 182s; isolated timing cannot close the CI target.
friction: The relay test's timer-based late reply could arrive before Atomics.wait under compilation contention. Synchronize the late reply with an observed timeout so terminal-state assertions do not depend on worker scheduling.
decision: Mode-controlled failure builds can share immutable generated Go source while retaining separate Vite roots, Go compiler invocations, private executable directories, services and receipts. Relocate only the fixture manifest's Go root; keep generated service bytes unchanged.
friction: Sharing alone measured 14.107s isolated, 51.006s in the local suite and 184.239s in CI. Three assertions awaiting one shared build also occupy three parallel test slots. Package-level compiler contention must be measured alongside fixture reuse.
friction: CI hit the exact restored-file failure tracked in #265. Keep watcher delivery and its separate proposed deterministic restructuring outside this optimization, as requested.
friction: One-package scheduling brought adapter CI to 56.168s but the full step rose to 324s. Reject that schedule; improving a package's reported elapsed time is insufficient when the ordinary suite regresses.
correction: Adding a native JS route still requires regeneration of the Go route graph. Sharing an older graph let a 503 masquerade as the intended 500 crawler failure until the independent blocked-body receipt assertion rejected it. Keep that fixture separate and require its literal failure marker.
# Native-build type output and disposable linking

Just substitutes backtick output directly into recipe text. A `go list`
result must replace newlines with spaces before interpolation into `go test`,
or only the first package gets tested and subsequent names become commands.
Backtick evaluation itself propagates `go list` failure. The package list
keeps both original module patterns and only changes adapter scheduling.

The successful shared build must enter the parallel pool before its consumers
wait: doing setup before `t.Parallel` serializes it ahead of every other native
pipeline. Three consumers occupy three slots while sharing one build. Native
fixture executables can also be reused without bypassing compilation: seed each
private output with the immutable executable, then run the original `go build`
so Go validates its build ID and dependencies. Compiler-failure fixtures stay
fresh. The extra cached binary belongs to the shared generated module and never
to a native owner's private scratch directory.

Kit 3.0.0's `exports/vite/build/index.js` calls `sync.all` before compiling;
`core/sync/sync.js` writes all route/app types there. Running the real
`svelte-check` after the native build can therefore reuse that output and drop
the preceding standalone sync. The crawler failure page must likewise exist
before the single Go generation so its route graph is current. Disposable Go
fixture binaries need no DWARF metadata; `-ldflags=-w` keeps compilation,
linking, runtime behavior and native stack traces while reducing repeated
linker output. Overlay cases must preserve that flag when supplying GOFLAGS.

Reject the package ordering and parallelism overrides: ordinary CI remained
over the target and scheduling did not remove work. Restore the original
package patterns and Go's default test concurrency.

The TS/JS subprocess profiles put tens of milliseconds in GC, while the Go
test parent sampled only 60ms CPU over 8.94s. A GOGC=400 trial demonstrated no
improvement. Repeated native tool startup and compilation warrant attention
before GC policy. Keep Node's compiled dependency modules in the package-owned
temporary directory; children still execute independently and Node keys cached
code by module contents. Do not carry this disposable cache between test runs.

Kit 3.0.0's native build omits the client bundle when every node inherits
csr=false. Server lifecycle/compiler/caught-timeout fixtures can use that
mode, while preload and client-consumer assertions require their client build.
Its no-client path needs a real static asset to populate output/client before
the crawler walks it. The malformed-input fixture additionally needs the
client-side remote registrations: disabling CSR hit metadata mismatch before
the intended decode assertion, so keep its original client build.

CI's mise action installs the pinned Node but export_path=false leaves the
plain Go test recipe's child processes on the runner's Node. Run the same Go
suite through the example's mise environment and use Go's -C to retain the
root package patterns. Local runtime measurements must likewise use the pin.

Actual request CPU and test setup must be profiled separately. Disposable Go
overlays measured the native TS/JS prerender services from main through clean
shutdown at 13.885ms and 12.851ms CPU (Getrusage, excluding profiler shutdown).
The existing 500-home-document test used 72.557ms CPU to construct one handler,
1.169033s CPU/490.355ms wall in its request loop, and 13.443ms CPU in the three
explicit GCs. Its sampled cumulative allocation was about 1055MB, with 860MB
under Engine.Render and Unicode string concatenation the largest flat source
at 308MB. The whole example test process allocated about 5734MB, about 69%
under handler construction, including repeated asset hashing and Goja setup.
These figures identify actual allocation work; they do not establish a slow
request bottleneck in the adapter's native builds. CPU-profile goroutine
labels separate handler setup/request stacks; background GC needs its own
accounting and must not be treated as absent from either merely because it
does not inherit those labels.

Svelte 5.57.1's renderer collects string chunks with content[this.type] += item
in both synchronous and asynchronous collection. Pinned Goja's Unicode Concat
allocates a new UTF-16 array and copies the whole existing prefix. This is a
candidate for the measured SSR allocation amplification, not attribution of
every allocation to that loop; HTTP buffer pooling cannot reach that storage.

decision: Preserve a caller-supplied NODE_COMPILE_CACHE in fixture child
environments. A package-private cache discarded at exit cannot reuse compiled
dependencies from the preceding ordinary build. Just recipes now share one
ignored worktree-local code cache; fixtures retain private fallback when invoked
directly without it. Runtime instances, output directories, and native compiler
calls are still fresh. Validate changed module bytes with an already-populated
cache rather than treating the cache's presence as correctness evidence.

friction: Another worktree's concurrent Go/native-build suite contaminated the
latest local timings. The ordinary invocation passed the adapter but failed
strict pnpm version checks because the global shim resolved 12.10.1; those checks
passed with the installed 12.9.1 executable prepended to PATH. Do not infer a
speedup or regression from this contended local run; use ordinary full-suite CI.

decision: Pure endpoint and module-path checks need one actual Node invocation
per checker, rather than a process per supplied input. Capture every result and
assert every named refusal and positive case; missing results fail. The three
publication assertions likewise inspect one real pnpm tarball, each decoding
its own independent map and byte slices. This reduces nine checker launches to
two and three pack launches to one. A focused child-inclusive CPU comparison
was 0.29s before and 0.07s after (one observation, not a stable CI prediction).
Independent disabled-checker, wrong-positive-refusal, changed-diagnostic, and
real tarball membership/bytes/metadata mutations all failed retained assertions.

The externally supplied Node code cache must preserve module execution and
runtime isolation: changed adapter bytes rejected a real native build, restored
bytes yielded a new literal result and fresh producer/body receipts, and the
unchanged-byte negative control failed the changed-byte guard. Cache storage
survived package cleanup; owned tarballs did not. These are behavioral checks,
not conclusions from cached filenames or a successful command alone.

decision: Disposable adapter artifact/lifecycle fixtures use the public
precompress:false option; their assertions do not consume gzip/Brotli variants.
The native TS/JS artifact, type, and compiled Goja consumer checks passed in a
controlled overlay comparison. Child-inclusive native-build CPU was about
2.9s per language with compression and 2.55s without it (single observations).
Default compression remains exercised by actual native generator redirect
artifacts. Measure retained claims independently before delivery.

friction: Plain successful CI output reports package elapsed time but hides
per-test/stage logs. Temporarily collect Go's native JSON output from the same
ordinary Just test invocation, with TS/JS stage child CPU alongside wall time,
to distinguish repeated work from CI contention. Do not make a separate suite
or change concurrency for this measurement; restore normal output afterward.

decision: The native crawler-failure route can be part of the immutable shared
lifecycle graph if its source exists BEFORE generation. Its native server load
returns a control result in other lifecycle modes and throws the literal native
500 only in crawler-failure after observing the independent blocked Go receipt.
That lets all four lifecycle cases share one generation and compiled service,
while preserving distinct native builds/processes and original failure/drain
assertions. The earlier stale-graph experiment remains invalid; adding a route
after generation still cannot establish native crawler-failure coverage.

The ec4b199 ordinary CI JSON observation passed 1589 named events with zero
skips/failures but adapter elapsed was 144.352s; runner variation prevents
attributing that difference from prior 91.926s to the source change. TS native
build used 8.485s CPU over 22.044s wall; its svelte-check used 6.382s CPU over
18.896s wall. JS native build used 8.928s CPU over 34.579s wall and checker
6.059s CPU over 15.760s wall. Compilation/checking does real work and additionally
waits under full-suite contention. Native request service CPU remains a separate
millisecond-scale measurement, not the explanation for these stages.

Reject svelte-check 4.7.6's supported --incremental option for these fresh
fixtures: a cold trial still passed both language consumers but checker CPU was
1.75/1.86s, versus about 1.64s without it. Its virtual-file path adds a TypeScript
CLI process; there is no repeated check in these fixtures to amortize its cache.
Keep the existing checker. Restore plain Just test output after capturing CI
events; the same package patterns and native contracts remain in one invocation.

correction: Changing copied configs to precompress:false left the malformed
Inputs test's old adapter:skgo() replacement unmatched. Check the unique anchor
and install the permissive HTTP policy explicitly. Independent actual native
builds proved valid Inputs survive a supplied page HTTP500 only with that policy,
malformed Inputs still reject with the exact array diagnostic, and corrupting
the anchor fails before any native build. Silent replacement cannot prove policy.

decision: The shared minimal successful native build can also prove a server
hook may import a server-only module. Install its literal module/import before
publishing the generated fixture; retain a separate native unsafe client-import
build and Kit's original four diagnostics. Avoid repeating a compatible success
merely to assert the same permission beside its refusal.

friction: Identical generated Go modules relocated to two roots still compiled
eight packages and linked again (about 0.52s CPU each in a focused observation).
A seeded -trimpath relocation compiled/linked neither (0.12s CPU), but its cold
first-build/toolchain-cache cost in ordinary CI remains unmeasured. Do not adopt
new compiler flags based on the warm observation alone.

Independent actual native guard mutations caught wrongful rejection of the
shared successful server-hook import and wrongful acceptance of the separate
unsafe client import (whose emitted client JS contained the literal secret).
Restored server-only and malformed cases passed on integrated main's standalone
devalue runtime, with zero skips. Main's deterministic late-reply synchronization
replaces the equivalent task-branch synchronization during integration.

Reject the transform-only csr:false trial: retained transform/drain assertions
passed, but focused package wall was 4.132s versus 4.141s with its original client
mode, and command CPU showed no reduction (4.62s versus 4.35s). No source change
adopted from that observation.

Reject sharing the TS/JS service executable: captured native generated modules
had 31 Go/module inputs each, with eight differing files. In particular,
RemoteSpec.Module and server-load registration strings carry distinct .ts/.js
paths. Those builds are not interchangeable consumer coverage.

Profile the actual generator, not only its test parent: the focused native TS
build retained its assertions while the generator sampled about 100ms CPU over
1.34s process duration and allocated about 39MB. Child Go tool invocations are
part of that duration; the profile does not establish their GC cost. Generator
GC alone cannot explain the native build/checker stage costs.

Reject disabling native SSR source maps as a runtime optimization: a clean
focused pair used 5.36s CPU and about 4.7s wall in both modes; native-build CPU
was 2.463s on and 2.450s off. The experiment required changing component source
discovery because the current adapter reads .js.map files unconditionally.
Kit's build_server_nodes uses resolve_symlinks through Vite's server manifest,
but reversing that mapping with a uniqueness constraint needs alias/symlink
qualification. Restore both adapter and fixture changes; there is no measured
speedup to justify that compatibility work in this optimization task.

Do not adopt externalizing Svelte in native SSR from the first full-package
pair alone. That pair reduced CPU in unchanged generation/checker stages too,
and focused follow-ups were contaminated by other worktrees' Go tests. A later
focused native-build observation was 2.50s CPU default versus 2.96s external,
not a demonstrated reduction. Keep native bundling and Goja's mandatory fully
bundled graph unchanged until a repeatable saving is established.

The undeclared-client-ID refusal needs a generated same-module query control,
not another generator invocation. Add that literal Query to the immutable
minimal fixture and clone its generated Go registration into the refusal's
private frontend. Keep its real native build and original ID diagnostics.
Its page must explicitly override inherited prerender=true: otherwise Kit
correctly rejects query execution during prerender before our client-ID guard
runs. Exact assertions caught that incompatible fixture; restore the original
request-time query behavior rather than suppressing Kit's HTTP error.

Independent native controls caught removal of the actual copied client-ID
guard (the emitted client contained literal 3215r6/manual), proved a declared
query client call builds successfully, and caught a fifth Go registration.
For the valid control, remove the unsupported manual export as well as its
call: Kit can retain its constructor and literal ID in the client even when
the page calls declared(). All five restored affected consumers passed with
zero skips; disposable validation helpers were removed.

Reject globally adding -trimpath solely for disposable fixture cache reuse.
Go's buildActionID normally hashes the package directory; -trimpath substitutes
module identity but changes standard-library cache keys too. With ordinary
exports already warmed, a fresh trimpath cache cost 266 compiler invocations
and 24.7 CPU seconds before its relocated copy reused everything. Consistent
real-build/test warm runs saved only about 7 percent CPU (65.98 to 61.39s).
The ordinary full suite then failed: trimmed test binaries have no implicit
GOROOT, breaking importer.Default and source-location classification in gen.
Restore both the global flag and its malformed-test accommodation rather than
extend a modest optimization into unrelated generator/toolchain behavior.
Native independent controls did establish that trimpath itself preserves
changed-source invalidation, compiler diagnostics and private-directory drain;
the rejection is whole-suite compatibility and insufficient measured benefit.
