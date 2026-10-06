Two native signal-drain contracts failed during a full shuffled adapter test run, then passed individually and on a full rerun with the same seed. Root cause is unknown. This needs diagnosis, not classification as harmless flakiness.

Observed 2026-10-06 in Gimbal run `01M498TZ4MSB3BXVYT81SYMWRX.implement`, Opus QA `outcome.2/implementation.1/task.1/qa-orchestration.1/turn.1`, on SKGo HEAD `f2376424a10024d00a647d79127c80ed945cd729` plus the uncommitted minimal-adapter-fixture consolidation. The production signal/drain code was not changed; that does not establish independence from fixture changes or test interactions.

```sh
go test -count=1 -v -shuffle=1791313852756971000 ./internal/adapter
```

Observed failures:

```text
--- FAIL: TestPrerenderInputsVPOwnerSignalDrainsBlockedProducer (53.04s)
--- FAIL: TestPrerenderInputsOuterVPCLISignalDrainsBlockedProducer (16.10s)
FAIL github.com/tylergannon/skgo/internal/adapter 294.501s
```

Both tests passed in isolation in 28.137s combined. The unchanged full same-seed rerun passed in 194.497s. No skips in those observed results.

Evidence limitation: the validator piped the original run through a filter selecting result lines, losing the actual assertion messages. We cannot now distinguish timeout, process-group survival, private-directory residue, receipt failure or another assertion. Neither failing test duration identifies that assertion. The diagnostic loss is being reported separately against the agent validation workflow.

The tests cover different entry points: SIGTERM sent to the real Vite owner versus SIGTERM sent to the outer `vp` CLI. Both expect non-successful build termination after the owned Go process group is drained, its private compilation directory removed, and no descendant performs late I/O. See `internal/adapter/prerender_inputs_lifecycle_test.go`, the two named tests and `assertInputsReceiptDrained`.

Expected investigation: capture the full first failing output and producer exit status; identify the failed lifecycle stage and the actual outer/Vite/Go process ownership and group state at that point. Keep the existing assertions. Do not increase deadlines, skip tests or count passing retries as a repair without causal evidence.

Related #250 records a different native crawler-failure symptom with captured `kill EPERM`, a live group and retained directory. A shared cause is NOT established. #247 explicitly does not promise to repair all lifecycle defects.
