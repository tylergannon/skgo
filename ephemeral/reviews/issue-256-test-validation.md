# Issue 256 test validation

Independent test assessment against issue #256 and installed SvelteKit 3.0.0. No implementation diff audit, new framework, or duplicate full build was used.

## Focused handler and socket run

Executed once after the Go owner initially reported stable, before the later lifecycle corrections. The full repository run and final adapter run below cover the completed implementation:

```text
go test . -run 'TestPrerender(Service|TypedLoad|Load|Remote)|TestAPanicking(Query|Command|Refresh)|TestAPanicIsReported|TestARemoteRedirect' -count=1 -v -timeout=45s
PASS
ok github.com/tylergannon/skgo 0.346s
exit status: 0
```

All 20 selected top-level tests and six subcases executed and passed; zero skips. The panic tests emitted their deliberately authored server-side panic diagnostics and passed their HTTP response assertions.

The assertions carry the intended behavior:

- `TestPrerenderLoadReceivesKitsEventAndReturnsGoEffects` requires the independently supplied parent, URL/query, route/parameter and cookie receipt `parent:42:/items/[id]:preview:hello`, header `from Go`, and outgoing cookie `seen=yes`. Missing application execution, replacing Kit's parent, or losing effects fails this test.
- `TestPrerenderTypedLoadConvertsRawPathToNamedMatcherResult` requires the typed matcher result `number:42` from `/orders/00042`.
- `TestPrerenderServiceLoadSettlesTransportedDeferredValue` requires immediate Money 1250 and exactly one settled chunk, ID 1, containing Money 990. `TestPrerenderServiceRemoteSharesTransportAndPublicSanitization` requires transported Money 780, original lowercase application header handling, private build diagnostic `private database detail`, and sanitized public `Internal Error`.
- `TestPrerenderRemotePreservesNativeErrorOrigin`, `TestPrerenderRemoteRedirectStaysStructuredForKit`, and the load error tests require authored 404/403 errors, unexpected error/panic classifications, diagnostic preservation and structured redirects with literal status/location.
- Declared-input tests require exactly atlas/beacon, no fabricated application Event, propagated producer failure, and explicit undefined encoding. Authentication requires 401 with no application call.
- Concurrent calls require independent alpha/beta URLs, cookies, headers and outgoing effects while preserving the original application Authorization header. Real sockets prove client disconnect cancels the producer and owner EOF cancels active work, terminates the service, and closes its listener.

## Existing build assertions

The minimal-inputs build tests compare literal native artifact paths and bodies for no-argument, atlas and numeric inputs; they do not discover expected values from generated output. Production fixture contracts compare literal native remote artifacts, document content, redirect bytes, and zero runtime body calls.

The lifecycle tests require the shared helper's process group and temporary directory to be gone when the actual build promise rejects. Crawler failure asserts Kit's original failure. Validation identified a missing authored-producer-message assertion in `TestPrerenderInputsBuildAppWaitsForProducerFailureDrain`; the adapter owner added the exact literal `declared Inputs producer lifecycle failure` to the existing test. The outer-launcher signal test waits for the actual Vite owner to exit before checking cleanup.

The initial socket-only timeout check was replaced by `TestPrerenderCallbackTimeoutFailsBuildDespiteApplicationCatch`. It builds the smallest actual Kit app with a Go prerender remote that waits for its request context to be cancelled. The page catches that callback's error and returns a usable fallback, so an uncaught application exception cannot supply the expected terminal build failure.

The final assertions require all of the following together: the application caught the literal `remote src/routes/timeout.remote.ts/blocked timed out after 30000ms`; `buildApp` rejected specifically with `skgo prerender service exited unexpectedly: 1` or `SIGKILL`; the Go callback wrote literal `cancelled\n`; and the helper process group and private directory are gone when `buildApp` rejects. Validation identified and corrected the initial generic `BUILD_APP_REJECTED` assertion, which could have accepted an unrelated later Kit failure. The strengthened owner/helper-specific rejection closes that gap. The test has no skip path.

