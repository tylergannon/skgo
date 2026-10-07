# Adversarial review: fix plan for #265 — round 03

Written 2026-10-06, local time. Read-only review; nothing outside this file
was changed. Earlier rounds: `watcher-265-plan-round-01.md`,
`watcher-265-plan-round-02.md`.

## Target

The third revision of `ephemeral/plans/watcher-265-fix.md` (untracked,
worktree `watcher-265` at `eec9fd4`). Still no implementation: `git diff
HEAD` outside `ephemeral/` is empty. "Current implementation and proof"
therefore means the code the plan builds on and the research logs.

Authoritative sources are unchanged: issue #265, `ephemeral/research/
watcher-265/`, the installed `@sveltejs/kit` 3.0.0 and the chokidar 3.6.0
bundled in `@voidzero-dev/vite-plus-core@1.0.0`, `CLAUDE.md`, and the user's
stated priority of stability and build success over modest added latency.
No scope narrowing was requested; none was ignored.

## Evidence inspected

Everything from rounds 01 and 02, plus:

- Bundled chokidar `createFsWatchInstance` (`node.js:15531-15542`): every
  `fs.watch` callback calls `emitRaw` — the public `raw` event — before the
  5 ms watch throttle (`:15709`) and the 50 ms change throttle (`:16685`).
  So the "witness" step 4 needs exists without touching `_throttle`.
- `async _emit` (`:16637`) and `emitWithAll` (`:16623`): a listener throw
  escaping `watcher.emit` becomes an unhandled rejection on the non-awf path
  and an uncaught exception inside the `fs.stat` callback of
  `_awaitWriteFinish` (`:16761-16777`) on the awf path.
- vite-plus 1.0.0 installs `unhandledRejection`/`uncaughtExceptionMonitor`
  handlers only inside its interactive prompt module
  (`vite-plus/dist/prompts-jMH8S4Wp.js:1494-1503`), not while serving.
  `mise x -- node --version` is v24.21.0, whose default terminates the
  process on an unhandled rejection.
- Kit 3.0.0 `src/core/config/index.js:75-95` (`load_template`, called from
  `write_server.js:112`, which kit's `watcher.on('all')` listener in
  `dev/index.js:253-262` calls synchronously) and
  `src/messages/build-errors.js:62-64` with `src/messages/internal/build.js:11-13,22-36`
  (`throw_error` / `format`).
- `internal/adapter/skgo-adapter/env.js:1086-1098` — the template-guard
  `notify`; `internal/adapter/dev_watcher_test.go` — its contract.
- `git log` for the guard: introduced at `49cc2ea` when `example/web/
  package.json` pinned `@sveltejs/kit` `3.0.0-next.28`; the pin is now `3.0.0`.
- Reproduction run from `example/web` (`mise x -- node`): importing kit's
  `app_template_tag_missing({file:'src/app.html', tag:'%sveltekit.head%'})`
  throws `name: "SvelteKit error"`, `message:
  "app_template_tag_missing\nsrc/app.html is missing `%sveltekit.head%`\nhttps://svelte.dev/e/kit/app_template_tag_missing"`;
  comparing it to the guard's `src/app.html is missing %sveltekit.head%`
  prints `guardMatches: false`.
- `example/e2e/steps/dev.ts:87` — the browser suite's template edit is a
  single `writeFile` of the complete replacement, so it never exposes a
  partial template to kit.

## Round 02 findings, status

1. Steps 3/4 timing contradiction — **addressed**: step 4 defines a valid
   trial by a witnessed second fs event inside 50 ms of the first delivery,
   bounds retries at five, and treats no-witness as inconclusive. Finding 2
   below is about where that rule applies.
2. Escalation without ceiling — **addressed**: 400 ms ceiling, justified
   against the fixture's 500/700 ms spacing and ~1 s settle, with the
   fixture to be re-derived before exceeding it.
3. FSEvents coverage — **addressed in intent**: three mandatory macOS
   invocations with recorded `watcher.options`, SHA and results;
   disagreement blocks. Finding 2 below shows the rapid-restore contract as
   specified cannot be satisfied in two of the three.
4. Partial-save timing — **addressed**: coalesced and delivered paths, both
   relative to the threshold. Finding 1 below is what the delivered path
   will actually hit.
5. Wrapper mechanics — **addressed** (`fixture-base.ts`, restart note).

## Findings

### 1. Issue — Step 5's "delivered partial save" path depends on a guard that cannot match the pinned kit; today it crashes `vp dev`

The adapter's template guard (`env.js:1090-1092`) rethrows unless
`error.message === \`${authored} is missing ${tag}\``. Kit 3.0.0 throws, via
`throw_error`/`format` (`messages/internal/build.js:11-13,22-26`), a
three-line message with the code on the first line, the tag wrapped in
backticks on the second, and a docs URL on the third (reproduction above:
`guardMatches: false`). The guard was written at `49cc2ea` against
`3.0.0-next.28`; the EventEmitter contract (`dev_watcher_test.go:46-52`)
throws the old single-line text itself, so it keeps passing against a
message kit no longer produces — an expectation derived from the fixture
rather than from kit, the trap `CLAUDE.md` names.

