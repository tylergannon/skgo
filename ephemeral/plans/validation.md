# skgo validation, from the ground up

2026-09-25. Replaces `validation-tiers.md`.

## What skgo is, for the purpose of proof

skgo's claim is narrow: the bytes Go sends are the bytes kit's own server
would send, and the document Go renders is the one kit's renderer produces.
The browser runs kit's own client. Given the same bytes, that client behaves
the same way whether Node or Go produced them. So skgo's correctness is
almost entirely a property of bytes, and bytes are asserted in Go in
milliseconds.

What that leaves for anything slower is small and specific.

## What can break, and the cheapest thing that catches it

| Failure | Cheapest detector | Cost |
| --- | --- | --- |
| Go sends the wrong status, header, cookie, body, devalue encoding | Real-handler test with literal fixtures | ms |
| Go renders the wrong document (goja, kit's renderer, pool) | Real-handler test asserting markup | ms |
| Generator emits wrong TypeScript, stubs, wire codecs | Golden files of generated output | ms |
| Generated TypeScript does not type-check against the components | One `svelte-check` over the example | seconds, once |
| The `skgo` CLI reports advice at the wrong location | One built binary, many invocations | seconds, once |
| Dev renderer cannot load a module from Vite's graph, stale runtime after an edit | One `vp dev` process, many renders | seconds, once |
| Kit's client cannot use Go's bytes: hydration, enhanced submit, client nav, transport, batching, noscript | Browser | minutes |
| The page is visibly wrong: blank, error boundary, flash, unusable form | Eyes | minutes, and attention |

The first six rows are `just test`. The seventh is the browser suite. The
eighth is the look. Nothing else exists.

## `just test`: the loop

One command. Go tests in both modules. Budget: **15 seconds wall clock** on
a laptop, measured. Agents run it constantly and move on green.

It is 2m10s plus 45s today because the toolchain tests spawn the same
process per assertion. The `skgo` binary is built from source four times per
run, the example is copied into a sandbox four times, `svelte-check` runs
five times, one test does eight full `packages.Load` passes. The rule that
fixes it: **an expensive command runs the minimum number of times the
assertions require, and its captured output is asserted against many
times.** One binary per package via `TestMain`. One sandbox per package,
mutated in sequence and restored. One `svelte-check` per distinct source
state. Goldens for generated text. Go's package parallelism then makes wall
clock roughly the slowest single package, which is one `svelte-check` or one
link of a goja-sized binary.

The dev renderer joins `just test` the same way: one `TestMain` starts
`vp dev` once, every route renders through the dev handler, one source edit
proves the runtime refreshes. Today that coverage exists only as a second
full browser run.

Two hazards, both handled in Go, both in milliseconds:

- `example/` tests read the embedded build. A stale build tests yesterday's
  frontend and passes. One test compares the source hash to the built
  manifest's and fails with "run `just build`" when they differ.
- No `-short`. No fast recipe beside the real one. If the measurement misses
  15 seconds, the fix is more once-not-many, not a second command.

PR CI runs `just build`, `just vet`, `just test`. Its minutes are GitHub
setup plus the frontend build; the tests are the same 15 seconds.

## `just e2e`: the browser, when bytes are not enough

The browser catches one thing Go cannot: the agent read kit's source wrong,
so the bytes are consistent with the fixtures the agent wrote and still not
what kit's client needs. That happens when behaviour is added or changed,
and it does not happen when a refactor leaves the bytes alone. So the suite
runs on behaviour change and on `main`, never in the loop.

- Every scenario is classified by its **complete claim**: it stays only if
  some part of the claim needs kit's client to have acted. A scenario whose
  whole claim is HTTP moves to a Go assertion and is deleted. The transport
  scenario stays because its claim is that the page called a method on a
  class the browser rebuilt; the reserved-query scenario goes because its
  claim is a 400 and a message.
- Zero screenshots on success. Delete the `shot` fixture,
  `SKGO_E2E_SCREENSHOTS`, every `shot(...)` call, and the success upload.
  Playwright's failure trace and screenshot stay for diagnosis.
- Both modes, because dev is where kit's client meets an unbundled module
  graph and the seconds-wide pre-hydration window. On `main` they run on two
  runners at once and block nobody. The reduced suite makes the second mode
  cheap; if measurement says otherwise, dev gets a per-page tag then.
- `release.yml` keeps it as the gate before a tag, so a merge that breaks
  the client cannot publish. An agent never has to decide "was that a
  behaviour change" for CI's sake; the human decides it for the look.

## The look: opening the box

A green suite is the sticker. The look is the person opening the box, and
it is the one step no test replaces: the example app, used as a person
uses it, with the pictures read by something other than the agent that
made them.

`gimble run validate-product` already does this. Opus drives the built
example through assigned tasks it cannot read the source for. Gemini Flash
reads the captioned screenshots for blankness, error text, and whether each
caption's claim is visible. A triage model writes findings. A human reads
`findings.md` and the handful of screenshots it cites, not six hundred.

- Suite file at `ephemeral/validate/example.yaml`, output gitignored. Two
  workloads against `just serve`: the enhanced path from Home through
  Actions (save, reject an invalid email with fields retained, sign in and
  land, forbidden and unavailable errors) and the same forms with JavaScript
  disabled plus the noscript upload. Expected outcomes are written into the
  assignment as the literal receipts the page shows.
- Runs when behaviour changed substantially, before a release someone cares
  about, or when a `main` browser failure needs a human-shaped explanation.
  The human or the mission that changed the behaviour decides. Not CI.

## Doctrine

The current text is what produced 640 screenshots: an agent read "screenshots
of the running app at the moment each scenario asserts" and, correctly,
built exactly that, then overrode an issue that said otherwise. So the
mechanism is deleted and the text replaced, not softened.

- `AGENTS.md`, `CLAUDE.md`: "Acceptance is the Gherkin suite" becomes
  "Contracts are Go tests at the real handler; the Gherkin suite is for what
  only kit's client can prove." "A passing command is not evidence" keeps
  "skips are failures" and "never derive the expectation from the thing
  under test" and drops the per-assertion screenshot requirement in favour
  of the three parts above and the once-not-many rule.
- `Justfile` header: "Whether skgo works is `just test` green in seconds,
  `just e2e` green on main, and a person who opened the box."
- Nothing named `SKGO_E2E_SCREENSHOTS`, `shot`, or `ephemeral/screenshots/`
  survives. Nothing to turn back on.

## Sequence

1. Once-not-many in `internal/gen`, `cmd/skgo`, `example/typedrift_test.go`,
   and the dev-renderer `TestMain`. Measure `just test`. This number decides
   everything else.
2. Classify the 143 scenarios by complete claim; delete the Go-answerable
   ones and their orphaned steps; add the Go assertions they leave behind.
   Delete the screenshot machinery. Measure `just e2e` in both modes.
3. Write the gimble suite file and run the look once against the result,
   so the first release under the new rules has had its box opened.
4. Rewrite the doctrine last, quoting the measurements.

Every number above that is not in the failure table is a prediction until
step 1 or 2 reports it.
