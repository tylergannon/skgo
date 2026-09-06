# Sprint 001 Codex Critique

## Overall verdict

Use the Claude draft as the starting point, but do not execute either draft as
written. Claude has the better architectural boundary and is more candid about
unknowns, while Gemini has the better clean-checkout/embed idea and a more
explicit acceptance matrix. Both miss a build-contract blocker: the intent
requires `go build ./example/cmd` from the repository root, but both create a
nested `example/go.mod` without a `go.work` and change the gesture to `cd
example && go build ./cmd`. The merged plan must either add a root `go.work`
covering both modules or remove the nested module.

The one-session bite should be narrower than either draft: first prove the real
dev path through Go + `vp dev` + Air + Overmind, then add the smallest production
static handler needed for the example, then run the same browser scenarios in
both modes. Full sirv/mrmime parity, precompression, range requests, graceful
shutdown, and a broad public facade are not prerequisites for this sprint's
observable promise.

## Source checks that affect both drafts

- Adapter-node really does copy Kit's client output to `build/client` and calls
  `writePrerendered` for `build/prerendered`
  (`ephemeral/inspiration/reference/kit/packages/adapter-node/index.js:27-31`).
  `writePrerendered`, however, flattens `output/prerendered/pages`,
  `dependencies`, and `data` into that destination
  (`ephemeral/inspiration/reference/kit/packages/kit/src/core/adapt/builder.js:216-230`).
  The runtime shell is therefore `build/prerendered/index.html`, not
  `build/prerendered/pages/index.html`.
- Adapter-node's pinned implementation defaults `precompress` to `true` and
  compresses both copied trees
  (`ephemeral/inspiration/reference/kit/packages/adapter-node/index.js:15-16,36-43`).
  A plan that defers compressed-response support must explicitly pass
  `precompress: false`; otherwise it embeds unused `.br` and `.gz` duplicates.
- The proposed CSR-shell mechanism is basically valid. With SSR disabled Kit
  renders an empty body (`ephemeral/inspiration/reference/kit/packages/kit/src/runtime/server/page/render.js:260-261`),
  omits hydration data (`ephemeral/inspiration/reference/kit/packages/kit/src/runtime/server/page/render.js:477-520`), and the client performs an
  `enter` navigation against the current URL (`ephemeral/inspiration/reference/kit/packages/kit/src/runtime/client/client.js:589-600`).
  Client routing matches the pathname against the built route list
  (`ephemeral/inspiration/reference/kit/packages/kit/src/runtime/client/client.js:1852-1874`)
  and produces the root 404 result when no route matches
  (`ephemeral/inspiration/reference/kit/packages/kit/src/runtime/client/client.js:2033-2057`).
- `paths.relative: false` makes built asset URLs root-relative in this setup:
  Kit selects an absolute Vite base when relative paths are disabled
  (`ephemeral/inspiration/reference/kit/packages/kit/src/exports/vite/index.js:899-917`).
  Kit's actual SPA fallback is always absolute regardless of this setting
  (`ephemeral/inspiration/reference/kit/documentation/docs/25-build-and-deploy/55-single-page-apps.md:39-43`),
  but these drafts reuse a prerendered `/` document rather than asking an
  adapter to generate a fallback, so disabling relative paths remains material.
- Kit's dev server derives the request URL origin from the request `Host`
  (`ephemeral/inspiration/reference/kit/packages/kit/src/exports/vite/dev/index.js:528-536,612-616`).
  Replacing it with the Vite target host changes Kit's view of the public
  origin. Preserve the browser-facing host and explicitly allow that host in
  Vite, or document and test an equivalent fixed-origin configuration.
- Sirv's weak ETag is size plus filesystem mtime
  (`ephemeral/inspiration/reference/sirv/packages/sirv/index.mjs:103-118`).
  The installed Go 1.27.1 toolchain's `embed.FS` reports the zero time for every
  file (`/opt/homebrew/Cellar/go/1.27.1/libexec/src/embed/embed.go:219-225`), so
  blindly copying sirv's formula permits collisions between different same-size
  builds. The Go source also confirms patterns are package-relative, exclude
  `_`/`.` names by default, and that `all:` includes them
  (`/opt/homebrew/Cellar/go/1.27.1/libexec/src/embed/embed.go:64-99`).
