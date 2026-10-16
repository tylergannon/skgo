# Independent implementation review: SKGO development generation

Outcome: **material findings remain**.

## Target and scope

Reviewed the current implementation in `/Users/tyler/.codex/worktrees/skgo-dev-generation/skgo` against the complete accepted plan `/Users/tyler/.codex/worktrees/dd88/skgo-project/ephemeral/plans/dev-generation-callback.md`, repository AGENTS.md, agent-protocol and adversarial-review. The authorized delivery is SKGO's generic integration now, with Gimbal deferred. Deferring Gimbal is not a review-scope exclusion: the generic contract and production separation still apply, but this review does not demand implementing Gimbal producers or adopting Polytype's new API in SKGO.

Inspected the entire tracked diff, all new `devgen` protocol, helper lifecycle/configuration/discovery code, raw-event acquisition/admission code, focused tests, existing surrounding session/replacement/watcher code, new integrated fixture and worklog. The implementer added the tagged integration fixture during initial inspection; I re-read its complete diff and requested a freeze. Subsequent review concerns the frozen implementation. I changed only this review artifact.

## Findings

### 1. Issue: a raw edit during first preparation permanently blocks startup

Location: `internal/devserver/session.go:102–126`.

The initial generation pass stores an acknowledged `applicationGenerator.epoch`, and `guardBuild` then requires that epoch on initial Commit. If any raw event arrives after that generation pass but before the initial Commit, Commit correctly returns `ErrObsolete`. However, lines 119–120 restart only `project.Prepare`; they never rerun `applicationGenerator.pass`. The epoch used by every later Commit stays permanently older than `project.rawEpoch`. Even after all editing stops, no build can pass this guard. The process keeps preparing and cannot reach Vite startup. This violates the plan's rapid-edit convergence and stale-selection recovery requirements.

I independently reproduced the startup failure by running:

`go test -tags=skgo_dev_integration ./internal/devserver -run '^TestRealSessionApplicationGeneration$' -count=1 -v`

Result: **FAIL**, 76.125s. The fixture timed out discovering its initial public origin; its complete session log was `skgo dev: preparing generated application`. No missing prerequisite or skipped test was involved. The implementer's independent run also failed its 75s startup deadline, reaching Vite but repeatedly compiling initial hosts without admitting one. That later failure is further evidence that startup raw-event convergence must be checked through the whole startup sequence; the exact event source in that second run has not been instrumented or established here.

Minimal repair: route every obsolete initial-build retry back through current generation before creating another compiler lease. Verify the full initial sequence, including frontend snapshots and seed publications, converges when tool-owned writes occur. Preserve the current guarded admission rather than simply deleting the epoch check. Add a controlled test that observes one raw event between initial generation success and initial Commit, then stops editing and requires startup to finish.

### 2. Issue: helper fingerprint omits real compiled assembly/native inputs

Location: `internal/devserver/generator.go:141–167,237–254,401–407`.

Discovery decodes and hashes only GoFiles, CompiledGoFiles, CgoFiles and EmbedFiles. Go's selected package also has SFiles, HFiles, CFiles, CXXFiles, MFiles, FFiles, SysoFiles and related compiler inputs. In particular, a local assembly helper dependency is fully compatible with a cgo-free Go program. Changing its existing `.s` implementation leaves every hashed Go/embed/module/config byte and path unchanged. Both rediscovery and identity comparison therefore return the old identity; `refresh` retains the old helper and calls stale compiled generator behavior.

Concrete reproduction construction: make the tooling main import a local Go package with a Go declaration for an assembly function returning a literal; have its callback use that result in generated output. Start development, change only the return constant in that package's selected `.s` file, then trigger generation. The watcher reports the path, but `discover` never includes its bytes, so the callback still executes the previous assembly. This is a source-derived causal reproduction, not a claim that I edited the tree to execute it.

The plan requires changes to the helper's actual compiled dependencies to use fresh code. Include Go-selected non-Go compiler inputs in the identity, with selected membership and controls. Add one literal native-input freshness test; the current new-membership test adds only `extra.go` and cannot catch this omission.

### 3. Issue: cancellation/replacement kills only the generator's parent process

Location: `internal/devserver/generator.go:56–62,104–120,302–315`.

