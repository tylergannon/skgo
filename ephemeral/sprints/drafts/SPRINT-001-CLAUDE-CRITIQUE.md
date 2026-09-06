# Sprint 001 draft critique (Codex vs Gemini)

Verified against pinned sources under `ephemeral/inspiration/`, routed through
`ephemeral/semantic-index/TAXONOMY.md` route A. Citations below are
`<leaf or reference path>:L<start>-L<end>`; leaf paths are relative to
`ephemeral/semantic-index/sources/`, reference paths relative to
`ephemeral/inspiration/`. The adapter-node choice itself is a settled decision
(intent, "Decisions taken with Tyler") and is not relitigated here — both
drafts are judged on how well they navigate its consequences.

## Verified facts that decide several findings below

- **`embed.FS` always reports a zero `ModTime`.** Confirmed empirically
  (`go run` against a scratch `//go:embed` program, this session): `Stat().ModTime().IsZero() == true`
  for every embedded file, on Go 1.27.1. Any weak-ETag scheme of the form
  `W/"<size>-<mtimeMs>"` collapses to `W/"<size>-0"` for every file, which (a)
  collides across same-size files with different content and (b) never
  changes across redeploys that don't change a file's byte length. This is
  decisive for the ETag comparison below.
- **adapter-node serves `client` before `prerendered`, and never sets a
  Cache-Control for generic (non-immutable) client files** — the pinned
  `sirv(...)` call for the client dir is `{ etag: true, gzip: PRECOMPRESS, brotli: PRECOMPRESS, setHeaders }`
  with no `maxAge`, and sirv's own rule is "`immutable`/`maxAge` absent → no
  Cache-Control header" (`libs/sirv.md:L35-L39, L183-L188`; adapter-node options
  table, same file `:L183-L196`). The semantic index's own **Go implementation
  notes** in `kit/build-adapt/static-serving.md:L110` and `build-output.md:L146-L147`
  independently *recommend* `etag, max-age=0` for Go's generic static
  responses as a design choice — that recommendation, not raw adapter-node
  behavior, is the closest thing either draft can cite for a `max-age=0`
  claim.
- **The dedicated, pinned-source-recommended way to get an SPA fallback shell
  is `builder.generateFallback(dest)`**, which renders a **root-layout-only**
  branch via a distinct code path (`resolve()`'s `prerendering.fallback`
  short-circuit), not by prerendering `/` as an ordinary route
  (`kit/build-adapt/adapter-api.md:L63, L195-L250`; `kit/server-runtime/csr-shell.md:L23-L26, L129-L136`).
  `@sveltejs/adapter-node` does not call `generateFallback` itself — only a
  custom `adapt()` receives the `Builder` needed to call it
  (`kit/build-adapt/adapter-api.md:L15-L34, L204`). Given the settled choice
  of `adapter-node`, the only way to reach `generateFallback` this sprint
  would be a thin wrapper adapter that delegates to `adapter-node`'s own
  `adapt()` and additionally calls `builder.generateFallback(...)` — a small,
  source-supported option neither draft considers. Absent that, reusing the
  prerendered `/` page as the shell (what the intent proposes, and what both
  drafts do) is a **known-risky substitute**: it renders through the ordinary
  page pipeline, so its `<head>` preloads reference the home route's nodes,
  not just the root layout, and the intent's own Open Questions flag it as
  unverified (`SPRINT-001-INTENT.md:125-127`).
- **`mrmime` itself carries no charset**, and sirv appends `;charset=utf-8`
  (no space) only for the literal value `text/html`; `.js`/`.mjs` get
  `text/javascript` with **no** charset (`libs/mrmime.md:L106-L107, L128-L130`;
  `libs/sirv.md:L128-L130`). Kit's own runtime table (`manifest.mimeTypes`) is
  described as "kit's table, which **extends** mrmime"
  (`kit/build-adapt/static-serving.md:L21`), so "byte-identical to mrmime"
  is itself an incomplete claim — the exact runtime table is kit's superset,
  not raw mrmime.

## Codex draft

