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

**Adoption:** this change moves skgo's committed toolchain from next.28 to
released Kit 3.0.0, with main integration through
[PR #237](https://github.com/tylergannon/skgo/pull/237). Runtime qualification on
released base `bfea45a` is recorded below. This review does not claim full Kit
parity. Publication uses the existing Release gate: both browser modes qualify
the merged commit before npm publication and tagging.

**Consumer compatibility:** the adapter now requires stable Kit `^3.0.0`, the
addon requires sv `^1.0.1`, and new project creation requires VitePlus exactly
1.0.0. Existing prerelease/older-bootstrap consumers must upgrade those tools.
The Conventional Commit feature increment signals these changes as a pre-1.0
minor release (`v0.16.0`), rather than a patch or a premature 1.0 release.

**Verified release:** GitHub published `@sveltejs/kit@3.0.0` on
2026-10-01 at 17:21:04 UTC; npm published it at 17:22:34.593 UTC.
Sources: [official release](https://github.com/sveltejs/kit/releases/tag/%40sveltejs%2Fkit%403.0.0)
and [official registry](https://registry.npmjs.org/@sveltejs/kit).
The [stable source tarball](https://registry.npmjs.org/@sveltejs/kit/-/kit-3.0.0.tgz)
was compared with
[3.0.0-next.28](https://registry.npmjs.org/@sveltejs/kit/-/kit-3.0.0-next.28.tgz),
the actual prior pin. Registry timestamps, peer constraints and tarball integrity
are recorded in [registry.json](ephemeral/js-dependencies/registry.json).

**Baseline:** reviewed from PR235's published head, then reconciled onto released
main `bfea45a9dc5be011f9e4f37204f3b37eea350fde`, which includes the release repairs
and PR236 generator optimization. No separate pending change was cherry-picked.
The native private fields from performance fix `972ea684` remain intact.

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
| Managed Vitest / browser-playwright family | `5.0.1` | Standalone latest is `5.0.3`; generated direct Vitest/browser packages follow VitePlus's bundled family to keep compatible shared instances. |
| vitest-browser-svelte | `3.1.0` | Stable sv emits `2.1.1`, whose peer requires Vitest 4. Version 3.1.0 supports managed Vitest 5 and passed real Chromium tests. |
| TypeScript | `6.0.3` | Retained despite stable `7.0.2`: Kit requires `^6.0.0`, and svelte-check accepts only 5/6. |
| Node types | `24.19.1` | Updated within 24.x to match pinned Node `24.21.0`; latest `26.6.4` describes another runtime major. |
| Playwright; playwright-bdd; yaml | `1.63.0`; `9.2.1`; `2.9.1` | Current verified stable versions retained. |
| sv; sv-utils; tsdown | `1.0.1`; `1.0.0`; `0.23.0` | Stable-only generator selection replaces prerelease selection; adapt `pnpm.allowBuilds` to `{ cwd, packages }`. Rebuilt the distributed addon. tsdown's separate lock resolves Rolldown `1.2.12`. |
| Storybook / create-storybook / SvelteKit integration | `10.6.1` | Upgrade matching packages together. Storybook's optional VitePlus peer range is stale; permit only `storybook@10.6.1>vite-plus: 1.0.0` after an actual Storybook build passes. |
| Extra generated-template packages | `@storybook/addon-svelte-csf 5.1.5`; `@sveltejs/enhanced-img 1.0.0`; `playwright 1.63.0` | Actual generated installations match verified stable registry versions. Upstream sv ranges resolve these versions; no additional pin was needed. |
| pnpm | `12.9.1` | Released after initial qualification; official registry/release verified on 2026-10-04. Update example/tooling/CI pins together. The aliased Vite package's own version is 1.0.0, so its narrow peer exception records that version, not exported Vite 8.3.1. |

Sources for each selection are the official registry URLs and metadata in
`registry.json`; managed tool versions were verified in installed packages and
against the [VitePlus 1.0.0 release](https://github.com/voidzero-dev/vite-plus/releases/tag/v1.0.0).
The peer exceptions are explicit compatibility decisions, not upstream range
satisfaction. Future version changes require reviewing or removing them. `skgo new` requires
bootstrap VitePlus exactly 1.0.0, and the addon refuses any other installed
VitePlus version. This deliberately prevents an older global executable from
silently generating its older managed family or a newer unqualified combination;
the README names the required version. Plain Vite projects have no VitePlus
version prerequisite. VitePlus itself emits broader wildcard Vite peer rules in
generated projects; those upstream rules are retained there. The example's own
alias exception is exact. A successful peer check includes those explicit
exceptions and is not a claim that upstream semver ranges accept the aliases.

### Local validation on the combined released-main base

All browser runs used two workers and Go runs used `GOMAXPROCS=2` and
`GOFLAGS=-p=2`. Dev source-edit scenarios completed before the final production
build so restored files could not invalidate the frontend freshness check.

- Main-example browser suite: **174 dev scenarios and 174 production scenarios
  passed**, with no skips, on the combined `bfea45a` base. Production ran against
  a task-owned Go binary with Node excluded from its runtime PATH. Logs:
  `final-dev-bdd.log` and `final-prod-bdd.log` under
  `ephemeral/js-dependencies/logs/`.
- Fresh production build, `just test` for both modules, `just vet`, and example
  type checking passed. The Go suite includes literal handler contracts,
  startup of the production SSR runtime, 500-document renderer retention with
  native private fields and frontend build freshness. Type checking reported
  zero errors and warnings. The only platform-conditional skip is Windows-only
  and was not applicable on this macOS host. Logs: `local-final-prod-build-fresh.log`,
  `local-final-go-test.log`, `local-final-vet.log`, `final-svelte-check.log`.
- Real CLI-created TypeScript minimal and JSDoc demo projects using VitePlus
  1.0.0 were regenerated with the merged Go generator, production-built and
  type-checked (zero errors and warnings); each passed two unit/Chromium tests
  with no skips. The **same JSDoc app** also passed its Storybook build and
  `pnpm peers check` with the narrow exact peer exception. Logs:
  `final-generated-ts-*` and `final-generated-jsdoc-*`.
- After adding the bootstrap prerequisite, the real CLI rejected installed
  global VitePlus 0.3.3 before it created an app, and fresh VitePlus 1.0.0
  TypeScript and JSDoc app creation succeeded. Literal Go fixtures also reject
  prerelease, newer patch and newer major versions before creation; an
  independent mutation without the prerequisite fails those regressions. Both
  post-fix fresh apps also passed type checks (zero errors/warnings), two
  unit/Chromium tests each and clean peer checks. Logs: `local-old-vp-rejection.log`,
  `local-qualified-ts-*`, `local-qualified-jsdoc-*` and
  `local-bootstrap-independent-*`.
- An independent temporary production mutation lowering native private fields
  failed the fixed 33,554,432-byte retention ceiling (72,350,880 bytes retained,
  versus 6,444,872 in the native candidate). Manifest extraction and redirect
  regressions also fail independent mutations. This negative proof was run
  before released-main reconciliation; the performance code and tests are
  unchanged from that candidate and released main, and the final full suite
  rechecks retention.
- Fresh Chromium inspection of the final local production build exercised a
  sign-in command and enhanced remote form with a distinctive literal input.
  It verified Go's resulting session and receipt, re-enabled submission and no
  page errors. Home and Actions screenshots were captured and inspected;
  capture details are in `local-final-browser-inspection.log` (2026-10-03
  15:17:34–15:17:36 UTC). The images are
  `ephemeral/screenshots/stable-js-final-local-home.png` and
  `stable-js-final-local-actions.png`; they visibly show the literal input
  `Tyler final local 2026-10-03`, distinct from earlier captures.

The working-tree code checked above was committed as `5d3e1a0`. The independent
[whole-change review](ephemeral/reviews/20261003-stable-js-main-round-03.md)
ran `just test` again at that committed head (21 packages passed), verified the
addon distribution against a fresh build, and inspected both screenshots. Its
outcome was **only nitpicks remain**. No Go or browser implementation changed after those runs; later edits clarify
the installation command and this record, and update the package manager as
described below. Committing already checked content alone does not require
repeating the checks.

An old global VitePlus bootstrap, stale CORS expectations, an existing manifest
extraction bug, browser readiness/form-completion races and stale build inputs
were found and resolved; no known failing contract was waived. The manual
inspection initially used an incorrect bundle path and guessed a remote input's
HTML name; the corrected run used the actual artifact and accessible control.
The readiness fixture intentionally depends on pinned Svelte's delegated event
symbol; a future event implementation change requires reviewing that fixture.
Dev form readiness uses a handler in the same component and is not a general
form-readiness API. Executor
interruptions delayed local qualification but are not passing evidence.

Remaining coverage limits are listed per contract above. The published version
and merged commit are determined by the existing
[Release gate](https://github.com/tylergannon/skgo/actions/workflows/release.yml);
it qualifies and tags the exact main SHA, without a separate manual tag or
publication path.

The 2026-10-04 registry recheck found only pnpm had advanced since this review
(to 12.9.1). Its [official release](https://github.com/pnpm/pnpm/releases/tag/v12.9.1)
is stable and fixes frozen catalog-peer installs, trust checks and lifecycle
handling. All three frozen installs with 12.9.1 passed; only the lockfiles' package-manager
documents changed, and the application dependency graphs remained byte-identical.
Exact-head CI and the normal release gates will qualify this update; previous
runtime evidence covers unchanged application code.

The from-scratch [CI run on `4dbcc11`](https://github.com/tylergannon/skgo/actions/runs/37167486860)
passed install, build, vet and all Go tests with pnpm 12.9.1. The local frozen
installs above were up-to-date checks, not clean relinks; CI is the clean-install
evidence. Review caught the local embedded build predating the refreshed
manager lockfile. A local `pnpm run build` attempt then failed because the
example script named `vite`, while the managed installation exposes `vp`.
The package's `build` and `dev` entries now invoke the installed `vp` built-ins.
The canonical `ORIGIN=http://127.0.0.1:18018 just build` completed and refreshed
`build/`, including `build/skgo.manifest.json` and `build/ssr/bundle.js`, so Go's
embedded frontend freshness is current. Direct project-local `vp dev` also
started on `127.0.0.1:51731`; its read-only `@vite/client` response identified
VitePlus core 1.0.0. Final local `just test` passed in captured tool output;
the saved final dev/Go log files contain the earlier restricted-socket failures,
not those successful retries. The exact-head [CI run on `f572a58`](https://github.com/tylergannon/skgo/actions/runs/37168579712)
passed install, canonical build, vet and all 21 Go test packages; it is the saved
final Go/build evidence. Final PR-head CI remains the merge gate, and the
merged-head browser gates remain the release gate.
Release receipts are the workflow, npm versions and annotated Go tag.
