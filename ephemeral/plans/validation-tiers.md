# skgo validation: one test command, one browser run, one occasional look

Proposal, 2026-09-25. Revised once after an adversarial review by Codex
(gpt-6-sol, high); the review's surviving objections are folded in below and
the ones that did not survive are noted at the end.

## What is wrong today

skgo is a framework. Its correctness claim is "Go answered the endpoints kit's
client calls, and the document Go rendered is the one kit would have
rendered." The current validation treats it like a consumer product and
demands a photograph of every claim.

Measured on `codex/issue-183-lean-form-validation`:

| Item | Value |
| --- | --- |
| Gherkin scenarios | 143 (main: 166) |
| Modes each scenario runs in | 2 (prod, dev) |
| Screenshots left by one green release run | 640 (320 per mode) |
| Release-gate wall clock, last run on main | 10m51s |
| `go test ./...` in the root module | 2m10s |
| `go test ./...` in `example/` | 45s |
| Slowest real-handler test in `example/` | 0.06s |

Three mechanisms produce the 640 images, and all three are policy, not tests:

1. `example/e2e/steps/fixtures.ts` declares the `shot` fixture `auto: true`
   and forces a `final` capture for any scenario that took none. A scenario
   cannot opt out.
2. `qualification.yml` sets `SKGO_E2E_SCREENSHOTS: all` and uploads
   `ephemeral/screenshots/**` on success with `if-no-files-found: error`, so a
   green run that produced no images fails.
3. `AGENTS.md` and `CLAUDE.md` say validation "has to produce ... screenshots
   of the running app at the moment each scenario asserts." Issue #183 said
   "no screenshots on success"; the codex agent's worklog records that it
   overrode the issue in-session because of that doctrine. The issue and the
   worklog were both agent-written. The owner's decision is the one in this
   session: the image dossier goes.

## Principle

A screenshot proves the product renders. It cannot prove an API contract, and
skgo's contracts are HTTP: status, headers, body shape, cookies, the rendered
document's markup. Those are asserted exactly, in Go, against the real handler,
in milliseconds. A browser adds one thing: kit's own client acting on Go's
answers. That is what the browser tier is for.

Classification rule for a browser scenario, applied **per scenario by its
complete claim**, never per feature file: if the whole claim could be made
with `page.request` alone, it is a Go test. If any part of the claim needs
kit's client to have acted (hydration, enhanced submit, client navigation,
`invalidate`, a streamed promise settling in the DOM, a transported class
rebuilt with its methods, a batch coalesced by the client, a noscript form post
rendering a usable page), it stays in the browser. `reserved-query.feature` is
a Go test. `transport.feature`'s first scenario is not, even though its wire
bytes are already asserted in `wire_test.go`: its claim is that the page
called a method on a value the browser reconstructed.

## `just test`: the gate, locally and on PRs

There is one Go gate, and it is the same command on a laptop and in PR CI.
It is slow today for one reason, and that reason is fixable without adding a
second way to run tests:

| Bucket | Examples | Time |
| --- | --- | --- |
| Real-handler tests (`newProdHandler`) | action protocol, CSRF, OPTIONS, remote calls, SSR render, transport | 20–60 ms each |
| Toolchain: `go build` a fixture app | `TestNamedDeferredPayloadSerializedNames` 33s, `TestFormClientGenerationRecoversAndTracksContract` 22s, ownership, arity, stalegen | ~100s of `internal/gen`'s 129s |
| Toolchain: build the `skgo` binary, run `check` and `mcp` | `TestRealCheckReportsWireAdviceAtAuthoredLocations` 21s, `cmd/skgo` interface tests | ~30s |
| Toolchain: `go generate` and `svelte-check` via mise | `TestGeneratedActionTypesRejectWrongUses` 10s, `TestChangingAGoTypeBreaksTheComponentThatUsesIt` 9s, `TestNothingGeneratedWasWrittenByHand` 6s | ~25s of `example/`'s 45s |
| Streaming tests with real settle delays | three `stream` tests at 3.6s | ~11s |