**Architectural soundness.** Sound and conservative. Two flat public
constructors (`NewDevProxy`, `NewStaticHandler`) over `http.Handler`, no
`internal/` layering for a sprint this size, explicit `fs.FS` boundaries that
make `embed.FS` and `testing/fstest.MapFS` interchangeable in tests — this
matches the Go implementation notes' own recommendation to keep the handler
"independent of disk paths" (`kit/build-adapt/static-serving.md:L95-L110`
mirrors this almost verbatim). Its production/immutable-miss/document-fallback
priority order (client → prerendered candidate → immutable-miss 404 →
document fallback) matches the pinned request order and the explicit gotcha
that client must shadow prerendered (`kit/build-adapt/static-serving.md:L136-L137`).

**Where it deviates from pinned facts.**
- Static-content-type plan states ".js/.mjs as `text/javascript; charset=utf-8`"
  (SPRINT-001-CODEX-DRAFT.md:104). Pinned fact is that `text/javascript` never
  gets a charset (`libs/mrmime.md:L106-L107`; `libs/sirv.md:L128-L131`). This is
  a concrete, fixable bug in the content-type table as specified, not a
  simplification — it will produce responses that don't match sirv/adapter-node
  byte-for-byte, which is the explicit design goal.
- Falls back to Go's standard-library `mime` package for "the rest" of the
  extensions (SPRINT-001-CODEX-DRAFT.md:104), which is exactly the
  non-determinism `libs/mrmime.md:L99-L124` warns against (Go's table is
  seeded from `/etc/mime.types` and varies host to host, e.g. `.ico` present
  in Go's table but absent from mrmime's). This is a real gap: a plan whose
  whole point is byte-identical static behavior should not mix in a
  host-dependent table for the tail of the extension list.
- Chooses "strong content-derived ETags" instead of sirv's weak
  `size+mtime` ETag. Given the verified `embed.FS` zero-`ModTime()` fact
  above, this divergence is actually the *correct* engineering call — but the
  plan doesn't say why, so a reviewer reading only the "replicate sirv"
  framing would flag it as an unexplained deviation. Worth one sentence in
  the plan.

