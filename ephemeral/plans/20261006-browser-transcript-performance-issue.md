## Promise: materially faster CI

Investigate whether browser activity recorded against a pure-TypeScript
SvelteKit reference application can supply independent protocol expectations
that ordinary Go tests replay against SKGo without launching a browser.

This belongs to a future performance promise. It is separate from #247, whose
purpose is correct, dependable testing without unrelated example consumers
blocking narrow contracts. A modest reduction in browser scenario count is not
the performance outcome being sought. Do not add this investigation to #247 or
make that correctness work depend on it.

## Proposed direction

When the example application's relevant behavior changes, or its SvelteKit
dependency pins change, run browser journeys against the equivalent
pure-TypeScript Kit server and capture the resulting HTTP conversation. On
ordinary Go changes, replay those requests against SKGo and compare its
responses with the reference transcript, without Chromium or Kit's client
running during replay.

Use Kit at the same pin as the example as the reference. Expectations must
come from Kit, not from recording SKGo's current responses. The TypeScript
server would be a test reference, not a new supported SKGo execution mode.

## Questions the investigation must settle

- How can we maintain equivalent reference behavior without creating a second
  large application whose drift costs more than replay saves? The current
  generated TypeScript server stubs throw; they are not a usable reference
  implementation of the Go handlers.
- What must a transcript retain for stateful journeys: methods, URLs, bodies,
  relevant headers, cookies, redirects, and streamed response semantics? Can
  deterministic fixture data keep comparisons exact, and which differences
  genuinely require controlled substitution (such as origins or session IDs)?
- How do replay requests consume values from prior Go responses without hiding
  mismatches? Which concurrency, cancellation or stream-order claims cannot be
  represented by a simple recorded request sequence?
- How will recordings identify their example revision, dependency pins and
  build mode, and become invalid when the reference changes? Regeneration must
  cover example changes as well as Kit upgrades; stale recordings must not
  silently qualify changed behavior.
- Which browser claims remain indispensable? HTTP replay can compare server
  behavior but does not itself execute hydration, rendering, client caches,
  CSP enforcement, user interaction or HMR. Retain a browser layer that proves
  the behavior replay cannot, with coverage justified before removing cases.

## First useful result

Try a small representative set of existing journeys with real Kit and real
SKGo, including a stateful interaction and a nontrivial response. Show that
browserless replay agrees with the reference and fails on deliberate relevant
Go regressions. Identify what it cannot prove.

Measure end-to-end CI impact against the current workflows, including reference
maintenance, recording refreshes, builds, replay and retained browser runs.
Separate ordinary Go-change cost from example-change and dependency-upgrade
cost. Browser pruning alone does not accelerate the current PR workflow, which
does not run E2E; any claimed CI benefit must identify the affected path and
its actual bottleneck.

Use the result to decide whether this can deliver a substantial reduction in
CI wall time while preserving confidence. Do not commit to replacing the
browser suite, a target speedup, or a reusable capture/replay framework before
that experiment establishes feasibility and value.

Related: #136 (browser coverage placement), #132 (release qualification scope),
#247 (fixture isolation and compatible setup sharing).
