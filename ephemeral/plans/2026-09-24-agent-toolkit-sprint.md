# skgo agent toolkit sprint

## Outcome

A developer or coding agent can discover how to build a skgo feature, inspect
the Go/Svelte application, check it, and apply routine repairs through a small
CLI and MCP surface. The everyday gesture is `skgo check --fix`: tools remember
the invocation order, project roots, generated ownership, and configuration.
The result explains remaining problems in authored source and does not claim
that static checks prove application behavior.

This is the user-requested sprint plan and execution handoff. Work in
`/Users/tyler/.codex/worktrees/4daa/skgo`, branch `codex/agent-toolkit`.
Read `AGENTS.md` and the agent-protocol skill. Kit is the specification; consult
`/Users/tyler/src/skgo/ephemeral/inspiration/reference/kit@3.0.0-next.27` directly
for mirrored behavior, and the adjacent pinned Svelte sources. Research below
orients decisions; builders independently map upstream rules they rely on.

Research: `ephemeral/research/2026-09-24-svelte-agent-tooling.md`.
Gimble input: `ephemeral/plans/2026-09-24-agent-toolkit-outcomes.json`.

## Product constraints

- CLI: `skgo check`, `skgo check --fix`, `skgo fmt`, and `skgo mcp`.
  Check supports human and structured output, watch mode, explicit fix paths,
  and an explicit build mode. Type checking retains project/dependency scope
  when repair scope is restricted. MCP invokes the same capabilities.
- Compose upstream tools and existing generator/adapter checks. Implement
  skgo-owned orchestration and analysis in Go; do not create another compiler,
  JavaScript engine, generalized tool framework, or shell/JavaScript harness.
- Classic official `svelte-check` is the default. Keep TypeScript 6 for Kit
  sync/build. The research found real diagnostic gaps in current alternatives;
  speed does not justify losing checks or source locations.
- ESLint plus `eslint-plugin-svelte` supplies component lint and supported
  automatic fixes. Use its real Kit 3 configuration path. Do not substitute
  script-only linting or run duplicate rule sets just to add more tools.
- Qualify the installed Vite+/Oxfmt Svelte support against official Prettier
  plus `prettier-plugin-svelte`. Prefer the integrated formatter when it
  preserves supported syntax, semantics, project policy, and idempotence;
  otherwise use Prettier. Choose once during setup and report that choice.
  Exactly one formatter owns each file. No broad native-tool bake-off.
- Go formatting/import organization uses gofmt/goimports. Include Go
  type checking, vet, and Staticcheck correctness checks, with project-pinned
  tools/configuration. Respect existing configured equivalent tooling.
- Checks do not rewrite authored or committed generated files. Fix mode may
  apply configured mechanical fixes and regenerate bindings; it does not
  suppress diagnostics, weaken rules, upgrade dependencies, or guess business
  logic. Diagnose stale bindings before repairing them. Missing dependencies,
  skipped required work, or incomplete checker output cannot become success.
- Preserve user configuration; installation/setup is explicit. New projects
  get a coherent default through upstream Svelte/Vite installer ownership.
  Avoid a second competing configuration format and second config mutator.
- The documentation server and MCP run locally over stdio for project tools.
  Upstream documentation/tools retain attribution and version information;
  don't pretend current web documentation matches a prerelease automatically.
- No hosted service, production request tracing, IDE extension, new browser
  dashboard, compiler replacement, release/publish/deploy, or unrelated cleanup.
  Runtime observation remains future work. Do not edit `docs/`; README and the
  existing skill may explain the delivered user interface. Shipped documentation
  resources can live with the tooling package without changing the docs website.
- All phases share this worktree and execute sequentially. Ownership is the
  capability named by the current outcome; preserve earlier outcomes and any
  unrelated work. The Gimble workflow does not commit/push/merge or alter this
  plan/outcomes input; the supervising caller owns checkpoint commits.

## 1. Whole-project diagnostics

Ownership: non-mutating checking and diagnostic reporting.

The developer receives formatting, lint, Svelte/TypeScript, Go, and skgo source
diagnostics in one invocation. Include declaration placement/signatures,
duplicate remotes/loads/methods, wire type/field-name constraints, permitted
Deferred and upload usage, and adapter/toolchain compatibility by reusing the
rules already enforced by generation. Surface distinct source codes, severity,
authored ranges, related locations, available fixes, documentation, and explicit
tool completion/failure. Do not fabricate a location when one is unavailable.

