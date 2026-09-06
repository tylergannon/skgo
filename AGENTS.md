# skgo

**A SvelteKit application server written in Go.**

A Go program owns the socket and the process. Its Kit-facing interface is an
ordinary SvelteKit adapter, so the frontend stays plain SvelteKit — kit's
tooling, kit's conventions, no Go-isms. Go serves kit's client-rendered output
natively, makes the trust decisions, and answers the endpoints kit's client
already calls: remote functions (`/_app/remote/...`) and server loads
(`__data.json`). Server logic — loads and remote functions — is written in Go
with end-to-end types; anything not written in Go stays plain kit.

**CSR only, to start.** The app runs with `ssr = false`; the document is kit's
own SPA fallback; kit's server bundle is built but never runs, and there is no
Node process at runtime. Server-side rendering via a supervised Node sidecar is
a feature we will add only if the CSR base case proves out. Do not design for
it, plan around it, or describe the system as having it.

To a Go developer: a real frontend framework for a Go monolith. To a Svelte
developer: the app is still SvelteKit.

One binary. One build gesture. Node is a build-time dependency only.

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
adapter package and generated stubs — never as the implementation language for
anything skgo owns. No `.mjs` harnesses, no shell test runners.

**Build the software.** Not ledgers, sprint files, chapter docs, acceptance-gate
runners, adjudication records, proof directories, or evidence manifests. When
asked to build something, build that thing. Verification is ordinary Go tests
and running the real program.

**This is mirroring, not design.** Kit is the specification. Before any design
question, recommendation, or option list about a feature skgo mirrors, map kit's
own implementation from pinned source — its API shape, its constraints, what it
forbids and why. Kit has usually already answered the question, and more
specifically than the answer being invented. A mirrored feature must obey kit's
rules, not merely resemble its ergonomics: the brief promises Go and kit code
coexist in one app, so identical functions must behave identically. If kit
answers it, it was never a decision — only unmapped research.

**Delegate missions, not methods.** A subagent gets: the capability a developer
should have when it is done, kit as the spec and where the pinned source is, the
proof that will be accepted, the non-negotiable constraints, and what it owns so
parallel work does not collide. It does its own mapping and chooses its own
path — that research is the work, and it must not arrive secondhand. Detailed
instructions cap an agent's quality at the dispatcher's understanding and
transmit the dispatcher's unverified assumptions as fact. Sprint documents obey
the same rule: mission, standard, ownership. No file lists, no prescribed
designs, no step-by-step. Rigor belongs on the receiving end — verify outcomes
against the standard, check claims against source, reject work that misses the
bar.

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