- The semantic token cache has pinned Kit, sirv, mrmime, devalue, polytype, and
  junkyard evidence, but no pinned Air or Vite+ implementation
  (`ephemeral/semantic-index/README.md:10-17,47-54`). Air config semantics and
  `vp`-specific flags therefore remain implementation-time checks, not facts a
  draft can claim from this source set.

## Claude draft

### Architectural soundness

Strongest ideas worth keeping:

- The dev boundary is appropriately thin: proxy every request this sprint and
  do not invent interception rules before Go loads/remotes/endpoints exist
  (draft lines 208-219). That agrees with the pinned Kit middleware topology.
- Preserving the incoming `Host`, forwarding proxy headers, and treating the
  actual Vite HMR connection as something to verify are the sounder choices
  (lines 208-216, 538-540).
- The draft catches the `all:` embed trap and the zero-mtime ETag problem, then
  chooses content-derived ETags rather than claiming false sirv parity (lines
  276-295, 536-538).
- It explicitly identifies the route-specific modulepreload limitation of
  reusing `/` as a universal shell (lines 297-327). The source supports the
  overall boot path, and the browser deep-link scenario is the right arbiter.
- A Go-owned end-to-end harness with an exact scenario-count check is materially
  stronger than relying on a manual command transcript (lines 452-473).

Weaknesses and corrections:

- **Blocking path error:** Steps 1 and 4 look for
  `build/prerendered/pages/index.html` (lines 380-382, 413-417), contradicting
  both the pinned adapter behavior and the draft's own correct statement at
  line 294. Use `build/prerendered/index.html` everywhere.
- **Fresh-checkout build is unsolved:** `//go:embed` patterns must match at least
  one file. Unlike Gemini, Claude specifies no tracked placeholders or generated
  checked-in build asset, yet its DoD assumes the embed package can be compiled.
  The merged plan must decide whether a clean checkout builds an explicit
  "assets missing" binary, or whether the web build is a mandatory predecessor.
- **Conflicting precompression scope:** the draft defers compressed serving
  (lines 260-263, 393-399) but adapter-node defaults to generating compressed
  siblings. Set `precompress: false` for this sprint or include negotiation and
  tests; do not embed unused compressed copies.
- **Duplicated routing authority:** the hard-coded `RouteMatcher` (lines
  223-255, 413-417) knows SvelteKit routes separately from SvelteKit. It is
  tolerable only as an explicitly example-local two-route expedient. It should
  not be part of the general `static` package API, and the plan should not imply
  adapter-node equivalence. If an HTTP 404 document status is not required this
  sprint, an all-HTML-paths `200` fallback is smaller and avoids the duplicate
  router.
- **Vite host configuration is incomplete:** preserving the public Host is
  correct for Kit, but the plan only says to "ensure" `server.allowedHosts`
  (lines 538-539); it never adds that setting to `vite.config.ts` or proves it.
- **Air does not cover the root library:** its proposed Air root is `example/`
  and it excludes the web and e2e trees (lines 341-356). That can restart on
  `example/cmd` edits but cannot observe `proxy/` or `static/` changes above the
  example module. Narrow the claim to command edits or fix and verify the watch
  root. No pinned Air source exists here, so the exact config must be exercised.

### Completeness

- The separate `example/e2e` package is never installed. `vp install` is run
  only in `example/web` (lines 374-382), while the harness later expects
  `example/e2e/node_modules` (lines 452-467). Add its lock/install gesture and
  Chromium installation, or make one actual workspace that installs both.
- Add `go.work` (or change the module layout) so the exact root build gesture in
  the intent works. Also state explicitly that root `go test ./...` does not
  traverse a nested module.
- Add the missing clean-build asset policy and ignore rules. A test that stats a
  literal hashed `_app/immutable` filename (lines 523-525) is brittle; discover
  at least one matching immutable file instead.
- The full 438-entry MIME generator, copied sirv fixtures, websocket handshake,
  BDD package, and dual-mode orchestration are too much surface to leave all API
  decisions until implementation.

### Phasing and ordering

The initial real Kit build before Go implementation is excellent (lines
368-382). After that, the order is too library-horizontal: it builds MIME/static
parity before demonstrating the primary dev loop. Prefer vertical ordering:

