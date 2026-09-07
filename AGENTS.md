# skgo

**A SvelteKit application server written in Go.**

A Go program owns the socket and the process. Its Kit-facing interface is an
ordinary SvelteKit adapter, so the frontend stays plain SvelteKit — kit's
tooling, kit's conventions, no Go-isms. Go serves kit's client-rendered output
natively, makes the trust decisions, and answers the endpoints kit's client
already calls: remote functions (`/_app/remote/...`), server loads
(`__data.json`) and the routes a `+server.ts` declares.

**Every server endpoint is Go's.** Not "Go where you want it, kit otherwise" —
all of them. Loads and remote functions are written in Go with end-to-end types,
and a `+server.ts` route is an ordinary `net/http` handler written beside it;
the JavaScript kit requires is a generated stub that throws, so any real
response proves Go answered. Running remote functions or server routes in
TypeScript is not a supported mode.

**CSR today. SSR is decided, and being built.** What ships now runs with
`ssr = false`: the document is kit's own SPA fallback, kit's server bundle is
built but never runs, and no JavaScript executes in production.

SSR is no longer conditional or unproven. The CSR base case proved out, and a
spike server-rendered four pages of the example app inside goja — a pure-Go
engine embedded in the binary — byte-identical to the same bundle under Node,
with Go answering every remote function in-process. One process, one binary, no
cgo, no Node at request time.

The spike and its proposal are not on `main` yet: they live on the branch
`claude/ssr-no-node-sidecar-9e3be4` as `internal/ssrspike/` and
`ephemeral/brief/2026-09-06-ssr-in-process.md`. Read them there, on that
branch, before planning an SSR slice — they answer issue #7 and they answer the
AsyncLocalStorage question that issue named as the kill criterion.

Build functionality first. The performance levers — a runtime pool, an engine
swap — sit behind the same seam and are deferred, not forgotten; the pool is
mandatory rather than optional, because a re-entrant render on one runtime
returns empty with no error. A supervised Node sidecar is not on the table and
that question is closed.

Until a slice actually ships, do not describe the system as having SSR.

To a Go developer: a real frontend framework for a Go monolith. To a Svelte
developer: the app is still SvelteKit.

One binary. One build gesture. Node is a build-time dependency only.

We never reimplement kit, never write or maintain a JavaScript engine —
embedding an existing one, as SSR does with goja, is a different thing — and
never claim anything that hasn't been demonstrated running.

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
rules, not merely resemble its ergonomics — the browser runs kit's own client, so
a constraint kit enforces on the server usually exists to protect a client-side
invariant (an argument-keyed query cache, say), and dropping it breaks the client
rather than skgo. If kit answers it, it was never a decision, only unmapped
research.

**Delegate missions, not methods.** A subagent gets: the capability a developer
should have when it is done, kit as the spec and where the pinned source is, the
proof that will be accepted, the non-negotiable constraints, and what it owns so
parallel work does not collide. It does its own mapping and chooses its own
path — that research is the work, and it must not arrive secondhand. Detailed
instructions cap an agent's quality at the dispatcher's understanding and
transmit the dispatcher's unverified assumptions as fact. Sprint documents obey
the same rule: mission, acceptance, ownership. No file lists, no prescribed
designs, no step-by-step.

**Acceptance is the Gherkin suite.** Behaviour written in business language is
the contract, and it exists so nobody has to read code to decide whether the
software works. Define what winning looks like as scenarios; let the builder
choose how. Validate by dispatching an agent that checks the scenarios are
load-bearing — would each one actually fail if the feature were broken or
removed — and then runs them. Do not audit diffs or re-derive claims from
source as a substitute; a passing load-bearing suite is the answer.

