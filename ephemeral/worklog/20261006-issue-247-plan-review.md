# Issue 247 plan review

correction: Concrete fixture planning must distinguish current main from active parameter branches; no single existing stable base currently contains all named issue tests. Implement existing-fixture changes first and parameter-fixture changes after integration, with separate comparable baselines.
decision: Checked-in fixture contracts use an explicit allowlist of the current example build bootstrap and dependency pins; never copy demo consumers or maintain independent per-fixture pins.
correction: Splitting the production artifact assertions exposes the existing global transform-middleware slot to ordering/race failures. Use fixture-local per-handler injection and local transform counters.
decision: Keep the deliberately failing layout fixture limited to /about, sharing bootstrap but not the successful route tree, so its diagnostic stays deterministic.

correction: Before estimating E2E migration savings, inspect current source and CI logs. Issue 136 still describes 154 scenarios, but main b3984d8 runs 175 per mode and example/contracts_test.go already contains the earlier HTTP-only migration.
decision: CI run 37256672613 spends 234 seconds in the Go-test step, including internal/adapter at 221.112 seconds and internal/gen at 127.767 seconds (packages overlap); ordinary CI runs no browser scenarios. Release run 37256672839 reports 55.8+5.4 seconds of prod Playwright and about 121+101 seconds of dev Playwright. Browser pruning targets release qualification, not the main PR-test bottleneck.
trap: The todo deep-link and rest-parameter scenarios look like HTTP checks, but their values sit behind Svelte pending boundaries; checking only server-rendered headings would not preserve their client execution claim. Inspect step definitions and component boundaries before classifying a scenario as removable.
decision: First-pass browser cleanup candidates are the two dynamic prerender entry rows, three HTTP-only source-edit cases (requiring real dev integration coverage), duplicated auth/stream scenarios, and conditional showcase consolidation. Do not promise a large suite reduction or wall-time saving until retained browser claims and replacements are mapped.
correction: User declined prioritizing modest browser pruning; the current promise is correctness and removing fixture obstacles, not a major CI speedup. Keep issue 247 focused on that result.
decision: Issue 248 records the separate future performance investigation: capture browser traffic from a pure-TypeScript Kit reference when example behavior or dependency pins change, then replay against Go without a browser. Feasibility, equivalent reference maintenance, retained browser confidence and total CI benefit must be established before adopting replay infrastructure.
# Gimbal execution constraint

decision: User authorized the Gimbal implement flow with Sol 6.1 implementation and Opus 5.5 independent validation. Retain workflow defaults for planning and scope review. Run in this worktree and monitor from the original chat at five-minute intervals.
decision: Main still lacks the typed-parameter integration at launch; dispatch Part A only and retain Part B as a required pending dependency. A successful Part A run does not complete issue 247. Use the existing Gimbal instance without creating another coordinator or altering the active parameter worktrees.

correction: Monitoring must assess run health, not just liveness: compare useful public results, pending command age, repeated failures, and validation progress; diagnose before recovery. User explicitly reinforced this on 2026-10-06. Five-minute bounded checks remain the cadence.

friction: Gimbal implement 01M497K2S3BYMEB2T0YC0CZ3S7 marked its broad Part A outcome complete after Opus accepted only the first task. Continue with separate ordered outcomes for failure isolation, adapter grouping and complete Part A integration proof; workflow completion is not scope completion.
correction: Opus discarded a payload mutation that failed setup, then demonstrated the targeted runtime status assertion (200 instead of 500) with a narrower restored mutation. Require actual failure causes, not filtered FAIL lines. Its GOWORK concern is already covered by package TestMain setting GOWORK=off.

correction: User requires causal investigation and filed bugs, not reassurance after repeated failures. Reported signal-contract intermittence as SKGo251, destructive validation-output filtering as Gimbal428, and added this run recurrence to Gimbal424. SKGo250 has captured crawler cleanup/EPERM evidence, but a shared cause with signal failures is unproven. Preserve full first-failure output and tested-command status before summaries; retry success is not diagnosis.

- decision: User authorized merging accepted Part A onto latest main without further deep validation; integration validation is deferred to Part B. Preserve this worktree and paused monitoring for continuation.
- integration: Main f0631f0 migrated load callbacks to generated RequestEvent while Part A relocated four callbacks into isolated testdata. Rebase retained the isolated fixtures and carried that signature migration into the three redirect loads and failure layout load; no runtime behavior was changed.