```gherkin
Scenario: Find errors across the application
  Given a project with a wrong prop type through a #lib component import,
    a template lint error, an accessibility warning, and a Go type error
  When I check the project
  Then each independently planted issue is reported at its authored source
    and all available checkers complete without changing source files

Scenario: A checker failure cannot report success
  Given a required checker dependency is missing, or a checker stops before completion
  When I check the project
  Then I receive a failed or incomplete result naming the affected check
    even when its underlying process exits zero

Scenario: Watch the application after an edit
  Given checking is watching an application
  When a valid component import is changed to an invalid use and then repaired
  Then diagnostics appear and clear for the affected project state
    and cancelling the watch stops its owned processes
```

## 2. Repairs, formatting, and setup

Ownership: deterministic edits and an immediately usable project configuration.

`skgo check --fix` applies supported lint fixes and Go import/format cleanup,
regenerates affected bindings, formats the result, and rechecks. `skgo fmt`
formats only. Honor explicit repair paths, ignore rules and authored/generated
ownership. New and existing projects can install/configure the selected tools
through one documented setup gesture and use the same commands thereafter.

```gherkin
Scenario: One gesture repairs routine mistakes
  Given selected Go and Svelte files have formatting problems and fixable lint,
    their generated bindings are stale, and a separate type error needs judgment
  When I run check with fixes for those files
  Then mechanical issues and stale bindings are repaired
    and the remaining type error is reported without suppression
    and unrelated authored files are unchanged

Scenario: Formatting preserves real Svelte behavior
  Given components using runes, typed snippets, render tags, async expressions,
    attachments, whitespace-sensitive markup, and configured styles
  When I format them twice
  Then the second pass makes no changes and the official compiler accepts them
    and representative rendered content and interactions remain correct

Scenario: Defaults require no tool choreography
  Given a newly created app or an existing app with established lint/format rules
  When setup completes and I invoke the skgo check and fix commands
  Then the configured tools cover Svelte components and Go source
    and established user rules are preserved and the selected tools are visible
```

## 3. Integration and build diagnostics

Ownership: generated ownership, Go/Kit agreement, and opt-in build checking.

Detect missing/stale/edited/orphaned generated artifacts, handwritten TS server
implementations, invalid Kit route exports/configuration, and the supported
prerender/load constraints. Build checking exercises real Kit output and skgo's
adapter/runtime: remote IDs, loads and route methods, private import violations,
manifest/adapter identity, referenced assets, and SSR bundle/node consistency.
Respect tree-shaking and Kit's actual semantics. Transport declaration checks
cannot claim arbitrary encoder/decoder equivalence without runtime testing.

```gherkin
Scenario: Explain a broken generated boundary
  Given a Go result field changed while its generated bindings remain stale
  When I check the app
  Then the diagnostic connects the authored declaration and generated binding
    and explains regeneration rather than asking me to hand-edit the stub

Scenario: Diagnose a build that Go cannot serve
  Given a frontend with an unregistered remote or incompatible adapter output
  When I request build checks
  Then the relevant boundary failure is reported with its upstream evidence

Scenario: Preserve Go ownership
  Given handwritten TypeScript server behavior in a skgo application
  When I check it
  Then I receive an actionable ownership diagnostic
    while legitimate generated throwing stubs remain accepted
```

## 4. Documentation server

Ownership: useful version-aware documentation and examples through CLI/MCP-ready APIs.

Supply a section catalog with use cases, search, batched retrieval, API signature
lookup, constraints and source links. Ship versioned skgo documentation/examples
for offline use, with paired Go/Svelte recipes covering remotes, forms/uploads,
live/batch, loads/streaming, transport, errors, Go request hooks and deployment.
Retain upstream Svelte/Kit documentation discovery. Identify installed versions,
label current/unmatched upstream material honestly, and provide relevant known
migration guidance rather than inventing compatibility. Cache fetched material.

```gherkin
Scenario: Learn how to implement a form
  Given an agent working in a skgo project
  When it searches for field validation and requests the matching documentation
  Then it receives a usable paired Go/Svelte example, API constraints,
    and attributable version/source information

Scenario: Documentation remains useful offline
  Given network access is unavailable
  When I request installed skgo documentation and an uncached upstream page
  Then the shipped documentation is returned
    and the unavailable upstream page is reported as unavailable

Scenario: Unknown versions stay explicit
  Given a project whose Kit version has no matching documentation
  When I request version-specific guidance
  Then the mismatch is explicit rather than presented as verified compatibility
```

## 5. Project understanding and MCP

Ownership: local agent-facing tools and their CLI parity.