1. scaffold the app and Go entry point;
2. proxy + Procfile + Air, then visibly prove HMR and Go restart;
3. minimal embedded production serving;
4. identical browser scenarios in both modes;
5. only then add cheap HTTP correctness that the scenarios or security boundary
   actually require.

### Risk coverage

This is the stronger risk section of the two drafts: it covers embed exclusions,
ETag instability, modulepreloads, HMR uncertainty, Playwright-BDD API drift, and
origin mismatch. It misses the nested-module/root-build conflict, the unmatched
embed-pattern clean build, the absent e2e install, and the Air watch-root issue.

### Feasibility within one focused agent session

Not feasible as written. The minimum working server and browser proof are
feasible; adding full generated mrmime parity, sirv-derived fixtures, explicit
WebSocket protocol testing, exact-count BDD orchestration, two modules, and
manual HMR/Air proof in the same session is not. Drop MIME-table generation and
route-aware status from this sprint unless the real browser run demonstrates
they are necessary.

### Definition of done

The DoD is observable and correctly distinguishes manual HMR/restart evidence
from automated route behavior (lines 504-525). It still cannot pass from a
fresh checkout as specified, does not prove the required root build command,
does not install the e2e runtime/browser, and does not automatically exercise
Air + Overmind together. Keep the dual-mode browser claims, but name the exact
evidence for HMR and Go restart and require the root build gesture verbatim.

## Gemini draft

### Architectural soundness

Strongest ideas worth keeping:

- The tracked-placeholder concept plus explicit runtime failure for an unbuilt
  asset tree is the best answer either draft gives to clean-checkout embedding
  (lines 222-248), though its ignore rules need correction.
- The static serving pipeline clearly separates immutable assets, version data,
  ordinary assets, prerendered pages, and the SPA fallback (lines 140-174). That
  is a useful design checklist even if most of it should not be implemented in
  Sprint 001.
- It connects adapter-node precompression to `.br`/`.gz` selection and proposes
  direct tests (lines 176-187, 545-557), which is internally more coherent than
  generating but ignoring those files.
- The DoD directly names both execution modes and the no-Node production
  boundary (lines 875-897).

Weaknesses and corrections:

- **Incorrect ETag design:** using `W/"<size>-<mtime>"` over `embed.FS` (lines
  185-187, 488-489) collapses mtime to the same zero value for every embedded
  file. Use a content hash or omit ETags this sprint.
- **Wrong dev origin behavior:** rewriting `Host` to `127.0.0.1:5173` (lines
  201-218, 491-507) makes Kit believe the request's origin is Vite's origin, not
  the browser-facing Go origin. Preserve the original host and configure
  `server.allowedHosts`, then prove HMR through the proxy.
- **Broken Air path composition:** Overmind runs Air from `example/`, Air sets
  `root = ".."`, the build writes `example/tmp/example-server`, but `bin` points
  to `./tmp/example-server` relative to the root (lines 252-283). Those paths do
  not name the same binary. Exact behavior must be verified against the pinned
  Air version, which is not present in the token cache.
- **False graceful-shutdown implication:** the sample waits for a signal but
  never calls `server.Shutdown` or otherwise drains/stops it (lines 610-626).
  Delete this ceremony for the sprint or implement shutdown honestly.
- **The claimed pnpm exception is false:** `onlyBuiltDependencies: []` (lines
  345-349) does not encode the known `minimumReleaseAge` exception. The proven
  file is `minimumReleaseAgeExclude: ['@types/node@26.4.1']`
  (`ephemeral/inspiration/junkyard/app/pnpm-workspace.yaml:1-2`).
- **The placeholder ignore example is incomplete:** after ignoring
  `example/web/build/*`, Git cannot re-include files below ignored directories
  without first re-including those directories (lines 239-247). Use rules that
  preserve the directory path or force-add and document the files.
- **"Sirv-equivalent" is overstated:** the MIME list is only a small subset and
  even gives HTML as `text/html; charset=utf-8`, while pinned mrmime returns
  `text/html` and sirv appends `;charset=utf-8`
  (`ephemeral/inspiration/reference/mrmime/deno/mod.ts:442-446` and
  `ephemeral/inspiration/reference/sirv/packages/sirv/index.mjs:103-118`). The
  draft also omits sirv range behavior
  (`ephemeral/inspiration/reference/sirv/packages/sirv/index.mjs:73-95`) while
  claiming general parity. State a bounded skgo policy instead.

