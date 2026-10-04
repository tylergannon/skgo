# Declared Inputs CLI cancellation review — round 06

Outcome: **material findings remain**.

Target: the main-owner lifecycle design and immutable implementation `a5bec993d903bc757526042770df638cdb349e6c`, in light of an actual outer-CLI cancellation counterexample. This targeted adjudication supplements round 05; it does not approve the whole feature or resolve its other findings.

## Evidence inspected

Read the installed VP 1.0.0 source in this worktree, the locked Inputs worklog and full extension-plan lifecycle clauses, and the independent Go proof source and fresh receipts. No product/test/dependency files were changed and no additional process experiment was run by this reviewer. The published package does not include the native binding's Rust implementation; no claim below relies on an inferred native signal-forwarding implementation.

The installed `example/web/node_modules/.bin/vp` shell shim uses `exec node .../vite-plus/bin/vp`; the shim does not remain as a separate shell supervisor. `vite-plus/bin/vp` imports `dist/bin.js`. In `vite-plus/dist/bin.js:1778–1790`, the Vite resolver returns a separate bundled CLI path and environment. The entrypoint passes the resolver and `process.execPath` to native `run` and exits with its result. `vite-plus/binding/index.d.cts:3511–3528,3683–3686` declares that native interface. The actual bundled Vite CLI at `vite/dist/vite/node/cli.js:765–786` creates the builder, awaits `buildApp`, and exits nonzero on a caught build failure. The adapter owner is installed inside that Vite process, not inside the outer VP launcher.

Read actual files under `../prerender-inputs-proof/product-fixtures/outer-vp-implementation-a5bec99/`:

- `node-before.json`: Vite owner PID 91523, parent PID 91515, Node executable `/Users/tyler/.vite-plus/js_runtime/node/24.21.0/bin/node`.
- `node-after.json`: the same Vite owner and executable, parent PID 1 after outer cancellation.
- `body.ready`: blocked Go PID 91551, parent PID 91523, process group 91551, executable inside private `skgo-prerender-kHrPNC`.
- `../prerender-inputs-proof/product-outer-vp-parentloss-a5bec99.log`: SIGTERM targeted outer VP PID 91515; an independent signal-zero probe subsequently returned ESRCH for it; the build wait did not complete within 20 seconds. The harness later cleaned its fixture groups and private directory.

Earlier `ps` attempts were sandbox-denied. Their empty files are not ancestry evidence. The direct same-process PID/parent-PID/executable receipts above establish the relationship without those files. The independent and maintained owner-target SIGTERM positives remain separate evidence: they signal the config-recorded Vite PID and do not establish cancellation of the outer command.

## 1. Issue: terminating the supported VP command leaves the main owner and Go work orphaned

The owner handles SIGINT/SIGTERM received by its own process, but it has no observation of losing its launcher. In the reproduced case, SIGTERM kills the outer VP process. The Vite owner receives no effective cancellation, reparents to PID 1 and keeps the blocked Go invocation alive in its detached process group. Inherited output pipes prevent the Go command wait from completing even though the outer process is already gone. Thus a nonzero outer process status is not a cleanup proof.

The original plan explicitly locates signal handling in the main Vite process, so an owner-target test is a valid proof of that literal mechanism. It is not a proof of the supported user's `vp build` cancellation. The authoritative Inputs assignment includes real native cancellation with zero live owned children, absent private compilation files and no late application I/O; the observed orphan remains work owned by the same main build process. Treating the launcher as an upstream exclusion after discovering this case would leave that concrete cancellation path broken.

## Bounded correction

Add parent-loss cancellation to the existing main-owner build session. Establish the launch-parent relationship before spawning owned Go work, observe loss/reparenting while the session is active, and route that event through the same latched failure, awaited drain and terminal nonzero path used for direct owner signals. On the observed POSIX platform, a change from the captured `process.ppid` directly exposes reparenting and avoids treating a reused numeric parent PID as proof of identity. Observation must be session-scoped, removed during cleanup, and must not by itself hold a healthy completed build open. It must not run in workers or the runtime engine graph.

Do not signal ancestors, replace the VP wrapper, patch native Kit/VP, add a daemon, or invent worker-local subprocess ownership. The already-live main owner can cancel its own jobs. Preserve the original build failure and surface cleanup failures as required by round 05. If the parent-loss policy is applied beyond the supported CLI relationship, explicitly account for intentionally detached/programmatic builds instead of silently adopting a new process-supervision contract. The receipts establish the POSIX case; they do not establish Windows parent-lifetime semantics.

Keep two distinct Go-owned controls: direct SIGTERM to the config-recorded Vite owner, and direct SIGTERM to the actual `.bin/vp build` process after blocked-body readiness. The latter must now terminate without a fixture timeout or harness rescue, with the same zero-live-owned-children, absent-private-directory and no-late-I/O assertions. Preserve the observed negative receipt and repeat the unchanged outer-CLI oracle against the correction. The seven acceptance groups and exact-head qualification remain authoritative.