`skgo mcp` exposes documentation catalog/search/retrieval as tools and resources;
project and route inspection; tracing a Go declaration through generated types
to statically resolvable frontend consumers; check/fix; explicit regeneration;
and upstream Svelte component analysis/playground capability. Retain upstream
features by composition rather than reimplementing their behavior. Expose route
parameters, layout/load ancestry, declared SSR/CSR/prerender options and HTTP
methods using authoritative project data. State unresolved/dynamic relationships
as such. Bound response size by query rather than dumping the whole repository.

One setup gesture and a short updated skgo skill make the capabilities discoverable.
Use `check` with a fix option for ordinary repair; don't require an agent to
remember a separate tool for every underlying linter or formatter.

```gherkin
Scenario: Follow a value across languages
  Given a page consumes a generated remote backed by a Go function
  When an agent inspects its route and traces that binding
  Then it sees the relevant authored Go and Svelte locations, generated bridge,
    declared input/output types and known consumers

Scenario: CLI and MCP agree
  Given the same broken project
  When I request checking through the CLI and a real stdio MCP client
  Then both report equivalent diagnostics and completion state
    and MCP fix mode produces the same permitted repairs

Scenario: Upstream tools remain useful
  Given Svelte code needing component analysis or a standalone playground
  When an agent requests those upstream capabilities through the configured toolkit
  Then the upstream results are returned with their original meaning
    and suggestions are not misrepresented as automatically applied edits
```

## 6. Fresh-user acceptance

Ownership: independent end-to-end qualification and concise user guidance.

Prove the delivered experience from a fresh skgo app and an existing app. A new
agent given only the shipped short skill must discover form documentation, find
a planted Go error, a Svelte error and stale bindings, apply routine fixes,
repair the residual authored error, and demonstrate the intended app behavior.
Measure cold, warm and one-file-edit checks/formatting on the same component
corpus; report latency alongside actual diagnostic/format coverage. Speed claims
require these observations, not vendor JS throughput numbers.

Acceptance is the load-bearing Gherkin behavior above, implemented in ordinary
Go tests where appropriate and the existing browser suite for application behavior.
The independent validator checks that removing the relevant capability makes its
scenario fail, then executes the scenarios. Retain inspectable command/protocol
outputs for CLI/MCP claims and actual screenshots of the running app for browser
claims. Open every screenshot; no blank/error page counts as successful evidence.
Do not build a new report UI, proof manifest, gate runner, or status ledger.

```gherkin
Scenario: An agent can repair and demonstrate the application
  Given a fresh app with a known greeting/form behavior and independently planted errors
  When an agent uses the shipped toolkit instructions to diagnose and repair it
  Then the known user-visible result works in the running application
    and the reviewed screenshot shows that exact result
    and the repair does not move server behavior into JavaScript
```

## Approved initial eight analyzers — September 24

This replaces the preceding broad Go-linter expansion. The user clarified:
"The idea is let's try to think up the advice that we would give to developers
using this thing, and then set up Go analyzers that can detect the anti-patterns
and barf on them." Continue the current run and keep its six outcomes. The
priority is useful skgo-specific developer guidance enforced by analysis, not a
large generic lint preset. goimports, vet and Staticcheck remain the baseline;
additional generic rule families are not mandatory sprint acceptance.

The user approved the eight rules below for implementation. Outcomes 1 and 3
own their analysis and integration; outcome 4 owns the matching developer advice
and outcome 5 exposes them through the shared MCP check capability. Implement
skgo-owned Go rules as ordinary golang.org/x/tools/go/analysis analyzers, sharing
existing generator checks and type information where appropriate. Reuse an
upstream analyzer when it already provides the desired advice. Do not create
an analyzer framework or duplicate enforcement logic just for this interface.

Each rule encodes developer advice, identifies a concrete consequence and names
the repair. Independently map the applicable skgo API and pinned Kit behavior
before coding; do not weaken the contract to match a convenient detector.

