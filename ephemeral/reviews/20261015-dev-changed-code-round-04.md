# Development-server repairs: independent complete review, round 04

Outcome: **no findings**.

Reviewed the complete current implementation and repair diff from `1d9e23e1760266cc83367c00999105e28db19944` through `02c1a9a`, including the working tree and surrounding code. Authorities remain the user's authorized repair request, repository AGENTS instructions and agent-protocol, current `ephemeral/projects/dev-server/{GOAL,DESIGN,PLAN,CONTINUE}.md`, the original development-server review in skgo-project, and installed Kit 3.0.0. The token-cache current architecture route and Kit source mapping inspected in earlier rounds remain applicable. Reloaded adversarial-review for this round. No scope narrowing was supplied or applied.

## Findings

No material findings or genuine nitpicks remain from this review. The no-findings outcome concerns the implementation and evidence inspected; it does not certify unfinished broad qualification or a release.

The review covers the complete repair set: Vite configuration epochs and Go constructor reload, remote-prefix publication, captured compiler/workspace selection, type-aware FetchConfig instrumentation preserving comments, explicit NativeDevelopment construction, load scheduling and parent demand, response effects, fetch payload transport and metadata, logical-owner cleanup, concurrent continuation-body ownership, explicit producer cancellation, and session shutdown. The earlier concurrency and Close findings are resolved in current source and have load-bearing regressions; they are not repeated as current findings.

The only changes after the round-03 target are the adapter fixture's readiness assertions and its worklog. The fixture now waits for a real Unix listener after configureServer writes its marker, retrying only missing/refused startup connections. It does not retry HTTP failures at the strict readiness assertions. The dotenv assertion awaits a new configuration epoch on the same private socket, replacing the old fixed sleep and incorrect expectation that the configuration instance stays unchanged. Environment contents, privacy checks, route changes and Go callback assertions remain. This matches the repaired in-process Vite restart contract. The product's `startFrontend` already probes the HTTP state endpoint before accepting startup readiness, so the fixture change does not substitute marker existence for product readiness.

The explicit development constructor preserves ordinary fs.FS composition and shares application setup with production. Captured Go environment/workspace inputs reach analysis, compilation and runtime; unsupported selection changes replace the host. Request effects are emitted once across hooks/actions/concurrent loads; Kit parents are requested only on actual Go Parent use. Continuation upload lifetime and next-step negotiation remain separate, while explicit Go Close cancels the individual JS producer without draining it. Those responsibilities remain within the selected architecture and production Go/goja construction remains separate.

## Evidence inspected and verification boundaries

- `/tmp/skgo-dev-repairs-prod-browser.log` ends with **176 passed (41.6 s)**, including Chromium and JavaScript-disabled scenarios. This is the parent's full production-browser execution, not a development-browser result.
- `/tmp/skgo-dev-repairs-final-session.log` ends with **TestRealSessionNativeRecoveryAndIsolation passed (75.95 s), package 76.191 s**. The log contains expected temporary upstream errors around deliberate replacement/crash recovery; the eventual pass is supported by the fixture's literal behavior and configuration assertions. It is not a no-errors-ever claim.
- Production build evidence inspected in round 03 completed successfully with the production adapter/goja output. No product source changed after that target; the new commit changes fixture readiness only.
- Fresh round-03 private-root verification at unchanged product source: native Bridge/Fetch/Development group passed ten repetitions in 6.127 s, and the same focused group with `-race` passed in 1.912 s, using VitePlus-managed Node v24.22.0. These cover the previously reproduced concurrency and explicit Close failures, progressive delivery, large request payloads, parent scheduling, header effects, metadata and owner cleanup.
- The broad Go run is parent-owned and starting during this round. A completed full development-browser result was not supplied. Neither is inferred from production or focused bridge success. No redundant test run was started for the fixture-only commit.

Only this requested review artifact was written. No shared application, generated example/build artifact or implementation source was modified. The accepted direct-SSR-subfetch automatic cookie-propagation limitation and existing platform/qualification boundaries remain explicit. This outcome does not claim new latency measurements, Linux qualification, release publication, merge or deployment.
