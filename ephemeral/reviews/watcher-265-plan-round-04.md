# Adversarial review: fix plan for #265 — round 04

Written 2026-10-06, local time. Read-only review; nothing outside this file
was changed. Earlier rounds: `watcher-265-plan-round-01.md` … `-03.md`.

## Target

The fourth revision of `ephemeral/plans/watcher-265-fix.md` (untracked,
worktree `watcher-265` at `eec9fd4`). Still no implementation: `git diff
HEAD` outside `ephemeral/` is empty, so "current implementation and proof"
is the code the plan builds on plus the research logs, unchanged since
round 01.

Authoritative sources are unchanged: issue #265, `ephemeral/research/
watcher-265/`, the installed `@sveltejs/kit` 3.0.0 and the chokidar 3.6.0
bundled in `@voidzero-dev/vite-plus-core@1.0.0`, `CLAUDE.md`, and the user's
priority of stability and build success over modest added latency. No scope
narrowing was requested; none was ignored.

## Evidence inspected

Everything from rounds 01–03, re-read against the revised steps, plus:

- `internal/adapter/skgo-adapter/env.js:1099-1115` — the `known` map is
  populated only on a non-`all` `add` event or by `transform`'s `remember`;
  with Vite's `ignoreInitial: true` (`node.js:21329`) no `add` fires for files
  present at startup, and `src/app.html` is never transformed, so its digest
  is unknown until its first delivered `change`, after which `unchanged`
  (`:1076-1082`) suppresses any later change whose bytes match.
- `example/devrender/devrender_test.go:280-316` —
  `TestATemplateEditReachesTheNextDevDocument` writes a new marker and defers
  restoring the original, i.e. two delivered changes to `app.html` that leave
  `known` holding the original's digest for every test that runs after it.
- Kit 3.0.0 `package.json` `exports`: `./src/messages/build-errors.js` is not
  exported (`ERR_PACKAGE_PATH_NOT_EXPORTED` when imported by specifier from
  `example/web`; importing by file URL works, which is how
  `dev_watcher_test.go:29-31` already loads `env.js`).
- Kit `src/messages/build-errors.js:52-55` — the sibling
  `app_template_missing` ("does not exist") thrown by `load_template`
  (`config/index.js:84-86`) when kit's `all` listener runs on an `unlink`.
- Bundled chokidar nodefs handler: a write to a watched file produces a
  `raw` event from the file's own `fs.watch` and another from its
  directory's (`node.js:15531-15542`, `fsWatchBroadcast`), so a witness sees
  two raw events per write.

## Round 03 findings, status

1. Template guard cannot match kit 3.0.0 — **addressed** (step 5: structured
   match on name, code line and body; EventEmitter contract re-anchored to
   kit's installed helper; scoped as a narrow prerequisite).
2. Witness rule unsatisfiable on FSEvents/polling and under-specified for
   CI — **addressed** (step 4: witness required only in the `fs.watch`
   mutation run; all other runs assert settled state and log timing).
3. "Improves reliability" undefined — **addressed** (`-count=3`, a
   reproducible failing condition at the smaller value required, no
   statistical claim).
4. Public `raw` event — **addressed** (step 4 names `raw` and `change`).

## Findings

### 1. Issue — The coalesced partial-save contract (step 6) is not load-bearing unless the completed save carries a marker the template did not already have

Step 6 asks for "an incomplete template write completed inside the
configured quiet window" that "must recover to a served complete template
with its literal marker". It does not say the marker must be new. If the
completed save restores the template's existing bytes, the assertion cannot
fail:

- If `known` holds the template's digest — which it does whenever
  `TestATemplateEditReachesTheNextDevDocument` (`devrender_test.go:280-316`)
  ran earlier in the package — the single coalesced `change` is suppressed
  by `unchanged` (`env.js:1076-1082`, `:1110`), nothing regenerates, and the
  served template is the pre-test one. The assertion passes with the watcher
  silenced.
- If `known` is empty (test ran first), the change is delivered and the
  served template is still the pre-test one. The assertion passes with the
  watcher dead, too: a server that never saw either write serves the same
  bytes.

The outcome depends on test order and in neither case can the assertion
distinguish a working stability path from no watcher at all — the "`Then`
whose only content is 'X is absent'" shape `CLAUDE.md` warns about, here as
"X is unchanged". The delivered path is safe because it requires observing
kit's error first; the coalesced path has no such anchor. The plan should
require that both partial-save paths complete to a template carrying a
literal marker distinct from the one served before the test, written into
the test the way the existing template test does (`before`/`after`), and
that the served document is observed to carry the new marker and not the
old.

### 2. Nitpick — Step 5's guard repair should decide about `app_template_missing`

`load_template` throws `app_template_missing` ("does not exist",
`build-errors.js:52-55`, `config/index.js:84-86`) when kit's `all` listener
runs on an `unlink` of the template — an editor whose rename-over save
exceeds chokidar's 100 ms `atomic` hold. It is the same class of throw, from
the same listener, with the same crash consequence as the one step 5
repairs, and the structured match the plan specifies (name + code line) can
include it at no cost. The plan says "not a broad error-handling rewrite",
which is right; it should still say whether this sibling code is in or out,
so the implementer does not have to re-derive the question.

### 3. Nitpick — Two import/invocation details that will otherwise stall steps 1 and 5

Kit does not export `./src/messages/build-errors.js`; the re-anchored
EventEmitter contract must import it by file URL from the installed package
directory (as `dev_watcher_test.go` already does for `env.js`). And Vite only
runs `config` hooks of plugins whose `apply` matches the command, so the
step-1 `resolveConfig` contract must resolve with `command: 'serve'` or the
`apply: 'serve'` plugin's hook never runs and the "default received"
assertion fails for the wrong reason. One line each in the plan.

### 4. Nitpick — The raw-event witness sees two events per write

On the nodefs backend a write produces a `raw` event from the file's own
watcher and another from its directory watcher. The witness should match on
the resolved file path and take the earliest, otherwise the 50 ms window is
measured against whichever arrives second.

## Outcome

`material findings remain`
