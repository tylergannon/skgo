# Group 1: Reliable development edits

## #330 fix(dev): recover from native relocation failures without silently serving stale code

https://github.com/tylergannon/skgo/issues/330

An ordinary body edit in `skgo dev` can leave the previous application serving HTTP 200 while the edit never applies and no normal revision/Vite error notification or automatic rebuild appears.

Observed while validating PR #329 on macOS ARM64 with Go 1.27.2 and project VitePlus 1.0.0. This is separate from the failed-structural-rebuild lifecycle repair in `78c04cd`; it occurred earlier, at the first compatible body edit, with the pre-fix session runtime selected through a Go overlay. The fixture in the committed test no longer includes the unnecessary mutex, so its passing result does not prove this loader defect repaired.

Reproduction shape:

1. Use the real-session fixture `TestRealSessionNativeRecoveryAndIsolation` in `internal/devserver/session_integration_test.go` on the PR #329 branch. In the route returned by `sessionRoute`, add a package-global `sync.Mutex` and wrap the `get` handler body with `mu.Lock(); defer mu.Unlock()`.
2. Run the real dev session through `mise -C example/web exec -- vp exec go -C ../.. test -tags skgo_dev_integration ./internal/devserver -run '^TestRealSessionNativeRecoveryAndIsolation$' -count=1 -v`.
3. The fixture initializes live state to 8, then changes only the handler response literal from `literal-4101` to `literal-5202`. The expected updated response is `literal-5202:8`.

Observed evidence from the retained local `ephemeral/structural-recovery-red-clean.log`:

```text
symName:sync.(*Mutex).Unlock relocateType:R_ADDROFF, symbol is not exist!
```

HTTP requests continued returning 200 from old code. The new literal never appeared during the 75-second assertion window. There was no subsequent normal `skgo: revision N:` diagnostic, Vite error notification, rebuild, or publication in the captured session output. The test failed at its expected `ALPHA literal-5202:8` assertion (86.67s total). The log is intentionally not committed as a repository artifact.

A blocked native-loader call or session loop is a hypothesis, not an established causal location. Applying another edit, restarting Vite, crashing the Go child, or restarting the whole session after this exact error was not exercised, so their recovery behavior remains unproved. Related issue #243 concerns authored Go load errors returning HTTP 500; this issue concerns native edit installation retaining stale HTTP 200 responses.

Desired behavior and proof:

- Failed or stalled native installation must surface a useful revision error through the ordinary development diagnostic path or take the normal full-rebuild fallback, within a bounded time.
- Preserve the previous healthy application until a replacement is ready, without publishing a successful refresh for an unapplied edit.
- The session must remain controllable and process subsequent source changes and child recovery. Demonstrate a corrected edit applying in the same running session after reproducing this failure.
- Add a regression at the real development boundary that fails when installation stalls or only emits raw loader stderr. Verify the error/retention/recovery behavior with literal responses and actual process identity; do not infer success from HTTP 200 alone.

This issue requests recovery and diagnostics for the existing loader path, not a loader redesign.


<!-- agent-session-group-20261018 -->
## Coordinated agent session: Development edit failure and recovery

Work together: #330, #113.

A running native Kit development session applies corrected edits after native installation failure, reports unapplied edits honestly, and remains responsive and coherent when route/layout structure changes during pending Go I/O. These issues share the real-session source-edit/recovery acceptance boundary; this grouping does not assert a common root cause. Preserve each issue's independent regression. The historical goja-development lock instructions in #113 are superseded by its current triage note.

Use the installed pinned Kit implementation as the specification. The session is complete only when both issues' retained acceptance criteria are satisfied; partial delivery does not close the other issue.

### Comment by tylergannon at 2026-10-18T16:57:41Z

**Triage — 2026-10-18**

