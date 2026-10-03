# SvelteKit compatibility journal

This journal records upstream releases reviewed for skgo, the Go contracts they
can affect, and the evidence for each adoption. A dependency pin or a successful
build alone does not establish compatibility. Go owns HTTP, server loads, remote
functions, actions and dynamic SSR; Goja runs the renderer. Node is build-time
only. Browser behavior and prerender output also depend on upstream Kit code.

## Maintaining the journal

For every reviewed or adopted Kit release:

1. Verify the exact stable version and publication date against the official npm
   registry and SvelteKit release. Record links, the previous reviewed version,
   and whether the release is merely reviewed, locally validated, adopted on main,
   or published. Do not substitute a prerelease silently.
2. Compare the installed source or official tarballs across those exact versions.
   Review the release notes too, but distinguish aggregate major-release history
   from changes since skgo's actual baseline. Review routing and redirects,
   manifests/build output, server fetch, cookies, data/actions, remote functions,
   serialization, CSP/SSR and browser lifecycle behavior.
3. For each relevant change, record the Go impact, the code and contract tests
   added, or the source-backed reason no Go change is needed. Separate inherited
   upstream behavior from a Go implementation and from an unproved edge case.
4. Reconcile stable dependencies and peers across examples, adapter and generated
   apps. Record retained pins and narrowly scoped peer exceptions with reasons.
5. Validate production and dev against the real Go server, generated TypeScript
   and JSDoc apps, and browser interactions. Keep fixture assertions independent
   of production helpers. Check that regressions fail when their fix is removed.
   Record failures, skips and coverage gaps honestly; inspect browser evidence.
6. Update this journal and the README status when the release is adopted. Recheck
   combinations with separately pending fixes before integration. A later main
   merge or publication requires updating the status, not assuming it happened.

This is a manual maintenance procedure; it creates no scheduled task.

## 3.0.0 — reviewed 2026-10-03

**Status:** locally validated candidate; ready for review. No merge, publication
or release has occurred. The review does not claim full Kit parity.