Consequence under the pinned kit, with or without the plan's default: a
template save that exposes truncated bytes long enough for an event to be
delivered makes kit's `all` listener (`dev/index.js:253-262` →
`write_server.js:112` → `load_template`, `config/index.js:89`) throw; the
guard does not match; the throw escapes `watcher.emit` into chokidar's
`async _emit` (unhandled rejection) or, with `awaitWriteFinish`, into the
`fs.stat` callback of `_awaitWriteFinish` (uncaught exception). vite-plus
installs no serving-time handler and Node 24's default terminates the
process. The browser suite does not see this because `dev.ts:87` writes the
whole template at once.

Impact on the plan: the step-5 "held beyond the configured threshold until
the template validation error is actually observed" branch will not observe
an error overlay; it will watch `vp dev` exit, taking the rest of the
package with it. The plan should (a) name this as a known defect to repair in
the same change — match on kit's structured error (`error.name ===
'SvelteKit error'` and the `app_template_tag_missing` code, or at minimum
the backticked second line), (b) re-anchor the EventEmitter contract to
kit's real message by constructing it from kit's own `build-errors.js`
rather than a literal, and (c) make the real-server assertion's positive
observation the overlay/log line kit's text actually produces. Classified
as an issue rather than critical only because the plan's own step 5 will
surface it; left unplanned it becomes a blocked implementation.

### 2. Issue — The step-4 validity rule is unsatisfiable on two of the three mandated macOS backends and under-specified for CI

Step 4: "A rapid trial is valid only if the second event is witnessed
inside 50 ms of the first delivery … If no valid trial is obtained, fail
qualification as inconclusive." The validation section then mandates three
macOS invocations — forced `fs.watch`, native FSEvents, explicit polling —
"each" covering "restored module and Go-document contents".

- Polling: chokidar's `fs.watchFile` interval is 100 ms (`interval: 100` in
  every research `options` dump); two events for one path cannot be
  witnessed 50 ms apart, so every trial is out of window and the polling
  invocation of the rapid-restore contract is inconclusive by construction.
- FSEvents: the issue's own table records events "roughly 100 ms apart"
  under the default backend, so the witness is improbable there too.
- Linux CI (default mode, default present): five attempts to land a GET
  round trip plus a write inside 50 ms on a shared runner will sometimes all
  miss. If "inconclusive" fails the permanent test, CI flakes for reasons
  unrelated to product correctness, against the user's build-success
  priority; if it does not fail, the plan has not said what the test asserts
  in that case.

The plan already distinguishes "negative mutation proof belongs to
`fs.watch`" from "preservation runs", but the rule is written as a property
of the contract, not of the run. It should say: the witness requirement and
the inconclusive outcome apply to the validator's mutation run on
`fs.watch`; in every other run (CI default, FSEvents, polling) an
out-of-window trial still asserts the settled final state — restored disk
bytes, Vite response, change-log entry, Go document — and reports the witness
timing as evidence, never as a pass/fail criterion. That keeps the product
contract load-bearing everywhere and the throttle-drop proof where it can
actually be made.

### 3. Nitpick — "Qualify increases up to 400 ms if they improve … reliability" has no definition of reliability

A single qualification invocation per value cannot show one value is more
reliable than another. The plan should state the repetition (for example
`-count=N` on the focused contracts) that an "improves reliability" claim
requires, so the recorded evidence for 200/25 versus a larger value is
comparable rather than anecdotal.

### 4. Nitpick — Step 4's recorder should be named as a public API

The fixture plugin can witness the second filesystem event through
chokidar's public `raw` event (`node.js:15533`, emitted before both
throttles) and the first delivery through `watcher.on('change')`. Saying so
in the plan removes the temptation to wrap `_throttle` in the fixture, which
the research patch did and the plan otherwise discourages.

## Outcome

`material findings remain`