Checked against current [`main` at `920b6aca`](https://github.com/tylergannon/skgo/tree/920b6aca738a65d6c046a67f4db2373596fafd03). Classification: `bounded-hard`.

The native loader and real-session recovery tests still exist under internal/hotreload and internal/devserver; replacing the old development renderer did not remove this failure surface. Current recovery covers several failures, but no matched sync.Mutex relocation-stall reproduction was found that closes this report. Keep open for bounded diagnosis and same-session recovery proof; HTTP 200 from old code is not acceptance.

<!-- skgo-issue-triage-20261018-920b6aca -->
## #113 Dev SSR: cover route edits while Go I/O is pending

https://github.com/tylergannon/skgo/issues/113

SSR #110 now lets Go endpoints and loads proceed while a dev page waits on Go I/O. Dev page renders remain serialized, preserving the existing relationship between the Go manifest and each Vite runtime's node table.

Add a focused browser regression for edits made during a pending dev render: change route/layout structure, let the render finish, then request the changed page and verify the expected route, markup, and hydration. Check that a Go endpoint remains responsive during the pending render. Exercise the real Vite server and inspect the asserted page screenshots.

The lock discipline has Go race-test coverage; this issue tracks the broader live HMR scenario deliberately deferred from #110. Do not introduce parallel dev page rendering without proving manifest/runtime generation consistency.


<!-- agent-session-group-20261018 -->
## Coordinated agent session: Development edit failure and recovery

Work together: #330, #113.

A running native Kit development session applies corrected edits after native installation failure, reports unapplied edits honestly, and remains responsive and coherent when route/layout structure changes during pending Go I/O. These issues share the real-session source-edit/recovery acceptance boundary; this grouping does not assert a common root cause. Preserve each issue's independent regression. The historical goja-development lock instructions in #113 are superseded by its current triage note.

Use the installed pinned Kit implementation as the specification. The session is complete only when both issues' retained acceptance criteria are satisfied; partial delivery does not close the other issue.

### Comment by tylergannon at 2026-10-18T16:58:38Z

**Triage — 2026-10-18**

Checked against current [`main` at `920b6aca`](https://github.com/tylergannon/skgo/tree/920b6aca738a65d6c046a67f4db2373596fafd03). Classification: `bounded-hard`.

The old development render lock/manifest generation mechanism was removed with #322/#331. A similar user-visible concern still exists in the replacement: route/layout edits while a Go load is pending must yield coherent Kit rendering and a responsive Go endpoint. Current session/handoff tests cover pending work and recovery, but no focused real-browser route-structure-edit-during-held-render scenario was found. Keep the behavior regression, explicitly superseding the old lock-preservation instructions; coordinate with #349.

<!-- skgo-issue-triage-20261018-920b6aca -->

## Initial orchestration assessment

Goal: #330 must surface unapplied/stalled native edits through normal diagnostics or a bounded rebuild fallback, retain healthy service until replacement is ready, and apply a corrected edit within the same controllable session. #113 must prove route/layout edits during a held Go load yield coherent real Kit page rendering and hydration while an independent Go endpoint stays responsive.

Proof: retain independent, load-bearing regressions for both issues. Use literal response values and actual process identity for #330; use the real Vite server and browser with inspected screenshots for #113. No full-suite loop or additional proof harness.

Potential design questions to resolve from the pinned Kit source, current code and cache before HITL:
- Does native installation expose a safe bounded failure/rebuild path without replacing the selected fast patch architecture?
- Which in-flight render behavior does pinned Kit itself prescribe when route/layout membership changes?

No product design choice requiring HITL is established yet. The issues already prescribe user-visible behavior and supersede the historic development goja lock design. Cache research must annotate this section before implementation dispatch.

## Token-cache annotation: #330 executable

Research entered through [/Users/tyler/src/skgo-token-cache/index/README.md](/Users/tyler/src/skgo-token-cache/index/README.md), then its development/native routes. These are prior-art observations, not current Kit authority or fresh execution proof.

- The selected architecture is already settled: [native Kit/UDS route](/Users/tyler/src/skgo-token-cache/index/dev-native-kit-uds.md:3) supersedes the older custom Go development renderer/graph-publication experiments. Normal compatible Go edits use native patching; rebuild/restart is fallback. This issue does not reopen that choice.
- The exact diagnostic is present as a returned error in cached loader source: [relocate.go](/Users/tyler/src/skgo-token-cache/repos/reload-native/pkujhd-goloader-current/link/relocate.go:338) rejects missing `R_ADDROFF`/method-offset symbols, and [Load](/Users/tyler/src/skgo-token-cache/repos/reload-native/pkujhd-goloader-current/link/ld.go:623) returns relocation failure. This supports investigating the complete installation/reply path; it does **not** establish the current report's blocked location or prove a safe cancellation mechanism.
- Historical [real-session observations and mutation](/Users/tyler/src/skgo-token-cache/index/dev-product-command.md:29) already distinguish acknowledged installation from literal response change and check replacement target identity. The [retained-publication report](/Users/tyler/src/skgo-token-cache/experiments/retained-publication-20261015/REPORT.md:18) demonstrates a bounded full replacement followed by native edits in the replacement PID. Neither exercises the reported mutex relocation failure, so neither closes #330.
- Current workspace inspection confirms an existing error/rebuild seam: [session installation](/Users/tyler/.codex/worktrees/e413/skgo/internal/devserver/session.go:456) recognizes `CompleteBuild` from install admission, and [Host.Install](/Users/tyler/.codex/worktrees/e413/skgo/internal/hotreload/host.go:415) records files only after an acknowledged exchange. Whether a stuck exchange needs bounded cancellation, escalation or another repair is a technical investigation for the builder, within the issue's retention/recovery constraints.

Question resolved: the requested behavior fits the existing selected architecture and fallback boundary. The precise native failure cause and safe bounded recovery mechanism are implementation work, not a missing product decision. **#330 is executable now; no HITL design assistance is required.** Acceptance remains the issue's independent real-session literal-response, honest-diagnostic, controllability and corrected-edit proof.

## Token-cache annotation: #113 executable

- The cache's [selected native Kit direction](/Users/tyler/src/skgo-token-cache/index/dev-native-kit-uds.md:3) explicitly supersedes the old split-renderer and custom graph-publication plans. Its historical qualification did not establish the complete dev browser suite. The [actual structural browser observations](/Users/tyler/src/skgo-token-cache/experiments/product-structure-20261015/OBSERVATIONS.md:7) showed added-route navigation and later removal, but also transient metadata admission failures; these observations did not hold a Go load while changing route/layout structure. They are useful prior art, not this issue's missing regression.
- The cache's [Vite/Kit route](/Users/tyler/src/skgo-token-cache/index/vite-kit-refresh.md:10) supplies graph invalidation and stale-transform anchors. It explicitly identifies older Kit/Vite artifacts, so its exact implementation must not be silently substituted for today's pin.
- Current authority verified in this worktree: [installed Kit package](/Users/tyler/.codex/worktrees/e413/skgo/example/web/node_modules/@sveltejs/kit/package.json:3) is **3.1.0**, matching the tracked [current-Kit skill](/Users/tyler/.codex/worktrees/e413/skgo/ephemeral/sveltekit-current/SKILL.md:8). Kit's [dev update_manifest](/Users/tyler/.codex/worktrees/e413/skgo/example/web/node_modules/@sveltejs/kit/src/exports/vite/dev/index.js:102) recreates route data, sync output and the SSR manifest; [route add/unlink watchers](/Users/tyler/.codex/worktrees/e413/skgo/example/web/node_modules/@sveltejs/kit/src/exports/vite/dev/index.js:225) debounce those updates. Each request invokes Kit's [configure/respond path](/Users/tyler/.codex/worktrees/e413/skgo/example/web/node_modules/@sveltejs/kit/src/exports/vite/dev/index.js:388).
- Kit's [generated dev manifest](/Users/tyler/.codex/worktrees/e413/skgo/example/web/node_modules/@sveltejs/kit/src/exports/vite/dev/generate_manifest.js:82) creates node loaders from discovered files and resolves their components through its runner. Its [page rendering](/Users/tyler/.codex/worktrees/e413/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/server/page/index.js:167) awaits server-load promises. These source slices locate the relevant behavior; they do **not** establish a universal old-generation/new-generation guarantee for a request spanning an edit. Preserve normal Kit behavior and demonstrate the requested post-edit page, instead of inventing a stronger transactional rendering policy or restoring the removed goja lock.

Question resolved: Kit owns route discovery, module invalidation and rendering in development. The user-visible acceptance is already specified: hold actual Go I/O, keep an independent endpoint responsive, perform a route/layout membership edit, release the held render, then demonstrate the changed route/markup and working hydration in the real browser, with inspected screenshots. Exact in-flight outcome is a technical mapping/proof matter, not an unanswered HITL product choice. **#113 is executable now; no HITL design assistance is required.** A regression-only resolution is valid if the current implementation already passes its load-bearing acceptance.