The toolchain tests pay for the same expensive process once per assertion
instead of once per package. Counted on the branch:

| Expensive step | Times per full run | Where |
| --- | --- | --- |
| Build the `skgo` binary from source | 4 | `internal/gen/check_test.go`, `cmd/skgo/interface_test.go`, `query_advice_test.go`, `refresh_route_advice_test.go` |
| `go mod tidy` on a throwaway fixture module | 3 | the three `cmd/skgo` tests |
| `go generate` a fixture app | 8 call sites | `internal/gen` via `runGoGenerate` |
| `go build ./...` a fixture app that links skgo and goja | 6 call sites | `internal/gen` via `runGoBuild` |
| Copy the whole example into a sandbox | 4 | `example/typedrift_test.go` via `sandbox` |
| `svelte-check` over the example | 5 | `example/typedrift_test.go` |
| In-process `Run`/`Check`, each a full `packages.Load` of the example | 8 in one test | `TestNamedDeferredPayloadSerializedNames`, 33s on its own |

The rule: **an expensive command runs the minimum number of times the
assertions genuinely require, and its output is captured once and asserted
against many times.**

- One `skgo` binary per package, built in `TestMain` or behind a
  `sync.Once`, shared by every test in that package.
- One sandbox copy of the example per package; mutation tests edit it in
  sequence and restore between cases.
- One `svelte-check` per distinct source state: a baseline run plus one run
  per planted change, every assertion reading captured output.
- In-process `Run`/`Check` tests plant every valid representation at once
  where the assertions do not interfere, so one load answers several
  questions, with a separate load only for the cases that must fail.
- Golden outputs where the assertion is about generated text: generate once,
  compare many files against fixtures the test supplied.

Target, to be measured: the full `just test` in both modules under a minute
on a laptop. If it lands there, there is no fast tier and no `-short`, and
the proposal ends here for Go. Only if it does not is a `-short` split worth
its cost, and that decision waits for the measurement.

Prerequisite that stays: `example/` tests read the embedded build, so
`just build` must have run since the last frontend change. Build cost, not
test cost.

Every scenario the classification rule moves out of the browser suite gets
one Go assertion at the real handler if none exists. One per distinct
contract, not a matrix.

### PR CI

`ci.yml` already runs the commit-subject check, `just build`, `just vet` and
`just test`. Nothing changes there except that `just test` gets fast. The
minutes it takes today are GitHub overhead (`setup-go`, mise, `pnpm install`,
`playwright` is not involved) plus the redundant subprocesses above; the
overhead is a couple of minutes and the rest disappears with them. No
browser on PRs. A merge to `main` that breaks kit's client cannot ship,
because `release.yml` gates tagging on the browser run, and the fix is one
more PR.

## The browser run: on `main`, no success images

`qualification.yml` and the suite:

- Delete the `shot` fixture, `SKGO_E2E_SCREENSHOTS`, the `curated` policy,
  and every `shot(...)` call in steps (109 sites). Steps that exist only to
  take a picture go with them. Playwright's `screenshot: 'only-on-failure'`
  and `trace: 'retain-on-failure'`, already configured, are the diagnostics.
- Upload `example/e2e/test-results/**` only when the job fails
  (`if: failure()`, `if-no-files-found: ignore`). A green run leaves no
  artifact.
- Reclassify scenarios by the rule above and delete the Go-answerable ones
  and their orphaned steps. The count is a result, not a target.
- **Both modes stay, on the reduced suite, in parallel on separate runners.**
  Codex's objection holds: Go renders the dev document by pulling transformed
  modules out of the Vite module graph, and a module that fails to transform,
  a stale pooled runtime after an edit, or the seconds-wide pre-hydration
  window can break any page in dev and none in prod. A tagged `@dev` subset
  is a later cut, made after the reduced suite's dev time is measured and
  with a per-page coverage argument, not before.
- What is lost, stated plainly: a green run whose page is visually blank but
  whose assertions pass will not be seen at this tier. The worklog records
  that screenshots caught exactly that once, on a noscript `ssr = false`
  shell. The defence here is the assertion discipline issue #183 already
  requires: every retained scenario asserts positive, literal, visible
  content, and a `Then` whose only content is "X is absent" is rejected in
  review. The visual catch moves to the occasional look.
