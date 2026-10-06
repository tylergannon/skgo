The implement workflow's validator discarded the failure messages from an expensive failing test run, leaving no evidence to diagnose two failing tests. This was an agent-authored shell command problem, not evidence that Gimbal's transcript recorder dropped supplied output.

Observed with installed Gimbal `v0.12.2-0.20261001161540-4f2f025eef52`, run `01M498TZ4MSB3BXVYT81SYMWRX.implement`, Claude Opus 5.5 QA session `outcome.2/implementation.1/task.1/qa-orchestration.1`, 2026-10-06 19:10:52–19:15:47 UTC.

The only capture was:

```sh
/usr/bin/time -l go test -count=1 -v -shuffle=on ./internal/adapter 2>&1 | grep -E 'shuffle|^--- |^ok|FAIL|SKIP|real|maximum resident'
```

The 294.501s Go test run failed two signal-drain tests. Names, durations, seed and totals survived; assertion messages and child-process diagnostics did not. The validator later explicitly acknowledged this in its structured assessment. An isolated rerun and same-seed full rerun passed, so the original failure mechanism remains unknown. More execution did not recover the lost evidence.

An earlier mutation check in run `01M497K2S3BYMEB2T0YC0CZ3S7.implement` also emitted only filtered FAIL lines. Coordinator intervention prompted complete inspection: the validator discarded that attempt because it failed setup, then made a narrower mutation that failed the intended runtime assertion. This is a concrete risk of false load-bearing proof, although it was corrected before acceptance here.

A related shell hazard: without pipefail or explicit producer status, the pipeline status describes the final filter. `/bin/zsh -f -c 'false | cat; print pipeline_exit=$?'` returned `pipeline_exit=0` on this host. We have not established the original validator shell's pipefail setting; diagnostic loss is proven independently of that setting.

Expected: ordinary validation commands preserve complete stdout/stderr and the tested command's exit status before producing a concise display. A mutation counts only when its actual assertion failure is inspected, not simply when FAIL appears. Failed checks must retain enough evidence for the next agent to distinguish setup errors from contract failures. Address the command-authoring/validation guidance and meaningful regression coverage without adding a new proof framework or broadly restricting shell tools.
