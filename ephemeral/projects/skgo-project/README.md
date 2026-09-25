# skgo-project

Started 2026-09-25. The repo where skgo's intent lives: headline, docs site,
feature parity matrix, and the instruments that look at `skgo` and report
how far along each cell is. `skgo` keeps the code and `just test`; it does
not know how it is judged.

## Decisions so far

- The docs site is an skgo application. Not static, not GitHub Pages. If it
  is not dogfood it is nothing.
- The site demonstrates every matrix cell in place: a page about a feature
  is served by that feature. The matrix page is itself a server load; the
  status feed is a `+server` route in Go; forms on the site are form
  actions; a domain type crosses via transport; and so on. Each cell names
  the place on the site where it is in use.
- Each cell carries named validation and one of three states:
  - **pass**: every named check ran and passed in the latest cycle
  - **fail**: at least one named check ran and failed, or was skipped
  - **unknown**: no check is named, so we do not have it and cannot say
  A cell may also be marked planned, which is unknown with a stated intent.
- The site works with JavaScript off because graceful degradation is a
  premise of SvelteKit, and skgo inherits kit's premises. Not because it is
  documentation.
- The site is a live skgo server whose docs pages are prerendered and whose
  matrix, forms and status feed are dynamic. Both halves are cells.
  "Static export of a whole site" is its own cell; today the matrix says
  prerendering is Limited (a branch with a Go server load cannot be
  prerendered, prerendered redirects are refused, no build-time remote
  execution). Whether skgo can build a purely static site is therefore a
  matrix question with a current answer of partly, and the site should make
  that answer visible rather than assume it.
- The matrix reflects the most recent validation run regardless of the
  site's own release cycle. The runner writes one status record per cell;
  the site's Go load reads them.
- Cells demonstrate unhappy paths too, in place: a form that rejects bad
  input and keeps the fields, a 404, an error boundary, a forbidden action, a
  redirect after a submit. The site is not limited to the happy path; the
  example app already shows failure modes and the site shows them the same
  way. What the site cannot do is exhaustive contract coverage; that is what
  the named tests are for.
- Headline widens from "A Go backend for SvelteKit" toward full Go tooling
  for SvelteKit, because `skgo check`, the advice analyzers and the MCP
  server exist. Every word still has to be demonstrable; the tooling cells
  in the matrix are what make the wider claim honest.
- `skgo/example` stays. It is the test fixture. The site is the demonstration.
- `skgo`'s browser suite stays in `skgo` as an ordinary CI job on main. What
  moves is interpretation: `skgo-project` reads results and turns them into
  cell status.

## Red team

`skgo-project` hosts agent loops that use the site as a black box and try
to break it. They never see, read, or are told about `skgo`'s source; they
get the running site, its user documentation, and a brief. This is the
reason the two repos are separate: `skgo` stays narrow and has no agents in
it, and the agents that hammer it live where the intent lives.

- Same rule `gimble run validate-product` already enforces: testers never
  inspect the implementation of the product under test. The red team is
  that rule made standing rather than occasional.
- Not a mapped pathway suite. Exploratory: load it up, turn JavaScript off,
  submit garbage, race two tabs, resubmit a form, hit back after a redirect,
  poke the JSON routes directly, whatever a hostile or clumsy user would do.
- Requires enough surface to be worth attacking: real actions, a database
  behind something (the junkyard guestbook was paid for and is the obvious
  seed), sessions, uploads, streaming. The site grows features partly so the
  red team has something to break.
- Output is findings filed as issues against `skgo`, with screenshots read
  by a cheap model first. Findings do not flip matrix cells; cells are
  driven by named checks. A finding that reveals a missing check becomes a
  new named check, and then the cell can fail honestly.
- Runs on a cadence or after a substantial change, from `skgo-project`'s own
  workflows. Never from `skgo`'s CI.

## Open questions

- **Hosting.** For now: serve from Tyler's machine behind a Cloudflare tunnel.
  Vercel's Go runtime is a per-request function model and skgo is a
  long-running server with a warm goja pool; whether that fits is a research
  spike, and "Deploy: Vercel" becomes a matrix cell in the planned state
  until it does.
- **Where the status records live.** Simplest: a directory of JSON files the
  runner commits or uploads, read by the site's load. A database only if
  history or trends are wanted.
- **What counts as a cell's validation.** The schema decision everything
  else hangs from. Proposal: a cell names Go test IDs in `skgo`, Gherkin
  scenario IDs, and optionally one gimble workload. The runner resolves each
  name to a result. A name that resolves to nothing is a fail, not unknown.

## To-do, not now

- **SvelteKit survey.** Spend real time over kit's pinned source and current
  docs listing everything kit does that skgo does not yet, and decide what
  belongs on the matrix. The goal is the whole experience of a Go developer
  publishing to the web through SvelteKit, not parity with the current
  README. Owner: a dedicated session, not a side task.
- **Vercel spike.** Deploy a trivial skgo app to Vercel's Go runtime and
  report what breaks. Output is a matrix cell state, not a document.
- **Cell schema.** Write the schema and one fully specified cell (form
  actions is the obvious first) before any site code.
- **Instrument runner.** A workflow in `skgo-project` that checks out `skgo`
  at its latest tag, runs the named checks and the look, and writes the
  status records.
- **Migrate `skgo/docs/`.** Once the site is live, `docs/` in `skgo` becomes
  a redirect and `pages.yml` goes.

## Sequence

1. Cell schema and one specified cell.
2. Repo, README with the headline, matrix as data.
3. The site as an skgo app, using each feature it documents.
4. Runner and status records; the matrix goes live from real results.
5. Tunnel it. Then the Vercel spike.
