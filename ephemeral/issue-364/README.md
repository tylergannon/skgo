# Issue 364 — documentation drift

Worktree: `/Users/tyler/src/skgo/.claude/worktrees/issue-364-ab900a` (branch
`claude/issue-364-ab900a`, at main `fa29e01a`). Issue text: `gh issue view 364`.

## Outcome

The README, the distributed skill (`skills/skgo/SKILL.md`), the generated app
README (`internal/newapp/gofiles/README.md.tmpl`) and the colocated guide pages
(`*.md` at the module root, `cmd/skgo/*.md`) describe current capabilities and
limitations consistently and correctly. Documentation only: no runtime
behaviour, no branding or headline change ("A Go backend for SvelteKit." stays).

## Evidence anchors

- Pinned Kit 3.1.0 source: `example/web/node_modules/@sveltejs/kit` (installed).
  `ephemeral/sveltekit-current/SKILL.md` explains how to read it.
- Commits that delivered the work the docs lag behind: `ea1d75a4` (Go routing,
  prerender entries, application error hooks), `fb8640df` (development edit
  recovery — closes #330 — and production lifecycle), `2dcc49a2` (prerender
  transform error policy, hash-router shell, Tailwind), `3b382953` (tracing,
  Kit 3.1), `66215e67`/#383 (guide pages moved beside code).
- Contracts: Go tests next to the code; browser scenarios `example/e2e/features`.
- Related issues worth reading: #367 (universal-load execution location), #176
  (extra MCP tools, not shipped), #330.

## Ownership (parallel agents in one worktree — stay in your topic)

- Topic A: universal loads, execution location (#367), prerendering (Go loads,
  `skgo.Entries`, remote prerender functions, remote query during prerender).
- Topic B: tracing/observability, `skgo check`, MCP (check/advice shipped vs
  #176), other tooling rows.
- Topic C: development edit recovery (#330 scope), `differences.md` items
  (route matching/trailing slash, hash router, prerender transform errors) and
  any other delivered-but-described-as-open statement.
- Lead: Node prerequisite (`internal/toolchain` + every "Node 22.17" line).
  Do not touch Node version text.

Several topics touch the same files (README tables especially). Edit only your
own rows/paragraphs; if an Edit fails because the file changed, re-read and
retry. Do not commit; the lead commits.

## Rules for the text

- Every claim checked against current code, tests or pinned Kit, not against
  another document. Link evidence the way the README already does.
- Keep the production/development distinction: production is goja SSR in the
  Go binary; development is native Vite/Kit with Go callbacks; a complete
  development browser-suite pass is not established.
- Guide pages: frontmatter keys `title`, `description` (<20 words),
  `division`; Go/Svelte/TS fences have empty bodies and an info string naming
  the source (see AGENTS.md "Documentation"). Never paste command output.
- Plain claims, no slogans, no status narration ("now", "recently fixed",
  "as of"). State what is true.
- Human-facing setup minutiae belong in `skills/skgo/SKILL.md`, not README.
- Never write under `docs/`. Never restore generated documentation removed by
  #383.
