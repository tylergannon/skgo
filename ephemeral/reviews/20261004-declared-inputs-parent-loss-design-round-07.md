# Declared Inputs parent-loss design review — round 07

Outcome: **no findings** in the corrected bounded design below. This is a design adjudication, not an implementation, completed-feature, merge or release approval. Round 05 implementation findings and round 06's actual CLI cancellation failure remain subject to correction and proof.

## Target and inspected evidence

Reviewed the proposed exact supported-CLI predicate and main-owner parent-loss hook against the authoritative Inputs scope, existing owner/session design, and installed VP 1.0.0/Kit 3.0.0 sources. Remained product/test/dependency read-only. Ran only read-only Node module/path resolution to compare physical CLI paths. No feature build or cancellation experiment was run by this reviewer.

The independent fresh `../prerender-inputs-proof/product-outer-vp-main-predicate-a5bec99.log` records a main-thread-only writer (`isMainThread:true`, `threadId:0`). Before cancellation, Vite PID 4245 had parent 4237; after SIGTERM to outer VP PID 4237, the parent probe returned ESRCH and Vite's parent became 1. Both receipts show argv `[node, <physical @voidzero-dev/vite-plus-core/dist/vite/node/cli.js>, "build"]` and `NODE_PACKAGE_MANAGER="vite-plus"`. Blocked Go PID/group 4375 remained owned by Vite 4245. The unchanged outer cancellation control failed its 20-second deadline at a5.

Earlier metadata receipts were written by both main and Kit workers, which share the process PID but have different argv. Those mixed-thread argv snapshots are not used to qualify the predicate. The clean log retains the full before/after metadata. The separate `product-fixtures/programmatic-predicate.json` records argv `[node]` and no package-manager marker, so that programmatic invocation does not match. These metadata controls do not replace the required behavioral controls after implementation.

## Corrected frozen predicate

Observe parent loss only for a main-thread owner with `NODE_PACKAGE_MANAGER === "vite-plus"`, `process.argv[2] === "build"`, and the real path of `process.argv[1]` equal to the actual bundled Vite CLI selected by the installed VP package. Missing argv or an unresolvable nonmatching optional launch path must not turn an otherwise unrelated programmatic build into a startup failure.

One source-backed correction to the proposal is required and incorporated here: resolve Vite from the VP package's module location, not directly from the app package. `vite-plus/dist/bin.js:1687–1699` uses its own `createRequire(modulePath)` to resolve `vite`; the app's Vite is only name/version-checked at lines 1669–1684 and may be a different physical copy. The resolver then selects the sibling `cli.js` at lines 1778–1783. For this installed package layout, use the app's `createRequire(app/package.json)` to resolve `vite-plus/package.json`, create the VP-local require there, resolve `vite`, and append sibling `cli.js`; realpath both sides. Read-only resolution confirms the app and VP-local paths happen to coincide at the current pin, but that coincidence is not the launcher contract. `DEFAULT_ENVS` in `vite-plus/dist/constants-BqJiMVVU.js:349–353` supplies the verified package-manager marker.

## Owner behavior

Capture the initial parent PID greater than 1 synchronously before owned Go work starts. While the eligible build session is active, observe a change in `process.ppid`, rather than treating a numeric signal-zero probe as stable parent identity. On the first loss/reparenting event, latch failure synchronously and join the existing fail/drain path before nonzero self-termination. Never signal an ancestor. Ensure observer callbacks handle cleanup rejection explicitly and do not introduce an unhandled rejection that bypasses the cleanup barrier.

The observation is session-owned, removed on cleanup, and must not hold a healthy completed build open. It belongs only in the matched main build process: no worker fallback, runtime-engine timer/I/O, CLI replacement, native dependency patch, or daemon. Ordinary completion and direct owner SIGINT/SIGTERM retain their existing semantics. Programmatic owners and deliberately detached programmatic processes fail this exact CLI predicate. These receipts establish the supported POSIX VP path; they do not establish arbitrary-launcher or Windows parent-lifetime behavior.

## Remaining proof

Retain separate direct-owner and actual outer `.bin/vp build` SIGTERM tests. Repeat the unchanged outer cancellation oracle after the correction: the real command must return nonzero without fixture timeout or rescue, with zero live owned Go children, absent private compile directory and no late application I/O. Also prove healthy CLI completion and the nonmatching programmatic path remain unaffected. A matching metadata receipt alone cannot prove cancellation. The seven acceptance groups and exact-head qualification remain the completion standard.
