Confirmed recurrence in a second SKGo outcome, with installed Gimbal `v0.12.2-0.20261001161540-4f2f025eef52`.

Run `01M497K2S3BYMEB2T0YC0CZ3S7.implement` supplied the reviewed Part A fixture-isolation outcome. Planner created four tasks: shared production fixture, isolated failing fixture, minimal adapter consolidation, and full Part A integration proof. Only the first executed.

At 2026-10-06 18:48:55.790 UTC, Opus explicitly submitted a structured result with `validation_passed: true` and `substantial_gaps: []`, covering the production fixture. Run completion was recorded at 18:48:56.234 UTC. The worker had explicitly stated that remaining Part A requirements and full-suite/browser proof were unfinished. The failing fixture and adapter consolidation were visibly still unchanged.

This was NOT an idle-session exit or crash. The assessment returned successfully and the implementation loop's passing-assessment branch completed the entire outcome. This corroborates the scope-confusion mechanism described in this issue; it does not establish that mandatory execution of every old planner task is the right fix.

Recovery uses three explicit remaining outcomes in run `01M498TZ4MSB3BXVYT81SYMWRX.implement`. Its failure-fixture outcome completed and it advanced to the adapter outcome, demonstrating that explicit caller outcomes avoid this particular premature stop. Distinguish task acceptance from actual whole-outcome completion; don't rely solely on caller granularity to prevent an incorrect success verdict.
