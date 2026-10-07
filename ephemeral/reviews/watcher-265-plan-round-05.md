# Adversarial review: fix plan for #265 — round 05

Written 2026-10-06, local time. Read-only review; nothing outside this file
was changed. Earlier rounds: `watcher-265-plan-round-01.md` … `-04.md`.

## Target

The fifth revision of `ephemeral/plans/watcher-265-fix.md` (untracked,
worktree `watcher-265` at `eec9fd4`). Still no implementation: `git diff
HEAD` outside `ephemeral/` is empty. "Current implementation and proof" is
the code the plan builds on plus the research logs, unchanged since round 01.

Authoritative sources are unchanged: issue #265, `ephemeral/research/
watcher-265/`, the installed `@sveltejs/kit` 3.0.0 and the chokidar 3.6.0
bundled in `@voidzero-dev/vite-plus-core@1.0.0`, `CLAUDE.md`, and the user's
priority of stability and build success over modest added latency. No scope
narrowing was requested; none was ignored.

## Evidence inspected

Everything from rounds 01–04, re-read against the revised steps, plus the
paths the new absent-template contract (step 6) and the guard extension
(step 5) will exercise:

- `internal/adapter/skgo-adapter/env.js:808-831` — the adapter's
  `watcher.on('all')` listener feeds the change log for *every* event kind
  (`add`, `change`, `unlink`) and bumps `manifestVersion` when the file is
  kit's app or error template; `:842-852` — `/__skgo_dev/info` reads
  `kit.files.appTemplate` synchronously inside a try/catch and answers 500
  while the file is absent; `internal/vite/dev.go:169` — Go fetches `/info`
  and installs the template when the version moves.
- Kit `src/exports/vite/index.js:234,267-274,572-577` — kit's `root` is
  `resolve_root(config)` from Vite's resolved config, the same root the
  adapter guard relativises against (`env.js:1090`), so kit's
  `${relative} is missing …` / `${relative} does not exist` bodies and the
  guard's `authored` agree.
- Kit `src/exports/vite/dev/index.js:253-262` — kit's `all` listener is
  registered in kit's `configureServer`, which runs before the adapter's, so
  a throw there (caught by the guard in `emit`) skips the adapter's own
  `all` listener for that event; the next successful event runs both.
- Bundled chokidar `_remove` (`node.js:16866-16868`) cancels a pending
  write before emitting `unlink`; `_emit`'s `atomic` branch (`:16653-16667`)
  holds an `unlink` 100 ms and converts a following `add` into `change`;
  `_handleFile`'s `_throttle(EV_ADD, file, 0)` and `_emit(EV_ADD)` route a
  re-created file through the `awaitWriteFinish` path. So the held-absence
  sequence the plan describes (unlink → kit's `app_template_missing` →
  guard → re-create → deferred `add` → kit regenerates → version moves)
  is what the pinned code does on `fs.watch`, and on FSEvents (`atomic`
  false) the `unlink` is simply delivered sooner.
- `server.ws` is still a property of the pinned Vite server object
  (`node.js:30051`), so the guard's overlay `send` is live, not only mocked.
- `example/web/package.json` at `49cc2ea` versus now: `vite-plus` 0.3.3 →
  1.0.0 and kit `3.0.0-next.28` → `3.0.0`; the guard's drift is a
  pin-update consequence, which step 5's re-anchoring to kit's installed
  helper addresses at the root.

## Round 04 findings, status

1. Coalesced partial-save marker — **addressed**: step 6 requires a literal
   `before` observed first and a different literal `after` asserted present
   with `before` absent, for all three paths, and says why identical bytes
   are excluded.
2. `app_template_missing` sibling — **addressed**: step 5 includes it;
   step 6 adds the held-absence path.
3. File-URL import and `command: 'serve'` — **addressed** (steps 5 and 1).
4. Two raw events per write — **addressed** (step 4: match the resolved
   path, take the earliest after the restore write).

## Findings

No material findings remain. The plan now maps each claim to the pinned
source, assigns the negative proof to the one backend that can make it,
keeps the product contract load-bearing everywhere else, bounds the tuning,
repairs the guard it depends on, and anchors every assertion to a literal the
test supplies. What follows are nitpicks.

### 1. Nitpick — Go must not be asked for a document while the template is held absent

During step 6's held-absence window `/__skgo_dev/info` answers 500
(`env.js:842-852`), so any document rendered through `example.NewHandler`
in that window fails for a reason unrelated to the contract. The plan keeps
Go qualification "separate from the race-sensitive module sequence" for the
rapid trial; it should say the same for the three template paths: observe
the kit error in the log, complete the save, then assert the Go document
carries `after`. "Server remains alive" is the `vp dev` process and a
successful post-save request, not a request made during the absence.

### 2. Nitpick — `viteLog` gains more concurrent readers

Step 6 observes kit's error "in the emitted error message/log", which in
the fixture means reading `viteLog` (`devrender_test.go:48-49`), a
`bytes.Buffer` the child's stdout/stderr pipe goroutine writes while tests
read it. The existing build-emit test already does this; adding three more
readers makes it worth wrapping the buffer in a mutex-guarded writer so a
future `-race` run does not flag the fixture rather than the product.

### 3. Nitpick — Record the listener-order dependency the guard relies on

The guard works because kit's `all` listener runs before the adapter's and
the throw is caught in `emit`, which skips the adapter's `changed.push` /
`manifestVersion++` for the failing event only. That is correct behaviour
(nothing to serve from a broken template) but it is implicit; a sentence in
the guard's comment, or in the worklog, saves the next reader from
re-deriving why the version does not move on the error event and does move
on the recovery.

## Outcome

`only nitpicks remain`