**A passing command is not evidence.** An exit code of zero is exactly what a
silently skipped test prints. Validation has to produce artifacts a human can
look at without reading code or rerunning anything: screenshots of the running
app at the moment each scenario asserts, and they have to be good — the actual
page, showing the actual state the scenario claims, legible enough to tell
whether it is real. A scenario that cannot produce one is a scenario nobody can
check. Skips are failures: a check that cannot run because its toolchain is
missing has not passed, and must not be able to report that it did.

**Never derive what you expect from the thing you are testing.** The one leak
this project has shipped got past an assertion that was already exact — the live
count had to equal *the count noted a moment earlier, plus one* — because the
baseline was read off the same broken counter. Every wrong value satisfied it.
Anchor an expectation in something independent: a fixture the test supplied, the
rows actually visible on the page, a number written into the scenario. The same
trap in another shape is a `Then` whose only content is "X is absent", which a
page that rendered nothing at all satisfies perfectly.

Assert at the granularity the behaviour deserves, and tighten one when it lets a
real bug through. A suite tightened *a priori* is brittle, and brittle scenarios
get deleted rather than fixed.

Then **open every screenshot and look at it.** Producing them is half the job;
an unexamined screenshot is the same unread artifact as an exit code. A green
suite over a screenshot showing an error boundary, an empty list, a stack trace
or a blank page means the suite is wrong, and that is the single most valuable
thing an agent can find. Never hand a human a screenshot you have not looked at,
and never hand them one that obviously shows a failure — if it does, the finding
is the failure, not the file.

**Wake yourself.** Every dispatched run gets a watcher started in the same
breath — a backgrounded `until ! kill -0 <pid>; do sleep 20; done` on its
process — so the session is re-entered the moment the run exits. A tractor run
does not announce itself. Without a watcher the work finishes and sits there,
and the human ends up asking whether anything is happening, which is the one
question a lead should never make someone ask.

**The pinned sources live at one absolute path.** `ephemeral/inspiration` is
gitignored, so it exists only in the root checkout — a worktree does not have
it, and an agent dispatched into one and told to read kit finds an empty
directory and proceeds on priors, silently, on the one thing that was supposed
to be non-negotiable. Refer to it as
`/Users/tyler/src/skgo/ephemeral/inspiration`, which is reachable from any tree
and cannot be committed by accident. Do not copy or symlink it into a worktree.

**There is no command that means "done."** The `Justfile` holds recipes that do
things — build, test, serve, run the suite — and not one of them returns a
verdict. Do not write one. A single script that exits 0 is the thing every
agent starts building toward, and it cannot see the app: that is how a project
reaches ten thousand lines with nothing working. `just test` passing is a fact
about the tests, not about the software.

**Don't narrate.** No status documents, no progress reports, no summaries of
work already visible in the diff. The worklog exists for actionable
intelligence — corrections, traps, things that will change a future decision —
and nothing else.

## Layout

- `ephemeral/` — tracked working material: worklogs, plans, downloaded
  artifacts. Never write to `docs/` without express permission.
- `/Users/tyler/src/skgo/ephemeral/inspiration/` — the pinned upstream sources,
  gitignored and read-only. `reference/` holds kit, devalue, sirv, mrmime and
  polytype at their pinned commits; `junkyard/` is an earlier attempt that never
  produced a working server. Mine the junkyard: the
  Playwright task suite, the guestbook app, and the kit-3 facts in
  `.agents/skills/sveltekit-current/SKILL.md` were all paid for. Take any idea
  that earns its place. What does not come across is the process — its ledgers,
  sprint contracts, proof harnesses, review rounds and evidence directories are
  what it built *instead of* the product.

## SvelteKit facts

Kit 3 breaks most model priors: `svelte.config.js` is gone, `transport` is a
universal hook, `$lib` became `#lib`, the app's origin is fixed at build time,
and TypeScript 7 cannot build it. Read
`/Users/tyler/src/skgo/ephemeral/inspiration/junkyard/.agents/skills/sveltekit-current/SKILL.md`
before writing SvelteKit code, and check pinned kit source over any
documentation site.
