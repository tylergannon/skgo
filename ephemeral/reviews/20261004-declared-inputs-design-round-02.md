# Declared prerender Inputs design review — round 02

Outcome: **no findings**.

Review target: corrected design at `ebb8d980d319487c7466579a5601807fdc45ebeb`, including the entire Inputs worklog, referenced extension design, round-01 artifact, and the parent's additional required Go-owned programmatic buildApp failure proof. This is a design verdict only; it does not qualify the ongoing implementation or any acceptance group.

## Evidence inspected

Reapplied agent-protocol and adversarial-review. Read the complete amendment, current worklog, original immutable review, and relevant surrounding source. Reverified installed Kit **3.0.0**. Rechecked installed VitePlus **1.0.0** application-hook dispatch, configResolved timing, hook sorting/cache, environment close and CLI exit. Also inspected Kit's compile/finalise separation, existing adapter buildApp hook, the original native Inputs/transport/worker boundaries mapped in round 01, and the current uncommitted Go/generator diff as surrounding context. No product file, test, dependency, worklog, or prior review was modified; no build/test/process experiment was run.

The targeted rereview request was treated as prioritization, not a restriction on reporting another material defect. The complete original Inputs goal and seven acceptance groups remain the standard. Only this new review artifact was written.

## Round-01 finding is resolved in the design

The amendment at worklog line 19 now installs main-only, catch-only wrappers in configResolved on every resolved plugin buildApp handler and the configured builder.buildApp callback. Each failure awaits the same owner cancellation/close promise before rethrowing the original error. This supplies the missing awaited boundary before the outer application build rejects or the CLI exits.

Installed source supports that placement:

- Vite `dist/vite/node/chunks/node.js:42768–42773` finalizes resolved plugins, installs hook utilities, and awaits configResolved before continuing; `createBuilder` resumes after resolved configuration at lines 40205–40207.
- Its sort cache holds plugin object references (`39181–39190`). The build loop subsequently reads each current `p.buildApp` and awaits its handler (`40218–40224`). Preserving hook metadata/order therefore retains native scheduling even if a sorted plugin list was already cached.
- The configured builder callback is a separate await before the first post hook (`40220–40226`), so wrapping it is necessary and the amendment includes it.
- Kit has separate `vite-plugin-sveltekit-compile` and `vite-plugin-sveltekit-adapter` hooks (`src/exports/vite/build/index.js:101,439,1007–1015`). The former owns native analysis/prerender and subsequent manifest/remote processing; the latter owns finalise, including service-worker build before adapt. Wrapping all resolved hooks covers both and intervening plugin failures.

The wrappers preserve this, arguments, return values and error identity; duplicate wrapping/shared plugin guards are explicit. Successful compile keeps the owner alive for later native phases. Existing adapt-finally cleanup supplies the successful completion boundary, while failure wrappers cover failures that never enter adapt. Environment closeBundle remains non-terminal for a healthy owner. The correction does not require changing native Kit/Vite sources or a global mutex.

The required programmatic buildApp failure-before-adapt proof is material: observe zero live children and no private compile directory before the outer promise rejects, while the test process remains alive. Together with native crawler failure and controlled SIGTERM, it distinguishes an awaited drain from apparent success caused only by synchronous CLI exit cleanup. These are still required future proofs.

## Remaining design assessment

No additional design finding remains. Worker-owned app transport encoding/decoding is now committed at worklog line 21: only serialized protocol strings and envelope metadata cross parentPort. Inputs are revived in the requesting worker before native canonical enqueue; body arguments are encoded there before main-process execution. This preserves class identity and native raw keys without requiring app hooks in the main owner.

The typed API remains explicit: Raw denotes the actual existing body argument type, so a string body uses `func() ([]string, error)` and Money uses `func() ([]Money, error)`. It is not a proposed exported alias for any. Noarg permits the explicit undefined sentinel producer. The remaining clauses still require positioned signature/reserved-key errors, lazy producers without Event, native response sharing, fatal producer/infrastructure errors distinct from body responses, fixed-command session ownership, permanent compile failure, process-tree cancellation, signal draining, and an inert goja helper.

During rereview, the uncommitted API draft introduced `type Raw = any` and []any-only producer checks, which diverge from that concrete-type requirement. I immediately notified the parent for correction. That in-progress implementation is not the reviewed amended design and receives no approval from this no-findings verdict. Final implementation review must confirm the concrete-type contract and run its mismatch controls.

The seven acceptance groups remain appropriately tied to native builds and literal observations, with the added application-boundary failure proof closing round 01's gap. No remaining design blocker or genuine nitpick was identified. Implementation, independent negative controls, regression qualification and release review remain outstanding.