### Completeness

- Like Claude, it never installs the standalone `example/e2e` dependencies or
  Chromium. The web install at line 439 cannot populate a sibling package that
  is not part of a real workspace.
- Its E2E commands use `mise x -- pnpm test` (lines 820-830), while the pinned
  working evidence invokes Playwright's CLI file directly
  (`ephemeral/inspiration/junkyard/.github/workflows/e2e.yml:24-29`). If the plan
  intentionally changes the invocation, it must first establish how pnpm is
  installed and pinned for the e2e package.
- The proposed hydration marker is referenced but no code defines or flips the
  `hydrated` value (lines 393-394, 778-780), so the navigation scenario is not
  implementable from the sketch.
- The 404 scenario asserts only rendered DOM (lines 739-746), not the document
  response status. That is consistent with the draft's eventual all-paths-200
  recommendation, but it must not be described as proving an HTTP 404.
- Add the root `go.work`/module decision, readiness waits, child-process cleanup,
  failed-process diagnostics, and exact expected scenario count.

### Phasing and ordering

The phase boundaries are clear, but they postpone all running-browser feedback
until Phase 4 after a large static server and public API have been designed.
This is risky for a first sprint whose central uncertainty is whether the real
dev and CSR-shell paths work. Prove the proxy/browser slice immediately after
the skeleton, and introduce production static behavior incrementally from the
actual adapter output.

### Risk coverage

The list catches origin, HMR, deep-link asset paths, TypeScript 7, and clean
embedding, but it misses its own zero-mtime ETag collision, invalid Air paths,
incorrect pnpm exception, missing e2e install, nested-module build gesture,
incomplete Git re-inclusion, and the difference between DOM 404 state and HTTP
404 status. It also presents unverified Vite+/Air behavior as settled despite
the semantic index explicitly lacking those pinned implementations.

### Feasibility within one focused agent session

Not feasible. It proposes roughly thirty production/test/config files plus
precompression, ETags, prerender extension lookup, redirects, a MIME registry,
WebSocket testing, graceful shutdown, a three-file public facade, Air/Overmind,
and four BDD scenarios. Several code sketches need correction before they can
compile or run. Keep the placeholder/runtime-validation idea and the acceptance
matrix; cut the rest to the first vertical server slice.

### Definition of done

The mode-by-mode structure is good, but the evidence is weaker than Claude's:
the commands assume manually managed servers, do not assert readiness or
cleanup, do not count tests/skips, and do not automate the no-Node claim. The
DoD also accepts a fresh-checkout binary containing only placeholders without
clearly requiring that running it fail fast, and it never proves the exact root
build command from the intent. Replace command checklists with observable
claims: browser renders and navigates through Go in both modes; HMR crosses Go;
Air restarts Go; a production browser run succeeds while Vite/Node are absent;
unknown-route HTTP status is explicitly chosen and asserted.

## Recommended merge decisions

1. Take Claude's thin proxy, host preservation, content-derived ETag reasoning,
   explicit uncertainty, and Go-owned dual-mode browser harness.
2. Take Gemini's tracked placeholder/runtime validation concept and its explicit
   asset-class test matrix, after fixing the ignore rules.
3. Add `go.work` or collapse to one module so `go build ./example/cmd` works at
   the repository root exactly as required.
4. Standardize on `build/prerendered/index.html`.
5. Set adapter-node `precompress: false` and omit compressed serving in Sprint
   001; add it later with conditional-request tests.
6. Keep the production static handler minimal: exact client files, immutable
   cache policy for successful `/_app/immutable/**`, no asset-to-shell fallback,
   and an HTML shell catch-all. Defer full mrmime/sirv parity.
7. Choose unknown-route document status explicitly. If `404` is required, keep
   routing knowledge example-local and hard-code only the two demonstration
   routes; otherwise assert the client-rendered 404 while documenting the HTTP
   `200` shell response.
8. Install and lock both JS packages, install Chromium, and make the Go harness
   prove non-skipped scenarios against both modes. Separately record direct
   observation of HMR and Air restart through the actual Overmind stack.
