User clarification: four-to-five-minute suites are survivable. The intolerable failure mode is repeated unexplained failures plus redundant checks, with feature agents diverted indefinitely into testing-harness research/repair.

Treat this as a bounded diagnosis with an explicit test-versus-application decision. The outer-launcher synchronization concern described above is grounds to re-evaluate the test contract, not proof that both failures are test defects. #250's captured cleanup failure must not be dismissed by that hypothesis.

Preserve the first full failing diagnostics and process/cleanup state; identify the user-visible guarantee and whether the test waits for the right event. Then fix the application, rewrite an invalid test, or reconsider an unsupported requirement and its machinery together. Do not increase sleeps, delete leak checks or repeat until green as a substitute for that decision.

Give unrelated feature work an explicit blocking/nonblocking disposition rather than an unlimited obligation to repair the harness. Report unknown causes accurately. Subsequent full unshuffled validation passed these tests; that does not establish their cause or repair. This issue remains open.
