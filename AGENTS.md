# skgo

**A SvelteKit application server written in Go.**

A Go program owns the socket and the process. Its Kit-facing interface is an
ordinary SvelteKit adapter, so the frontend stays plain SvelteKit — kit's
tooling, kit's conventions, no Go-isms. Go serves static and prerendered output
natively, makes the trust decisions, and supervises a Node sidecar running
SvelteKit's own server for rendering. Server logic — loads and remote functions
— can be written in Go with end-to-end types; anything not written in Go stays
plain kit.

To a Go developer: a real frontend framework for a Go monolith. To a Svelte
developer: the app is still SvelteKit.

One binary. One build gesture. A system Node for the sidecar.

We never reimplement kit, never build or maintain a JS runtime, and never claim
anything that hasn't been demonstrated running.

## Read this first

**Invoke the `agent-protocol` skill at the start of every session and after any
context compaction.** It is mandatory, not advisory: worktrees, worklogs,
`ephemeral/`, checkpoint commits, squash-merge PRs, and root-checkout
synchronization all come from there. Do not improvise a substitute.

## Rules

**This is a Go library.** Write Go. Test with `go test`. Build tooling as Go
commands. JavaScript exists here only where SvelteKit itself requires it — the
adapter package and the sidecar entry — never as the implementation language for
anything skgo owns. No `.mjs` harnesses, no shell test runners.

**Build the software.** Not ledgers, sprint files, chapter docs, acceptance-gate
runners, adjudication records, proof directories, or evidence manifests. When
asked to build something, build that thing. Verification is ordinary Go tests
and running the real program.

**Don't narrate.** No status documents, no progress reports, no summaries of
work already visible in the diff. The worklog exists for actionable
intelligence — corrections, traps, things that will change a future decision —
and nothing else.

## Layout

- `ephemeral/` — tracked working material: worklogs, plans, downloaded
  artifacts. Never write to `docs/` without express permission.
- `ephemeral/inspiration/junkyard/` — an earlier attempt that never produced a
  working server, kept as gitignored read-only reference. Mine it: the
  Playwright task suite, the guestbook app, and the kit-3 facts in
  `.agents/skills/sveltekit-current/SKILL.md` were all paid for. Take any idea
  that earns its place. What does not come across is the process — its ledgers,
  sprint contracts, proof harnesses, review rounds and evidence directories are
  what it built *instead of* the product.

## SvelteKit facts

Kit 3 breaks most model priors: `svelte.config.js` is gone, `transport` is a
universal hook, `$lib` became `#lib`, the app's origin is fixed at build time,
and TypeScript 7 cannot build it. Read
`ephemeral/inspiration/junkyard/.agents/skills/sveltekit-current/SKILL.md`
before writing SvelteKit code, and check pinned kit source over any
documentation site.