The initial version passed in `ephemeral/adapter-real-timeout.log`. The strengthened version passed in the uncached full repository run and again in the final complete verbose adapter-package run. The latter exposes the native build output directly in `ephemeral/adapter-output-drain-fix.log`:

```text
APPLICATION_CAUGHT_CALLBACK:skgo prerender remote src/routes/timeout.remote.ts/blocked timed out after 30000ms
DRAIN_OBSERVATION:{"pid":60900,"group":60900,"groupAlive":false,"privateDirExists":false}
BUILD_APP_REJECTED:skgo prerender service exited unexpectedly: 1
--- PASS: TestPrerenderCallbackTimeoutFailsBuildDespiteApplicationCatch (32.80s)
```

The same captured output shows the Go service classifying its canceled active callback and stopping. Kit continues transforming and rendering after the application catches the timeout; the final rejection identifies the owned helper failure, rather than an unrelated Kit error. This is the actual swallowed-timeout composition that the earlier stand-in HTTP test did not prove.

## Inspected application output

Independently inspected the final `ephemeral/build-output-drain-fix.log`, which records `just build exit status: 0`, and its resulting example build:

- Exactly one `skgo prerender service started (pid 58248)` and one matching stopped line, followed by `done`. A direct `ps -p 58248` found no remaining process.
- `/about.html` contains About, parent `skgo example`, transported `$7.50`, and `Go prerender remote: atlas`.
- `/about/__data.json` contains both parent and child nodes, child `parentDeployment` equal to `skgo example`, Money cents 750, and settled chunk ID 1 with `GO_PRERENDER_DEFERRED`.
- Both declared remote artifacts exist and contain authored results `Go prerender remote: atlas` and `Go prerender remote: beacon`.

These are observed results from the real multi-callback Kit build. Independently read `ephemeral/browser-output-stable.log`: all five `engine-build.feature` scenarios passed, including prerendered parent/page loads, dynamic Go load entries, reused remote artifacts and Kit's remote result ID. The root owner also reports inspecting the final About page in Chrome and seeing all four authored values.

## Final repository and lifecycle result

Read the complete captured `ephemeral/test-output-validated.log`: `go test -count=1 ./... ./example/...` passed every package and records `just test exit status: 0`. The root package passed in 2.316s, adapter in 138.094s and generator in 62.340s. This run occurred before the final JavaScript-only Darwin drain correction; no Go execution or generator source changed afterward. The earlier failed/interrupted runs are diagnostic evidence; they are not counted as passing validation.

The final drain correction retains transient Darwin EPERM as a cleanup cause without assuming Node has already observed the leader's exit. It still requires bounded reaping and actual process-group absence. The final rebuilt example passed, followed by the complete unchanged adapter package against that final source: `go test -count=1 -v ./internal/adapter`, `PASS`, `ok github.com/tylergannon/skgo/internal/adapter 109.509s`, `go test adapter exit status: 0`, captured in `ephemeral/adapter-output-drain-fix.log`. Every adapter test executed, with zero skips/failures. Named passes include producer/crawler failure drain, malformed compiler input, actual-owner interruption, outer-launcher interruption, declared-input artifacts and the actual-build caught timeout above.

Also read the final-source generator capture `ephemeral/generator-output-drain-fix.log`: `go test -count=1 ./internal/gen`, `ok github.com/tylergannon/skgo/internal/gen 43.280s`, `go test generator exit status: 0`. Generator application fixtures therefore have a successful run against the final adapter source, in addition to the preceding complete repository run. Unaffected Go checks retain that earlier full-run evidence.

Read `ephemeral/vet-output-validated.log`: `go vet ./... ./example/...`, `just vet exit status: 0`.

A repository test-source skip audit found only `internal/gen/format_test.go`, whose skip is conditional on Windows. This Darwin run cannot take it; affected handler, service, timeout and lifecycle tests have no skip paths. Packages marked `[no test files]` are not represented as tests that ran.

No material test-correctness findings remain in this assessment. The actual swallowed-timeout-to-terminal-build-failure guarantee has a load-bearing application test with visible native output; the final affected-package run and real application rebuild passed. Windows runtime behavior was not exercised on this host.
