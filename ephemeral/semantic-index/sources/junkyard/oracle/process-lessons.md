# Oracle sprints and reviews — what changes a future decision

**Purpose.** The short residue of the OR1/OR2 sprint contracts, adjudications, reviews and worklogs: decisions that still bind, traps that cost time, protocol gaps that were found and deferred, and what was built instead of the product. Process narrative is deliberately omitted.

## What was actually proven (and what was not)

- Proven at runtime, in Vite+ dev and built adapter-node production, against the unchanged kit client: one scalar-argument `query`, one `command` with a single `requested(...).refreshAll()` refresh returned in the same flight, one `query.live` stream with initial value + external update; duplicate export names across two modules; nondefault `base`/`appDir`; cold rename changes IDs. That is the whole proven surface. — `junkyard/oracle/README.md:L3-L8`, `junkyard/ephemeral/remote-oracle/sprint-01.md:L37-L41`, `sprint-02.md:L62-L66`, `validation/blackbox-02.md:L127-L132`.
- Explicitly **unproven**: rich devalue values / custom transports, no-argument calls, validation/error/redirect parity, stale-ID errors, live heartbeat/teardown/retry, SSR carriers, origin guard, `form`, `prerender`, `query.batch`, loads. — `junkyard/oracle/proof/run.mjs:L74-L82`, `junkyard/ephemeral/remote-oracle/inventory.md:L27-L48`, `overview.md:L74-L76`.
- The most useful remaining artifact is the **source-to-test inventory** mapping each protocol rule to kit source lines (ADDR/CODEC/QUERY/COMMAND/REFRESH/LIVE/HTTP/SSR groups). Use it as the checklist for skgo's protocol work. — `junkyard/ephemeral/remote-oracle/inventory.md:L27-L64`, `overview.md:L145-L159`.

## Protocol gaps found by review and deferred (still open for skgo)

1. **Refresh failure must not discard the command result.** Kit returns `type:"result"` with `_` preserved and per-key `{e:{status,message}}`; the Go first cut returned `type:"error"` after the mutation persisted. Also, Go let the client's `refreshes[]` list decide what to execute; kit's `requested(query, limit)` selects by query identity and bounds the count. — `junkyard/ephemeral/remote-oracle/reviews/opus-01.md:L68-L117`, `reviews/adjudication-01.md:L35-L42`.
2. **Cross-site Origin guard is missing in Go.** Kit (production) rejects mismatched `Origin` with `Cross-site remote requests are forbidden`; dev skips the check, so dev passes prove nothing. — `opus-01.md:L119-L142`, `adjudication-01.md:L44-L50`, `overview.md:L55-L56`.
3. **Empty payload = `undefined`**, not an error. — `opus-01.md:L163-L169`.
4. Module-keyed dispatch was only enforced for queries; command/live correlation by export name alone would still pass the suite. — `reviews/or2-plan-01.md:L44-L68`, `or2-plan-03.md:L88-L95`.

## Decisions that still bind a Go implementation

- Kit's generated IDs are the authority; extract them from the build, never from a handwritten table or a reimplemented hash used as oracle. — `proposal.md:L31`, `overview.md:L99-L101`.
- The runtime, not the domain handler, owns requested-query identity, deferred refresh, and live reconnect instructions; handlers are ordinary context-aware Go functions that never see envelopes. — `proposal.md:L32-L35`, `worklog/remote-oracle-go.md:L3`.
- Preserve JS value distinctions in the codec instead of flattening to `map[string]any`; compare typed graphs, and exact identity bytes for keys/payloads. — `overview.md:L93-L96,L79-L83`.
- Expected values come from a live Node run at the pin, never from Go and never from stored goldens alone; a reference failure is an upstream finding, not permission to weaken the test. — `overview.md:L133-L136`, `inventory.md:L59-L60`.
- Go owns the entire remote prefix including unknown IDs. — `proposal.md:L38-L39`.
- Fresh browser context per lane; Node reference lanes before Go; warm dev dependency discovery unscored. — `worklog/20260904-remote-oracle-design.md:L13`, `sprint-01.md:L46-L51`.

## Traps that cost real time

- Playwright's default SIGINT handler exited before the runner's cleanup, orphaning `vp dev`; fix was `chromium.launch({handleSIGINT:false, handleSIGTERM:false})`. — `worklog/20260904-remote-oracle-or1.md:L8-L9`, `reviews/opus-02.md:L17-L25`.
- Playwright locator timeout escaping the outer deadline during cold Vite optimisation. — `worklog/202609041200-remote-oracle-or2.md:L8`.
- `kill EPERM` on a detached Go process group. — `…or2.md:L7`.
- Global vs local Vite+ instance identity. — `…or1.md:L4`.
- Symlinked fixture sources change kit IDs. — `reviews/or2-plan-03.md:L66-L75`.
- Dev registry captured before the browser imported every remote module → missing IDs. — `reviews/or2-plan-01.md:L91-L115`.
- TypeScript 7 cannot build kit (`ts.sys`); TS 6.0.3 is the deliberate pin. — `junkyard/oracle/source-lock.json:L98-L100`.
- Evidence produced by an older runner than the reviewed commit was flagged; only matters if you keep the hash-binding evidence model. — `reviews/opus-01.md:L144-L161`.

## What was wasted (do not repeat)

- Three plan-review rounds, two adversarial code-review rounds, two black-box validations, adjudication records, identity JSONs, signed artifact links with expiry dates, and hash-bound artifact indexes — for a fixture that proves three functions with scalar values. — `junkyard/ephemeral/remote-oracle/proof/README.md:L1-L167`, `proof/published-artifacts.md`, `reviews/*.md`, `junkyard/ephemeral/reviews/*.md`.
- Score-counting ceremony (18/6/2, 27/2), "harness checks" H01/H02, and ~600 lines of runner devoted to counts, identity hashing and process forensics rather than protocol coverage. — `junkyard/oracle/proof/run.mjs:L237-L266,L940-L974,L1092-L1123`.
- The verdict of all that review was "only nitpicks remain" while the two material protocol gaps (refresh-failure envelope, origin guard) stayed open — review effort did not translate into protocol coverage. — `reviews/adjudication-02.md:L21-L25`, `junkyard/ephemeral/reviews/or2-fable-round-02.md:L44-L58`.

## Reusable verdict
Keep: the inventory as a checklist, the pin/bootstrap/byte-verify approach, the ID-harvesting plugin, the switchboard recording technique, the four deferred protocol gaps as the first items to get right. Drop: scoring, adjudication, identity/evidence binding, sprint contracts.

## Recipes
- Starting point for the next protocol slice: `junkyard/ephemeral/remote-oracle/inventory.md:L27-L48` (pick a row, read its kit lines, write the Node observation first).
- Reproduction shapes for the two deferred defects: `junkyard/ephemeral/remote-oracle/reviews/opus-01.md:L77-L101` and `L122-L135`.