`startGenerator` never calls the existing `ownProcess` helper, and `close` calls only `Process.Kill`. The pre-build command similarly uses bare `exec.CommandContext`. Both paths can spawn descendants: even the accepted pinned `go tool`/`go run` prerequisite creates a tool process, and generic application generators may execute pinned producer tools. Killing the immediate parent does not kill or join those tools. They can continue writing application output after helper replacement or development shutdown; a surviving child that inherits stdout/stderr can also keep command I/O copies alive while `Wait` is waiting for them.

This is an ownership/lifecycle correctness problem, not a hypothetical security warning. The accepted design places helper/prerequisite lifetime under SKGO and requires failed/replaced helper work not to race fresh generation. The repository already supplies process-group ownership and termination in `internal/devserver/process_unix.go`; the Vite side uses that established facility.

Minimal repair: apply equivalent owned-process-group cleanup and joining to generator and prerequisite processes, including cancellation/error paths. Add a small child-spawning fixture whose descendant cannot survive helper replacement or canceled pre-build. The existing tests inspect parent PIDs only and do not cover descendant ownership.

### 4. Issue: opting out still retains an unbounded raw path history

Location: `internal/gen/retained.go:105–108`; `internal/gen/retained_watch.go:269`; `internal/gen/raw_events.go:18–33,45–53`; `internal/devserver/session.go:81–91`.

Every Project now allocates a raw path map, and every interested watcher event records a path before filtering. Clearing happens only through `AcknowledgeRaw`, which is called only by generator passes. A normal `skgo dev` application without `--generator-config` has no raw consumer or acknowledgements, so every distinct created/deleted path remains retained for the whole session. Temporary output names, repeated directory membership changes and application/runtime files therefore grow this map even though the optional feature is disabled. Deleted paths are never removed.

This contradicts the selected optional integration's promise that applications without configuration keep existing behavior, and regresses sustained development-session memory use. Enable raw collection only for sessions selecting a raw generator consumer, or otherwise avoid retaining unused path history. A no-generator watcher test that creates/removes many distinct paths should assert no accumulating raw backlog; the existing raw tests always intentionally consume this feature and cannot establish the opt-out boundary.

## Executed checks and whether the assertions matter

Passed independently:

`go test ./devgen ./internal/gen ./internal/devserver -run 'Test(SerializedCallback|RejectNonMonotonic|RawInputs|WatcherDeliversNonGo|GeneratorRefresh|GeneratorTargeted)' -count=1 -v`

- Protocol serialization asserts literal request data and exact partial-write error replies, and rejects non-monotonic/corrupt requests. Removing the callback or discarding its changed result would fail these assertions.
- Raw-event tests assert compiler-independent paths, obsolete acknowledgement preservation, actual filesystem non-Go removal delivery, and prevention of a stale admission callback. Removing the raw epoch check would fail them.
- Helper tests assert parent process retention across unchanged refresh, rejection of invalid edited helper code, recovery with a fresh PID, new Go-file membership invalidation, and a literal pre-build output needed for successful compilation. Removing refresh/bootstrap behavior would fail them.

These assertions are load-bearing for their stated units. They do not qualify startup retry convergence, all compiled-input kinds, process descendants or the no-generator memory boundary.

The new tagged real-session test is also load-bearing in its intended scope: it reads a literal generated header through the real handler, checks application PIDs and saved project state, and attempts partial failure/no-op recovery, helper death after writes, output repair and fresh helper code. **It failed before reaching those observations**, as recorded in finding 1. Those later checks are unexecuted evidence, not passes. Its initial failure demonstrates why passing unit checks do not establish the integration works.

The broader command `go test ./devgen ./internal/gen ./internal/devserver ./cmd/skgo` was started independently and is **still pending** at artifact completion (execution session 14922). Its first output showed devgen passed; no verdict is claimed for the remaining packages. The parent requested completion of this actionable review without waiting for broader checks that cannot resolve the demonstrated startup defect.

## Scope and positive boundaries

The new protocol adapter is tooling-only by intended import direction; the application runtime package was not changed to import it. No Gimbal source, shipping generation command, production rendering contract or native compiler tag profile was changed in this SKGO slice. Boolean-result latching, explicit reconciliation and bypassing native Prepare/Install on a pending full replacement are present in the session implementation. These useful pieces do not resolve the lifecycle/freshness failures above. No production-binary absence claim is made from source inspection alone.
