Part A is implemented and independently accepted by Opus 5.5. The final assessment of run `01M498TZ4MSB3BXVYT81SYMWRX.implement` returned validation_passed=true and no substantial gaps. This does not close issue 247.

The practical bottleneck is not simply a four-minute test run. It is repeated failures, lost diagnostics, redundant verification and feature agents being diverted into open-ended harness repair. Observed full Go runtime was 240.39s, of which the adapter package took 237.168s. Browser commands took about 47.5s production and 258.3s development; both passed 175 scenarios. These are local observations, not controlled CI benchmarks.

Operating approach for the remaining work:

- One independent validator owns acceptance and reuses applicable captured evidence. A new agent/handoff is not a reason to repeat every command.
- Run affected contracts during implementation and the agreed broad checks once at a stable source state. Repeat only checks invalidated by a relevant change or a concrete evidence gap.
- At first failure, preserve complete diagnostics and command status, then classify application defect, test defect, setup defect or unknown. Assess whether the test protects a justified user-facing promise and synchronizes on the correct event.
- Bound diagnosis. Fix change-caused defects in scope; report unrelated defects promptly and make an explicit blocking/nonblocking disposition. Do not silently waive failures or automatically turn feature work into a harness redesign.
- Passing retries do not explain prior failures. SKGo #250/#251 remain unresolved and are not claimed fixed by fixture isolation.

The repeated acceptance layers themselves caused work: final QA repeated an already demonstrated contact-break experiment, regenerated on restoration, invalidated the example's build-freshness timestamp, then had to rebuild and rerun example tests. Its final root/example checks passed, but this illustrates verification-created work rather than a newly introduced application defect.

Phase B readiness: fetched origin/main now contains `f0631f0` (#249), concrete typed-load parameters and tracking, including `internal/gen/load_params_test.go`. The typed-load evolution portion can proceed after rebasing this branch and establishing the planned fresh baseline. `internal/gen/shared_params_test.go` is still absent from this main revision, so verify the separate shared/remote-parameter integration before claiming that portion unblocked. Do not import unmerged APIs into this issue's scope.

Work is paused at the user's request. No Part B work has started. The scheduled monitor remains paused; no new Gimbal runs are authorized by this update. The broader findings and continuation are saved in `ephemeral/plans/20261006-testing-validation-rethinking.md` on branch `codex/issue-247-test-fixtures`.