**Completeness / risk coverage.** Best-in-class on this axis. It is the only
draft that:
  - Explicitly identifies the `/` -reuse-as-fallback risk and gives a bounded,
    source-correct remedy: "switch to Kit's own `builder.generateFallback`
    path; never hand-write the boot document" (SPRINT-001-CODEX-DRAFT.md:272,
    392, 444), which is exactly what `kit/build-adapt/adapter-api.md:L63`
    describes as the supported mechanism. The intent's own bounded fallback
    ("Go synthesizes the document,"
    `SPRINT-001-INTENT.md:36`) is a *weaker* remedy than what Codex proposes.
  - States the immutable-miss/never-fallback rule as a hard, independently
    unit-tested rule (Phase 3, task 1; DoD line, SPRINT-001-CODEX-DRAFT.md:377),
    matching `kit/build-adapt/static-serving.md:L107-L109`.
  - Calls out the missing pinned Vite/Vite+ HMR-path source as a named risk
    ("Vite HMR uses a WebSocket path outside pinned Kit source and Vite+ can
    change it," SPRINT-001-CODEX-DRAFT.md:394), which matches the semantic
    index's own "Known debt" note (`ephemeral/semantic-index/README.md:51`)
    almost word for word — this draft is reading and trusting the index's own
    caveats rather than asserting confidence the sources don't support.
  - Treats the missing-build case as a hard compile error with no
    placeholders (UC5, SPRINT-001-CODEX-DRAFT.md:53-55), which is safer than
    a checked-in-but-empty embed that reports success while serving nothing.

**Phasing / ordering.** Clean dependency order: proxy (pure Go, no frontend
dependency) → static handler (pure Go, `MapFS`-driven) → real Kit app + embed
→ Air/overmind wiring → Playwright-BDD. Each phase's exit gate is a concrete
command (`go test`, `go vet`, a `curl`, a browser check), which is testable
without waiting on later phases. This is the more incremental of the two and
front-loads the riskiest unknown (the fallback-document question) into Phase 4
with an explicit escape hatch, rather than leaving it implicit until browser
testing in Phase 6.

**Feasibility in one session.** High. No `internal/` package split to
maintain, no second MIME table to author and test, no precompression
negotiation to implement and verify against sirv's candidate-list algorithm.
The Files Summary is the smaller of the two drafts and every file has a
single clear owner-phase.

**Definition of done.** Concrete and falsifiable (specific header assertions,
specific status codes, a literal binary-string check for absence of
adapter-node's server banner). One gap: it doesn't quote a plan for the
`Vary` header for non-compressed responses (moot only because it deliberately
disables precompression) — but it also never explains *why* Sprint 001 is
allowed to fully skip the "cheap" precompression the intent explicitly flags
as cheap to add (`SPRINT-001-INTENT.md:127`) rather than deferring it as a
stretch goal within the same phase.

## Gemini draft

**Architectural soundness.** More ambitious layering (`internal/proxy`,
`internal/static`, a dedicated `mrmime.go`), which is a reasonable shape for
where the library is eventually going, but it front-loads structure the
sprint doesn't need yet and that isn't exercised by anything in Phase 1-3
(no second adapter, no second consumer of `internal/static` this sprint).
The request-processing pipeline diagram (SPRINT-001-GEMINI-DRAFT.md:144-174)
correctly orders client before prerendered and hard-404s immutable misses,
matching `kit/build-adapt/static-serving.md:L136-L137, L107-L109`.

**Where it deviates from pinned facts (concrete, fixable bugs).**
- `example/web/pnpm-workspace.yaml` is specified as `onlyBuiltDependencies: []`
  with the comment "Bypasses pnpm minimumReleaseAge check for newly
  published types" (SPRINT-001-GEMINI-DRAFT.md:345-349). Checked against the
  pinned junkyard file it claims to reuse
  (`ephemeral/inspiration/junkyard/app/pnpm-workspace.yaml`, read directly
  this session): the actual key for that purpose is
  `minimumReleaseAgeExclude: ['@types/node@26.4.1']`.
  `onlyBuiltDependencies` is a real pnpm key but controls which packages may
  run install/build scripts — it does nothing for `minimumReleaseAge`. As
  written, this file will not do what the plan says it does, and `vp install`
  is likely to fail on the freshly-pinned `@types/node@26.4.1` exactly the
  way `sources/junkyard/app/toolchain.md:L52` warns about.
- The weak-ETag plan — `ETag: W/"<size>-<mtimeMs>"`
  (SPRINT-001-GEMINI-DRAFT.md:185-187, repeated in the `internal/static`
  handler tasks at :489) — is a **real correctness bug** given the verified
  `embed.FS` zero-`ModTime()` fact above: in production every embedded file
  gets `mtimeMs = 0`, so the ETag degenerates to `W/"<size>-0"`, which (a)
  collides between distinct files of identical size and (b) never
  invalidates across a redeploy that doesn't change a file's byte length.
  This directly undermines the "If-None-Match → 304" acceptance test the
  same draft specifies (SPRINT-001-GEMINI-DRAFT.md:553), since a stale-content
  304 is exactly the failure mode a real ETag should prevent.
- Claims `.html`/`.htm` → `text/html; charset=utf-8` as an "exact" mrmime
  mapping (SPRINT-001-GEMINI-DRAFT.md:193, 453) — mrmime itself has no
  charset entry, and sirv formats it as `text/html;charset=utf-8`
  **with no space** (`libs/mrmime.md:L106-L107`; `libs/sirv.md:L128-L130`,
  which explicitly calls out "no space after `;`" as a byte-level assertion
  in sirv's own tests). The stated goal ("MIME Table Parity... embeds the
  exact MIME mappings from mrmime 2.0.1") is not met by the table as written.
- "Other Static Assets in `client/`... serve with weak ETag and
  `Cache-Control: max-age=0, must-revalidate`" (SPRINT-001-GEMINI-DRAFT.md:161-163)
  is presented as adapter-node parity, but adapter-node's own client `sirv()`
  call passes no `maxAge` at all for generic files — no Cache-Control header
  is sent for them (`libs/sirv.md:L183-L188`). The `max-age=0` behavior is
  what kit's **dev/preview** static middleware does
  (`kit/build-adapt/static-serving.md:L64-L65`), not what adapter-node in
  production does. This should be labeled as a deliberate Go-side design
  choice (which the semantic index's own notes do support,
  `build-output.md:L146-L147`), not as adapter-node parity.

**Completeness / risk coverage.** Mixed. Strengths: it does implement
precompression negotiation (`.br`/`.gz`, brotli-before-gzip) matching sirv's
candidate order (`libs/sirv.md:L77-L83`), and its Open Questions section does
correctly reason through the 200-vs-404 SPA-shell status-code question
(SPRINT-001-GEMINI-DRAFT.md:953-956), reaching the same simplification Codex
reaches but via its own analysis. Gaps: it never engages with the
prerendered-`/`-as-fallback risk at all — no risk-table entry, no open
question — despite the intent explicitly flagging it
(`SPRINT-001-INTENT.md:125-127`) and the pinned `generateFallback` mechanism
being one leaf away in the same taxonomy route
(`kit/build-adapt/adapter-api.md:L63`). Its embed-safety answer (checked-in
`.gitkeep` placeholders so `go build` "succeeds" pre-`vp build`,
SPRINT-001-GEMINI-DRAFT.md:239-248, DoD item 2 at :882) trades a hard,
informative compile failure for a silent, misleadingly-green build that
serves nothing — weaker than Codex's fail-fast UC5, and in tension with the
sprint's own success criteria (a binary that actually serves the app).

**Phasing / ordering.** Reasonable but coarser: four phases instead of six,
with the Go library (proxy + static + MIME table) built together in Phase 2
before the real Kit app exists (Phase 1 only stubs placeholder builds). This
means the static handler's sirv-fidelity claims (weak ETag, precompression,
MIME table) are exercised only against synthetic fixtures until Phase 4,
somewhat later than Codex's plan, which builds the real app in Phase 4 of six
and still has two full phases (5-6) left to catch integration surprises.

**Feasibility in one session.** Lower than Codex's, mainly because of scope:
a hand-maintained `mrmime.go` table, full precompression negotiation with
`.br`/`.gz` sibling lookup, and three internal packages are meaningfully more
surface to implement and test correctly in one sitting than Codex's two flat
files. The MIME-table and ETag bugs above are exactly the kind of mistake
that shows up when a sprint takes on more surface than it can fully verify.

**Definition of done.** Reasonably concrete, but weaker on the "prove it's
really adapter-node-equivalent" axis: DoD item 4 asserts the standalone
binary "serves the SvelteKit app from `embed.FS`" without a check equivalent
to Codex's binary-string absence check for adapter-node's server code, and
DoD item 2's "succeeds... using the checked-in `.gitkeep` placeholders" bakes
the silent-empty-build behavior into the definition of success rather than
treating it as a failure mode to guard against.

## Strongest ideas worth keeping (either draft)

- Codex: bounded fallback to `builder.generateFallback` if the reused `/`
  document doesn't boot deep links cleanly — correct per
  `kit/build-adapt/adapter-api.md:L63`, and worth stating as the shared
  contingency regardless of which draft wins.
- Codex: fail-fast `go:embed` with no checked-in placeholders (UC5) — matches
  the sprint's own bar of "a binary that actually serves the app," not one
  that merely compiles.
- Gemini: precompression negotiation and its correct brotli-over-gzip
  ordering (`libs/sirv.md:L77-L83`) is a legitimate reading of the intent's
  own "precompressed `.br`/`.gz` are cheap" note (`SPRINT-001-INTENT.md:127`);
  if precompression is wanted this sprint, this is the right algorithm to
  copy, once the MIME/ETag bugs above are fixed.
- Neither draft explored the smallest fix for the fallback-shell risk: a thin
  wrapper adapter that still fully delegates output to `@sveltejs/adapter-node`
  but additionally calls `builder.generateFallback(...)` once, matching the
  recipe pinned at `kit/build-adapt/adapter-api.md:L195-L250`. This doesn't
  reopen the adapter-node decision — it is a few lines wrapping it — and
  would remove the single biggest unverified risk both drafts carry forward
  into browser testing.

## Net assessment

Codex's draft is more likely to produce a correct, testable Sprint 001 as
written: its factual deviations from pinned sources are smaller (one
mis-specified content-type rule, one non-deterministic MIME fallback) and it
explicitly engineers around the two biggest real risks (the fallback-shell
mismatch and the silent-empty-embed footgun) with source-grounded mitigations.
Gemini's draft is more ambitious in scope (precompression, layered packages)
but ships two bugs that would surface as real test failures or silent data
corruption — the fabricated `pnpm-workspace.yaml` key (install-time failure)
and the `embed.FS`-incompatible weak-ETag scheme (cache-correctness bug) — and
is silent on the fallback-shell risk the intent itself raised. A merged plan
should take Codex's phasing, fallback contingency, and fail-fast embed
policy, and graft in Gemini's precompression negotiation only after fixing
its MIME table and switching its ETag scheme to content-derived hashing.
