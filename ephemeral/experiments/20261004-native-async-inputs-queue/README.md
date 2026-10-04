# Stock Kit 3.0.0 asynchronous Inputs queue counterexample

Actual retained source and output from the independent Go-owned native reference. No Go bridge, adapter emulation, installed-native-source patch, or queue modification is present. The fixture uses the frozen Kit 3.0.0 and VitePlus 1.0.0 installation from this task. Its adapter is a native noop.

The ordinary prerendered root completes positively. The supported asynchronous producer then returns atlas/beacon after 500 milliseconds; its exact trace is `inputs:start\ninputs:end\n`, with zero body calls. Native `core/postbuild/prerender.js:721` rejects enqueue because `queue.js:40–44` closed the initially idle queue before the Inputs-seeding loop reached its explicit `q.done()` at prerender.js:728.

`go-test.log` PASS means the negative counterexample was reproduced and its literal trace/root output asserted; **the native build failed**, as `build.log` records. Independent driver remains at `../prerender-inputs-proof/native/native_test.go`, test `TestNativeDelayedAsyncInputsExposeClosedCrawlerQueue`. Maintained product regressions assert successful minimal builds and exact assets, so they currently fail honestly.

The constrained bridge cannot repair this without changing native queue lifetime or producer/body overlap. See the immutable round-08 review. Do not add unrelated page work, preload producers ahead of bodies, weaken assertions, or claim this negative as feature completion.