- Prediction, to be measured: wall clock dominated by `pnpm install` and
  `playwright install --with-deps`, not by scenarios.

## The occasional look: `gimble run validate-product`

This is exploratory product testing, not regression validation. Codex is
right that `gimble run validate-product` is a focus-group tool; that is what
is wanted here. Its job is to answer one question a suite cannot: "is the
running example fudging reality," by having a model that never reads the
source use the app as a person would, and a different, cheap model read the
pictures.

- Roles as installed: Opus 5.5 operates, **Gemini Flash** reviews the
  captioned screenshots for blankness, error text, and whether each caption's
  claim is visible, GPT-6 Astra triages and files issues. No frontier model
  opens the images.
- Suite file, `ephemeral/validate/example.yaml`, gitignored output:
  - `product`: the example app; `issue_repo: tylergannon/skgo`.
  - `guides`: `README.md`, `skills/skgo/SKILL.md`, and the example's own
    route list. Nothing from `internal/` or the Go sources.
  - Two workloads, each with an isolated workdir, `start: just serve` on its
    own port, `ready: curl -sf $ORIGIN/`:
    1. **Enhanced path.** From Home, use Actions to save a profile, submit
       an invalid email and see the rejection with fields retained, sign in
       and land on the signed-in page, trigger the forbidden and unavailable
       errors. Expected outcomes are written into the assignment as the
       literal receipts the page shows ("Saved Grace Hopper", "Enter a valid
       email address", "Signed in as ada", "You cannot edit this profile").
    2. **Native path, JavaScript disabled.** The same forms, plus the
       noscript upload, asserting the returned document is usable and the
       receipts match.
  - Assignments are user tasks with independent expected outcomes, not
    feature checklists.
- Run it when a change touches rendering, forms, hydration, or the adapter;
  before tagging a release; or when a release-gate failure needs a
  human-shaped look. Never on every merge and never in PR CI.
- Prerequisites to confirm before the first run: `playwright-cli` with its
  browser, ffmpeg with libx264, authenticated `gh`, a configured
  `gimble upload-artifact` destination.

## Doctrine changes

Codex's last objection is the one that matters most: the doctrine is what a
future agent will re-derive the screenshot dossier from. So the mechanism is
deleted, not made optional, and the text is rewritten, not softened.

- `AGENTS.md` and `CLAUDE.md`, "A passing command is not evidence": replace
  the per-assertion screenshot requirement with the three parts above and
  the per-scenario classification rule. Keep "skips are failures" and "never
  derive the expectation from the thing under test" verbatim; both are
  correct and neither is about images.
- `Justfile` header: the codex branch's "captures its meaningful outcomes for
  inspection" and main's "a screenshot of the running app, looked at" both go.
  Replacement: "Whether skgo works is a green full test run, a green browser
  run on main with positive visible assertions, and an occasional product
  run whose screenshots a cheap model read first."
- `ephemeral/plans/form-actions.md`: keep the issue #183 supersession note,
  drop the screenshot sentence.
- No `SKGO_E2E_SCREENSHOTS`, no `shot`, no `ephemeral/screenshots/`. Nothing
  a future agent can turn back to `all`.

## What this does not claim

- `just test` does not prove hydration or dev rendering. Only the browser run does.
- Post-change timings above are predictions until measured on the branch.
- The occasional look is not a regression gate and produces findings, not a verdict.

## Codex objections that did not survive

- "The draft reverses the owner's recorded screenshot decision." The record
  it cites is the codex agent's own worklog overriding an agent-written issue.
  The owner's decision is this session's instruction to remove the dossier.
  The substantive residue of the objection, the blank-page catch, is kept
  above under Tier 2.
- "Build tags or a separate package would be a better split than `-short`."
  Moot: after the owner's review the split is gone entirely. The full run is
  made fast instead, and a `-short` tier is deferred until a measurement
  shows it is needed.
