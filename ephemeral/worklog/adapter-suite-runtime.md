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