**Verified release:** GitHub published `@sveltejs/kit@3.0.0` on
2026-10-01 at 17:21:04 UTC; npm published it at 17:22:34.593 UTC.
Sources: [official release](https://github.com/sveltejs/kit/releases/tag/%40sveltejs%2Fkit%403.0.0)
and [official registry](https://registry.npmjs.org/@sveltejs/kit).
The [stable source tarball](https://registry.npmjs.org/@sveltejs/kit/-/kit-3.0.0.tgz)
was compared with
[3.0.0-next.28](https://registry.npmjs.org/@sveltejs/kit/-/kit-3.0.0-next.28.tgz),
the actual prior pin. Registry timestamps, peer constraints and tarball integrity
are recorded in [registry.json](ephemeral/js-dependencies/registry.json).

**Local baseline:** published PR235 head `e9081881810928308a052854538545104c6f089d`
on branch `codex/stable-js-dependencies`. Main was inspected at
`69cf454f1932bd452a11ba2da716ddd1782c4882`. The unpublished release repair
`8ee7a34` and separate generator change `32a874e` were not incorporated. Their
combined behavior must be validated during any later integration. The baseline's
native-private-field performance fix from `972ea684` remains intact.

### Changes and backend assessment

| Upstream change or reviewed contract | skgo impact and local decision | Evidence and limits |
| --- | --- | --- |
| Adding a trailing slash now prefixes relative redirects with `./`, so a colon in the final segment cannot be interpreted as a URL scheme (`src/utils/url.js`). | Updated `static.go` and `endpoint.go` to share this behavior, preserving query strings. | Real-handler regressions `TestAPrerenderedColonPathRedirectStaysOnTheSameOrigin` and `TestABasedRootRedirectCannotTurnAColonIntoAURLScheme` assert literal `./foo:bar/?q=1`, status and destination body. Both pass; an independent copy using the old behavior fails both. |
| Manifest wrapper still imports and calls `create_server(manifest)`; this wrapper is unchanged from next.28. | Fixed an existing adapter bug that stripped only the older `Server` constructor. Extract the manifest without evaluating Kit's server or invoking lazy nodes; reject unknown wrappers before evaluation. | `internal/adapter/manifest_test.go` uses a literal upstream-shaped wrapper, a server that throws if evaluated and a lazy node that must remain uncalled. Positive extraction and unknown/old-wrapper rejection pass; independent server-execution mutation fails. This is a prior bug uncovered by the release review, not a new 3.0.0 delta. |
| Diagnostics moved to `src/messages`; error names, text, links and sometimes identity changed. | Updated the two independent literal CORS error expectations in `example/universal_fetch_test.go` to released `load_fetch_cors` diagnostics. | Actual renderer/fetch contracts exercise missing and incorrect allow-origin headers. No claim that every diagnostic is unchanged. |
| `remote-functions.js`, `server/data/index.js`, `server/csrf.js` and `core/generate_manifest/index.js` are byte-identical to next.28. | No new Go wire migration identified in these files. Reserved `x-sveltekit-*` rejection and single-flight behavior already belong to the baseline. | Exact source comparison plus existing server/browser contracts; this statement covers these files, not all Kit server behavior. |
| Special self-origin asset, prerender, reroute and fallback fetches now use `redirect: 'manual'` to avoid an origin-spoofed network redirect. | Go serves the corresponding owned files/routes in process (`fetch.go`, `remote.go`), without Kit's network fallback. No equivalent Go redirect-following code was identified there. Generic external HTTP redirect behavior is retained. | Source ownership review and existing fetch destination/credential contracts. No adversarial redirect-sink-count test was added, so that security property is not claimed as proved. |
| Action-result locations starting `//` are prefixed with `/.`. | No Go port: skgo rejects non-clean double-slash request paths before action dispatch. Its supported path policy differs from this upstream edge case. | Source inspection; double-slash action parity remains unsupported/unproved. |
| CSP SHA-256 moved to awaited WebCrypto; prerender cache-control attributes are escaped. | Go computes dynamic CSP using native SHA-256; Kit's build owns prerender output. No Go API change was identified. | Existing `csp_test.go` checks independent literal hashes, nonces and directives; production build exercises normal prerendering. Hostile quotes/ampersands in prerender cache-control were not tested. |
| Cookie option handling became optional-safe. | Go uses typed cookie options and has no equivalent destructuring of an undefined JavaScript object. No migration identified. | Source/type review and existing cookie contracts, not exhaustive option equivalence. |
| Matcher validation moved; directory traversal now uses dirents with a symlink fallback. | Manifest/routing remains build-owned; no change identified for valid generated routes. | Generated apps and route suites exercise normal paths. Malformed parameter and symlink edge cases were not qualified. |
| Client navigation focus/scroll, bfcache, same-location origin comparison, keyed remote-form cleanup and prerender-resource finalization changed. | Inherited upstream browser code. No Go port identified. Dev browser qualification exposed hydration readiness and remote-form completion races: critical clicks now wait for an actual Svelte delegated handler; the example remote form disables submission while pending. | Tests retain single clicks and literal operation counts. Shared-form receipt checks wait for the real pending state to finish before reusing inputs. Normal navigation/forms are exercised; bfcache/focus/finalization edge cases are unproved. |

### Dependency selection

Inventory covers all four package manifests and all three pnpm locks:
`example/web`, `example/e2e`, `internal/adapter`, and `internal/sv`; the adapter
has peer declarations and no standalone lock. Generator/advice pins, CI and
package-manager configuration were checked too. Locks were regenerated from
clean installations rather than keeping stale transitive selections.

| Package/family | Selected stable version | Retained constraint or migration |
| --- | --- | --- |
| SvelteKit / adapter peer | `3.0.0` / `^3.0.0` | Replaces `3.0.0-next.28`; installed source is the authoritative adapter reference. |
| Svelte; Vite plugin; svelte-check | `5.57.1`; `7.3.1`; `4.7.6` | Current verified stable versions. |
| VitePlus / aliased Vite core | `1.0.0` / `1.0.0` | Exports Vite `8.3.1` and Rolldown `1.2.11`. Standalone latest `8.3.2` / `1.2.12` are not substituted into the managed runtime: module identity matters to Kit's SSR environment. Exact alias override avoids duplicate Vite cores. |
| Managed Vitest family | `5.0.1` | Standalone latest is `5.0.3`; generated direct Vitest/browser packages follow VitePlus's bundled family to keep compatible shared instances. |
| vitest-browser-svelte | `3.1.0` | Stable sv emits `2.1.1`, whose peer requires Vitest 4. Version 3.1.0 supports managed Vitest 5 and passed real Chromium tests. |
| TypeScript | `6.0.3` | Retained despite stable `7.0.2`: Kit requires `^6.0.0`, and svelte-check accepts only 5/6. |
| Node types | `24.19.1` | Updated within 24.x to match pinned Node `24.21.0`; latest `26.6.4` describes another runtime major. |
| Playwright; playwright-bdd; yaml | `1.63.0`; `9.2.1`; `2.9.1` | Current verified stable versions retained. |
| sv; sv-utils; tsdown | `1.0.1`; `1.0.0`; `0.23.0` | Stable-only generator selection replaces prerelease selection; adapt `pnpm.allowBuilds` to `{ cwd, packages }`. Rebuilt the distributed addon. tsdown's separate lock resolves Rolldown `1.2.12`. |
| Storybook / create-storybook / SvelteKit integration | `10.6.1` | Upgrade matching packages together. Storybook's optional VitePlus peer range is stale; permit only `storybook@10.6.1>vite-plus: 1.0.0` after an actual Storybook build passes. |
| pnpm | `12.8.1` | Update example/tooling/CI pins together. The aliased Vite package's own version is 1.0.0, so its narrow peer exception records that version, not exported Vite 8.3.1. |

Sources for each selection are the official registry URLs and metadata in
`registry.json`; managed tool versions were verified in installed packages and
against the [VitePlus 1.0.0 release](https://github.com/voidzero-dev/vite-plus/releases/tag/v1.0.0).
The peer exceptions are explicit compatibility decisions, not upstream range
satisfaction. Future version changes require reviewing or removing them.

### Validation

All runs use this isolated branch, bounded Go concurrency (`GOMAXPROCS=2`,
`GOFLAGS=-p=2`) and two main browser workers. Source-edit dev scenarios must
finish before the final production build: they touch generated stubs and would
otherwise invalidate the independent build-freshness check.

- Adapter manifest contracts, native-private-field syntax-lowering contracts,
  fresh production runtime startup and 500-document renderer retention passed.
  Independent mutations prove the extraction/lowering regressions fail. The
  native candidate retained 6,444,872 bytes; lowering its private fields in a
  temporary production copy retained 72,350,880 bytes and failed the 33,554,432
  byte ceiling. Performance code and its assertions remain unchanged.
- Actual CLI-created TypeScript minimal and JSDoc demo projects completed Go
  generation and production builds. Both passed `pnpm check` with zero errors
  and warnings, and `pnpm test` with two tests (unit and Chromium), no skips.
  A wrong literal heading failed the generated TypeScript browser test. The
  JSDoc demo's Storybook build passed; a fresh generated app's peer check passed.
- The generated JSDoc Go binary served SSR with Node excluded from its runtime
  PATH. A Chromium command interaction updated the exact greeting and write
  count from 0 to 1; its screenshot was inspected.
- Main-example browser suite: all 174 scenarios passed in dev (4.8 minutes)
  and all 174 passed in production (1.2 minutes), with no skips. Production ran
  against the Go binary with Node excluded from PATH. A separate cold-page
  Chromium inspection verified Home and an enhanced remote form, the literal
  receipt `Remote Go form received Tyler`, re-enabled submission and no page
  errors. Both screenshots were inspected.
- Example `pnpm check` passed with zero errors and warnings; `just vet` passed.
  `just test` passed for both modules, including production SSR, retention with native
  private fields and frontend build freshness. No test toolchain was missing;
  the only conditional skip in the suite is Windows-only and was not applicable
  to this macOS run. The final embedded frontend was rebuilt after both modes'
  source-edit scenarios restored their sources.

Earlier attempts exposed stale literal CORS diagnostics, an existing manifest
extraction bug, browser readiness/form-completion races, and stale build inputs
after dev generation. Those were fixed or resolved before the final green runs;
no known failing contract was waived. Executor disconnection interrupted an
earlier run; resumed qualification used fresh task-owned servers.

Remaining limits are listed per contract above. Ordinary successful flows do
not prove every upstream edge case. Adoption on main, integration with separately
pending generator/release fixes and any publication remain separate work.