| Rule | Advice and detectable misuse | Valid counterpart required |
| --- | --- | --- |
| 1. Queries cannot change cookies | Detect SetCookie/DeleteCookie on the request event in a known read-only query context. Explain that the cookie change belongs in a command or form. Include the query variants covered by Kit's actual restriction. | The same cookie operation in a command/form is accepted; legitimate cookie reads are accepted. |
| 2. Page inputs belong in query arguments | Detect load-only Event.Param/URL/SearchParam/RouteID use inside a known query. Explain that page-dependent inputs must be arguments so the query cache identity includes them. | Passing the value as a typed query argument works; request header/authentication-cookie reads remain valid. |
| 3. Preserve the request context | Detect EventFrom fed a demonstrably fresh Background/TODO context inside a declared handler. Explain that the fresh context lost the skgo request event. | The supplied handler context and contexts correctly derived from it are accepted; unrelated intentional background work is not prohibited. |
| 4. Handle skgo operation failures | Detect discarded errors from skgo operations such as cookie writes, refreshes and live-query yield callbacks. Explain the specific lost operation or disconnect signal. Reuse upstream unchecked-error analysis where it fits. | Propagation or real handling is accepted; do not invent recovery logic or insert blank assignments as fixes. |
| 5. Refresh the right kind of function | Detect statically resolved refresh/reconnect targets whose known registration kind contradicts the typed helper's contract. Explain ordinary-query refresh versus live-query reconnect. | Correctly matched targets work; unknown dynamic registrations are not asserted invalid. |
| 6. Validation errors must name real fields | Detect literal Invalidf/Issue.Field paths inconsistent with the form's declared input shape. Explain a misspelled field with its matching declared field when unambiguous. | Whole-form errors, valid nested paths, valid array paths and nonliteral unresolved paths are handled correctly. |
| 7. Only supported values cross the wire | Surface existing generator rules for unsupported wire types, duplicate serialized names, file results and Deferred outside loads. Identify the authored offending field and a supported representation. | Supported value/nullable/form-upload/load-deferred patterns pass under the existing generator contract. |
| 8. Route parameters must exist | Detect a literal parameter name inconsistent with a statically known applicable route. For example reading id from a route that declares customerID. | Declared parameters and valid rest/optional parameters pass; shared layouts/helpers are judged only when their complete applicable route context is known. |

All eight belong to the initial implementation; the analyzer must report the
limits of its static knowledge instead of guessing intent. Shared helpers may
run under multiple handler kinds: mere reachability must not make valid uses
invalid. Hard failures require demonstrated contract violations. Uncertain
advice must not fail a build. Do not expand this into a general whole-program
analyzer to erase those limits.

For each rule, ordinary Go analysis tests must plant an independent misuse and
its valid counterparts and verify source locations and explanatory diagnostics.
Exercise the real skgo check command as well; a directly called analyzer alone
is not proof that the user-facing toolkit runs it. The independent validator
must check that removing each rule makes its accepted bad-case scenario fail.

A useful diagnostic says what goes wrong and how to fix it, for example:
"This query cannot set cookies. Move the cookie change into a command or form."
Its code links to the same versioned advice served by the documentation/MCP
outcome. CLI, MCP and editor-compatible Go analysis should not teach conflicting
rules. Automatic fixes remain limited to changes with an unambiguous meaning.

```gherkin
Scenario: Advice catches a real skgo misuse
  Given a registered query attempts to change a cookie on its request event
  When I check the project
  Then the diagnostic identifies the query and cookie call
    and explains the command or form alternative
    and the same cookie operation in a valid command is accepted

Scenario: The checker explains lost request context
  Given a declared handler obtains its skgo event from a fresh background context
  When I check the project
  Then the diagnostic explains that the new context has lost the request event
    and obtaining the event from the supplied handler context is accepted

Scenario: Developer advice and diagnostics agree
  Given a skgo-specific diagnostic
  When an agent requests its explanation through the documentation tool
  Then it receives the corresponding advice, reason, and valid repair example
    for the installed version
```

Explicitly deferred: load reads bypassing dependency tracking; requested
refreshes left without a decision; and request state escaping into shared globals.
Do not implement those three in this initial set. The user said to start here
and grow later.

Keep the original cross-language build/registration checks and runtime evidence
boundaries. Broader security presets, extra dependency-scanning modes, generic
resource/style rules, import-policy frameworks, and whole-program intent analysis
are not newly required by this clarification. The ambition is to encode valuable
framework expertise and catch real broken connections, without pedantic noise.

Starting evidence, not a substitute for independent mapping:
- skills/skgo/SKILL.md (developer guidance)
- event.go and loadevent.go (request event and query/load constraints)
- refresh.go, requested.go, and remote_live.go (typed refresh/reconnect contracts)
- internal/gen (existing declaration and wire-shape enforcement)
- /Users/tyler/src/skgo/ephemeral/inspiration/reference/kit@3.0.0-next.27/src/runtime/app/server/remote/shared.js
- /Users/tyler/src/skgo/ephemeral/inspiration/reference/kit@3.0.0-next.27/src/runtime/app/server/remote/requested.js
- https://pkg.go.dev/golang.org/x/tools/go/analysis
