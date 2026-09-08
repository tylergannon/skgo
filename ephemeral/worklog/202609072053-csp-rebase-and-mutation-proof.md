# PR 86 rebase, mutation proof, and a live concurrent-process collision

friction: Mid-session, `git status`/`git diff` on this exact worktree showed
csp_test.go, csp.feature, csp.ts and vite.config.ts changing content I had not
written, with mtimes seconds old. Investigation (`ps`/`lsof` by exact pid, not
`pkill`) found a live `just serve` + `just e2e` process tree rooted at PPID 1
(orphaned to launchd) running in this identical worktree path, plus a shell
snapshot from the harness's own Bash-tool convention — almost certainly this
same agent's prior incarnation, killed by a context compaction (matches the
`compaction-kills-background-agents` memory: the CLI process restarts, but
background children it spawned keep running as orphans). Its foreground edits
were mid-write when I first observed them; a `git status` taken then would
have been misleading. -> Before trusting a `git status`/test failure as your
own regression in a resumed/handed-off worktree, check for orphaned processes
by exact pid (`ps -eo pid,ppid,etime,command | grep <worktree-path>`) rooted at
PPID 1, and re-check file stability (diff twice, a few seconds apart) before
deciding a file's uncommitted content is stale vs. actively being written.
Once the writer process ended (its `just e2e` finished), the tree stabilized
and its edits turned out to be correct, further-along continuation of the same
task — adopted rather than reverted.

decision: Rebasing PR 86 onto post-#80 main required manually porting the
CSP `req.csp` wiring from the old monolithic `internal/adapter/skgo-adapter.js`
into the new `internal/adapter/skgo-adapter/entry.js` (the file that now holds
`__skgo_render`) rather than trusting git's merge — the conflict's "ours" side
was empty (content moved, not edited) so the standard 3-way merge produced a
huge, useless conflict block. Ported by hand (10 lines), then regenerated
`example/web/skgo-adapter.js`/`skgo-adapter/entry.js` via `just generate` and
diffed to confirm the source of truth (`internal/adapter/...`) was untouched by
the rebase apart from the PR's own +9 line `csp: builder.config.csp` addition.

decision: `example/ssr_test.go`'s `TestARenderedPageDoesNotRepeatItself`
assumed byte-identical reruns are possible for any page. Once the example's
CSP became global (`mode: 'auto'`, resolving to nonce for every page since
none prerenders — see below), that assumption is false by construction: kit
computes the ETag from the nonce-substituted HTML (`render.js:636`), and
reusing a nonce across renders would defeat CSP nonce's whole purpose (kit's
own docs treat "no HTTP caching under nonce CSP" as an accepted trade-off, not
a bug). Fixed by stripping the one nonce attribute before comparing bodies
(still catches real nondeterminism, e.g. map-iteration-order bugs), asserting
the nonce itself is fresh every render, and requiring a fresh 200 (not a false
304) for a conditional request against a stale ETag. Verified: only the single
`nonce="..."` attribute differs between two renders of `/items/42`; everything
else byte-identical.

decision: Between the fork point and this rebase, `/about` lost its
prerendered status — the root layout gained a Go load for #81's `error.html`
fixture, and kit refuses to prerender any branch with a Go-only load
(the root layout is in every branch). The prior WIP's csp.feature/csp.ts/
vite.config.ts scenario proving hash-mode via `/about`'s real prerendered
output no longer has a fixture to run against; the Go-level test
(`TestCSPAutoMode_DynamicIsNonceModePrerenderedWouldBeHashMode`) is now the
whole proof of that branch until #81 lands. This explains the mission's noted
"one known-red engine-build.feature scenario" too — same root cause, in scope
for #89.

Mutation proof, verbatim results: (a) forcing the streamed-chunk `nonceAttr`
to always be `""` in `document.go`'s `stream()` failed all four named
streaming scenarios plus csp.feature's `/stream` scenario; restoring passed
all. (b) `cspProvider.source()` returning `""` failed 8/10 Go CSP tests and
both csp.feature scenarios (the header ends up with no nonce/hash source at
all, so the browser blocks every inline script including the boot script, not
just streamed chunks); restoring passed all.
