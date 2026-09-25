# Lean browser suite

trap: `CLAUDE.md` is a symlink to `AGENTS.md`. A script that edits both in turn
edits one file twice; the second pass fails its own "old text present" check.

trap: "the browser never asked for the page's data" and "exactly 0 remote
requests were made since" pass on a page whose client never booted. Kit's
`_hydrate` never fetches `__data.json`, so the data count is a document claim;
the remote count is a client claim only after `_start_router` ran. Both steps
now call `booted(page)` (strict; `hydrated` gives up silently when there is no
boot script) before counting.

trap: a Chromium "native form" scenario that asserts nothing after hydration is
not a client claim. Its browser half (a real form post) is the noscript
project's; its bytes are a Go test. Only "native then hydration" rows, which
click the hydrated page afterwards, need the browser in scripting mode.

trap: the native request-context scenarios filled the form without waiting for
hydration (`submit()` hydrated only for enhancement). In dev that is the
"submitted Ada instead of Grace" failure #183 names. They are gone; the claim
is `TestANativeActionIsAnsweredWithTheDocument`.

trap: the Write tool turned `<` inside a Go raw string into `<`. Write
escape-bearing literals with a script, and grep the file afterwards.

tool: `pnpm exec bddgen export --unused-steps` lists orphaned step definitions.
Its table splits a regex step's `|` alternation into columns; handle those by
hand. `tsc --noEmit -p . --noUnusedLocals` then finds the orphaned helpers.

open: `zz-source-update`'s "next document follows live source" is an HTTP claim
with no Go home until the dev-renderer `TestMain` lands (validation.md step 1);
it is folded into the HMR scenario rather than deleted. Move it when that lands.

open: the Justfile comment on `log` still says a scenario reads the engine's
console from the server log. That claim is now
`TestWhatTheRendererReportsReachesTheServerLog`; the suite still reads the log
for Kit's dev-only SSR-off warning in `actions-options`. The comment is outside
this change's ownership (header only).
