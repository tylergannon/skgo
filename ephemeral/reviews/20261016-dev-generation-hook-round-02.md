# Independent implementation review, round 2: SKGO development generation

Outcome: **material findings remain**.

## Scope and evidence

Re-reviewed the entire current implementation in this worktree, including the tracked diff and all untracked implementation/test files, against the complete accepted development-generation plan, user authorization to implement generic SKGO integration now and defer Gimbal, repository instructions, and adversarial-review. Read-only except for this artifact. No review-scope narrowing was applied. The implementation remained frozen during the checks below.

Inspected `devgen`, development CLI/config/session integration, helper compilation/discovery/process/protocol implementation, retained capture/watcher/raw-event/admission code, all new focused regressions, the complete tagged real-session fixture changes, and the implementation worklog. Earlier findings are adjudicated against current code rather than repeated after repair.

## Finding

### Issue: unrelated Node compile-cache writes prevent generation from becoming current and can starve startup

Locations: `internal/gen/retained_watch.go:251–269`; `internal/gen/raw_events.go:34–53`; `internal/devserver/generator.go` final AcknowledgeRaw admission; `Justfile:23–25`.

The raw watcher treats every file event anywhere under every selected local module as an application-generation input. `RecordRaw` now excludes the private route mirror, but otherwise increments the global generation epoch for every path. The application fixture has a local SKGO dependency replacement, so SKGO's own worktree and its `.cache/node-compile` directory are part of these recursive watch roots. This directory is an explicitly configured Node tooling output (`Justfile:25`), not an authored generator input. Concurrent ordinary repository build/test commands write it.

Every such write invalidates the current callback acknowledgement and later guarded application admission, even after the application edit and generated output have settled. Because the raw gate covers dependency module caches as well as application inputs, development startup can remain in generation retry indefinitely while unrelated tooling runs. The callback cannot solve this just by selecting no producers: the newer raw epoch still rejects its successful false result before application preparation/admission.

I independently reproduced this with the revised frozen code:

`go test -tags=skgo_dev_integration ./internal/devserver -run '^TestRealSessionApplicationGeneration$' -count=1 -v`

Result: **FAIL**, 75.560s, timing out before initial serving. Its stderr diagnostics identify the exact event domain. For example:

- helper pass 1: epoch 5, paths include `/Users/tyler/.codex/worktrees/skgo-dev-generation/skgo/.cache/node-compile/v26.12.0-arm64-310b2ce3-501/5a659587` and sibling cache entries;
- pass 2: epoch 9, more entries from that same directory;
- pass 3: epoch 32, the intentionally injected `startup-observed.json` and generated `applicationoutput/value.go`, plus many unrelated cache entries;
- subsequent passes keep accumulating those cache entries and increasing epochs after the intentional application edit has finished. The test never admitted its initial origin.

The implementer's normal full/test checks were running concurrently. This is a realistic supported workflow and the source/paths explain its failure; it is not a missing dependency, timeout inferred solely from load, or the already-repaired stale-epoch loop. I did not create or modify cache files to manufacture it. The test's own output was very large because cumulative raw selections kept those unrelated paths pending.

Minimal resolution: keep known tool-owned output/cache activity outside the application generation freshness domain, including configured Node compile-cache roots in any watched local module. Preserve actual authored sources, generator controls, imported source dependencies and application-owned generated output repair. Do not suppress arbitrary generated headers or all dot directories: those can contain real declared inputs. Use the known tooling ownership/configuration boundary and verify selection coverage. Add a controlled regression that writes an explicitly configured tool cache during startup and still reaches serving, while a separate real generator input event remains capable of invalidating admission. Re-run the real integration with ordinary repository checks concurrently; passing only in a quiet tree would not resolve the demonstrated operational failure.

## Earlier findings resolved

1. **Initial retry:** `session.go` reruns `applicationGenerator.pass` inside the first Prepare/Commit loop, so an obsolete raw epoch is no longer permanently reused. The strengthened live fixture injects a non-Go event after the first generation pass and before initial preparation; source ordering now handles that case. Current live failure is caused by continuing unrelated cache events, not the old missing retry.
2. **Compiled native inputs:** discovery now includes selected assembly/header/C/C++/Objective-C/Fortran/object/SWIG inputs. The new assembly regression passed and asserts actual helper output changes from literal `101` to `202`, not just an internal fingerprint or PID.
3. **Descendant lifecycle:** helper and prerequisite now own process groups and stop them on replacement/cancellation. The descendant regression passed, observes real child PIDs, and requires them to disappear after helper refresh/close and canceled prerequisite completion.
4. **Opt-out retention:** raw state is allocated only when a raw consumer is selected, and RecordRaw returns immediately otherwise. The no-generator watcher regression passed; it creates/deletes 64 distinct paths, waits for an independently observed Go barrier, and asserts zero raw history and no channel allocation.

The private generated-link exclusion is narrow and justified by ownership: its authored originals remain independently watched. A focused assertion preserves application-owned generated Go as a raw input. This resolved the previous mirror publication feedback cycle and is not itself a finding.

## Independent check results and assertion assessment

Passed:

- `go test ./devgen ./internal/gen ./internal/devserver -run 'Test(SerializedCallback|RejectNonMonotonic|Raw|WatcherDeliversNonGo|Generator)' -count=1 -v` — protocol, raw-epoch admission, raw deletion delivery, narrow mirror filtering, helper retention/freshness/bootstrap, compiled assembly behavior and owned descendants all passed.
- `go test ./internal/gen -run '^TestWatcherWithoutGeneratorDoesNotRetainRawHistory$' -count=1 -v` — passed separately because the first regex did not select this name.
- `go test ./devgen ./internal/devserver ./cmd/skgo -count=1` — all packages passed, including devserver in 7.275s and cmd/skgo in 32.878s.
- `git diff --check` — no diagnostics.
- Production dependency selection `go list -deps . ./nativehost ./example/cmd` produced no `skgo/devgen` or `skgo/internal/devserver` matches. This is a reachability observation, not a claim that I built and inspected every production binary.

Failed:

- Tagged `TestRealSessionApplicationGeneration`, as described above. It is load-bearing: it requires literal generated data through the real handler, changed application PIDs after actual generation, stable process/mtime after no-op, healthy old responses after partial writes/death, a full replacement after recovery, helper survival across application replacement and helper renewal after source changes. But the revised run never reached initial serving, so those later live assertions remain unexecuted in my review run. The startup failure itself exposes a real raw-input boundary defect.

The focused tests are load-bearing for their stated units: the assembly test would fail without selected native-input freshness; the descendant test would fail if only the immediate parent were stopped; the opt-out barrier would fail with unconditional retention; and raw admission assertions would fail if the epoch guard were removed. None covers unrelated known tooling-cache churn, explaining how the new focused tests can pass beside the integrated failure.

All my owned check executions have completed; no review-owned runtime or pending broader test remains. Gimbal integration, browser UX qualification and all-target production binary validation remain outside the claimed proof of this generic slice. No new optional machinery or broader architecture change is requested.
