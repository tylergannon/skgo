# Form actions implementation workflow: inefficiencies

This report is being written while the final proof stage runs. It records work that consumed time without adding required product behavior. Final timings and disposition will be added after the run finishes.

Gimble run: `01M3AJSZ430H9JX3GDX4TKPY4N.implement`, started 2026-09-24 20:48:10 UTC. There is one application under `example/`; the Playwright “projects” below are browser test configurations within that application.

| Stage | Elapsed time | Scope completed at that point |
| --- | ---: | --- |
| 1 | 37.8 min | Typed save and reproducible generated bindings |
| 2 | 18.2 min | Validation and archive; redirect and errors carried into Stage 3 |
| 3 | 178.8 min | Six composition tasks with repeated stage-wide QA |
| 4 | 68.5 min | Four form-data, protocol, declaration, and trusted-origin tasks |

These are stage wall times from Gimble's run record, not estimates of avoidable time. The portion attributable to each inefficiency still needs a separate audit; the entire Stage 3 duration must not be called waste.

## Confirmed findings

1. **Broad outcomes caused repeated planning and validation.** The temporary Gimble input described each plan stage as one outcome, but the workflow validates one selected task at a time. Stage 2 advanced after the archive task passed, before redirect and error actions existed. Stage 3 had to backfill those actions. To avoid another premature advance, the QA session after each later task was steered to assess the entire stage. This kept missing work visible, but repeatedly ran already-proved journeys: Stage 3 QA eventually reran 46 existing composition scenarios per mode, then 62 scenarios per mode once rendering options were added. The same broad-outcome issue repeated in Stage 4. **Next time:** give the implement workflow outcomes small enough that a task-level QA pass can legitimately close them, then run one full-stage regression at the end.

2. **Batch image viewing falsely displayed valid screenshots as white.** During shared-route and page-option proof, white frames in a batch preview prompted speculative screenshot and timing changes. Individual PNG inspection showed byte-identical files to visibly correct frames, with hundreds of colors; the original CDP captures contained the expected page. The feature-specific Playwright capture path and unnecessary waits were reverted. **Next time:** inspect the original PNG individually, and compare hashes or pixels, before changing screenshot capture or application timing.

3. **The temporary workflow input misstated the browser matrix.** It said “three browser projects.” There is one example application, `example/`. Its existing Playwright configuration calls its two browser test configurations “projects”: Chromium with JavaScript and Chromium without JavaScript. The accepted plan requires three submission paths across those two configurations. My wording caused the final-stage planner to flag a nonexistent third browser-configuration gap. The temporary input was corrected and the active worker was steered to the plan’s actual matrix. No example application or browser configuration was added. **Next time:** copy the plan’s configuration and path counts literally into workflow outcomes, and distinguish Playwright’s term from an example application.

4. **Several probe failures came from request setup, not product behavior.** Direct API probes initially omitted `Origin` and correctly hit Kit’s CSRF guard. A browser regression used a port other than the app’s fixed build origin and correctly got 403. A Python follow-up GET omitted a `Secure` workspace cookie on localhost, making saved state appear lost. Each probe was repeated with the appropriate origin, port, or explicit cookie. **Next time:** use the built app’s configured origin and preserve the returned workspace cookie before treating such a result as a defect.

5. **One task started work already assigned to a later task.** The form-data worker began adding `OPTIONS`/`Allow` and action-name protocol checks while an explicit protocol task was queued. Those edits were removed after scope steering. **Next time:** leave discovered defects in the next task’s handoff unless they block the current capability.

6. **Validation was duplicated inside and after several tasks.** Workers ran their own focused suites and asked independent validators to rerun them, then Gimble QA ran the same focused or broad suites again. Some repeat runs were needed after actual assertion changes, but broad Stage 3 and Stage 4 reruns after each slice added little new information. **Next time:** use focused independent validation for each bounded slice and reserve the complete browser matrix for final qualification, except when a concrete cross-slice regression is found.

7. **A test command selected more work than intended.** During the final showcase adjustment, an extra `--` in the Playwright command selected all 247 tests instead of the two affected Actions features. The worker stopped it after 12 tests and restarted with the feature names passed directly to `pnpm test`. **Next time:** inspect the selected test count at the start of a focused run and stop immediately if it exceeds the requested slice.

8. **Visible-result framing was deferred until the final stage.** Earlier Stage 3 QA recorded screenshots that scrolled the saved-profile panel out of frame as a small gap. Final fault-probe QA found several passing frames that cropped the receipt or saved state, so the showcase had to be rearranged late. The affected 18 core scenarios then ran in both modes, followed by a 15-scenario upload/shared-route regression in both modes. The page change was necessary to satisfy the plan; deferring the known visual gap caused the extra late regression cycle. **Next time:** fix a screenshot that cannot show the assertion’s claimed state before accepting that slice’s visual proof.

## Required work that was not redundant

The plan explicitly calls for all 36 core journeys, applicable edge scenarios in development and built modes, individual screenshot inspection, and four break-and-restore probes. Those are product acceptance requirements. The probes have now made all four selected scenarios fail for their intended broken behavior and pass after exact restoration. The final full-suite qualification remains in progress.

## Final audit pending

- Add measured elapsed time and the final Gimble run result.
- Confirm whether the last showcase review found a concrete visual defect or only optional polish.
- Record final suite counts, screenshots inspected, and any unresolved claim.
- Separate implementation defects from tool, fixture, and workflow costs in the delivery summary.
