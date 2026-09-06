# Lessons and traps from the junkyard colocation slice

**Purpose.** The corrections, dead ends, and decisions that cost time in the junkyard's
route-colocation work, distilled so skgo does not pay for them twice. Each item names the
source that proves it. Process artifacts (review rounds, adjudication, evidence JSON) are
deliberately not carried over.

All paths relative to the token cache root `ephemeral/inspiration/`.

## Decisions that were right and should be reused

1. **Kit is the only authority for a remote ID.** skgo never hashes anything Kit-facing;
   it reads the ID from Kit's transformed client code and joins on `output#exportName`
   (`junkyard/ephemeral/remote-codegen/fixture/vite/skgo-capture.ts:L6-L9`;
   `route-go-colocation/orientation.md:L32-L33`). The rename case shows the ID changes when
   the module path changes (`proof/run-slice.mjs:L683-L689`), and dev/prod IDs matched only
   because both were captured, not computed (`run-slice.mjs:L595-L605`).

2. **Colocation is purely a Go import-path problem.** Kit and Vite never see the Go files
   or the symlinks; the `.remote.ts` sibling sits at the authored route path and that is what
   Kit hashes (`orientation.md:L41-L43`; `fixture-progress.md:L14-L16`). Do not let the
   Go-side layout leak into the TS module path.

3. **Split "authored path" from "link path" in the generator.** `go list -json` reports a
   symlinked package's `Dir` as the symlink, so `packages.Load` returns link-path files. The
   generator must carry both and use each for the right job (`orientation.md:L55-L70`;
   `worklog/route-go-colocation.md:L8-L11`).

4. **Dot-directory link root + `src/routes/go.mod` boundary.** `./...` skips both for free;
   the boundary is what turns `invalid char '('` into a clean `go list ./...`
   (`orientation.md:L45-L57`, `L102-L105`).

5. **Base32 of the relative path** for link names — injective, no collision table, no
   route-syntax parsing (`core-progress.md:L32-L35`).

6. **Developer-authored jsonschema registrations, never inferred.** Enum/union config is
   in the `//go:build jsonschema` stub file; missing registration is a hard, named error
   (`fixture/backend/remotes/accounts/profile_jsonschema.go:L17-L29`;
   `run-slice.mjs:L330-L348`). Issue text: "Do not infer enum/union configuration or generate
   a second type system" (`route-go-colocation/issue-07.json`, "Agreed direction").

7. **Strict inventory reconciliation in the capture plugin.** If Kit reports a binding the
   Go side did not declare, or a different kind, fail the build (`skgo-capture.ts:L123-L138`).
   This caught real drift and costs nothing.

8. **`ssr = false` + `onMount`** for pages calling Go remotes, and `$app/state` for params
   (`app-overlay/.../+page.ts:L1-L3`, `+page.svelte:L15-L24`). With no SSR in skgo this is
   simply the model.

## Corrections that were paid for

- **Rejected generation leaked scaffolding.** The first strict run showed a *refused*
  `generate` had still created `app/src/routes/go.mod`. The first "fix" relaxed the
  unchanged-tree assertion to tolerate that file; the manager rejected it, the assertion
  went back to strict, and the generator gained rollback of exactly the links and boundary
  the run created (`worklog/route-go-colocation.md:L23-L32`; `core-progress.md:L93-L102`).
  Lesson: setup that must precede validation needs an undo path; never weaken the test.

- **`skgo test -C DIR` read the inventory from the wrong root**, so route tests silently
  did not run; and **appended packages landed after `-args`** and became test-binary
  arguments (`reviews/sol-01.md:L28-L55`). Both are pure argument-vector handling; write
  the Go test first.

- **A universal `load` in `+page.ts` broke `svelte-check`** with `Cannot find module
  './$types'` under the shared `node_modules` symlink layout. Replaced by `$app/state`
  `page.params.id` with an explicit `undefined` guard (`worklog:L34-L41`, `L57-L59`).

- **A negative control that mutated text no longer present** (the `data.id` edit after the
  `load` removal) passed vacuously until the harness asserted the edit actually changed the
  file (`worklog:L61-L64`; `run-slice.mjs:L262`). Lesson: every disposable-edit negative
  must assert `text !== original`.

- **Dev capture raced Vite's initial reload.** Waiting for `load` was not enough; the fix
  was `domcontentloaded` then polling `capture.complete` (`worklog:L66-L70`;
  `run-slice.mjs:L497-L507`).

- **The scratch gopls `invalid type` result was not real.** Against the full generated
  candidate, `gopls definition` resolved `remote.QueryBinding[GetMessageInput, Message]`
  cleanly; the earlier observation came from an incomplete scratch module
  (`fixture-progress.md:L25-L30`). Lesson: test editor tooling against the complete tree,
  and record limitations as observations rather than asserting a clean bill in advance
  (`run-slice.mjs:L899-L902`).

- **Fixture `go.mod` relative `replace`** only works inside the repo; the candidate
  assembler rewrites it (`candidate.mjs:L20-L28`). In skgo, a Go `testdata` module should
  use a `replace` the test rewrites to `t.TempDir()`-independent absolute paths, or the
  test should run inside the repo module.

## Traps by area

### Go toolchain
- `filepath.WalkDir` does not follow symlinks; every walker (scanner, stage copier) needs
  explicit link handling or link targets materialized as real copies
  (`orientation.md:L17`, `L83-L86`; `worklog:L12-L14`).
- `go build ./...` does not compile packages under a dot-directory. A colocated package
  only compiles if something imports it (the registrar) or if it is named explicitly.
- `go tool gen-jsonschema` requires the `tool` directive in `go.mod`
  (`fixture/backend/go.mod:L22`); the provider's `-no-changes` is not read-only
  (`orientation.md:L27-L28`).
- Validated only on Go 1.27.1 / macOS arm64 (`validation/final-01.md:L12-L13`, `L95-L97`).
  Linux CI is the obvious next proof; Windows symlinks are unexplored.

### Symlinks on macOS/Linux
- `realpath(link) === realpath(target)` is the right resolution check
  (`run-slice.mjs:L754-L762`); `readlink` alone gives the relative target string.
- Pruning must confirm the path is still a symlink pointing at the recorded target before
  unlinking, and never walk into the authored directory (`core-progress.md:L104-L108`).
- `treeHashes` skipped symlinks entirely (`harness.mjs:L162`) — fine for content
  fingerprints, but it means a stale link is invisible to that check; `links.json` drift is
  what catches it.

### Vite / Kit
- Plugin must be after `sveltekit()` and `enforce: "post"`; skip SSR transforms; strip
  query strings from ids (`skgo-capture.ts:L55`, `L111-L114`).
- The regex depends on Kit's client transform emitting `.query('<id>')`,
  `.command('<id>')`, `.query_live('<id>')` (`L120`). Re-verify against the pinned kit 3
  source when upgrading; a silent regex miss surfaces as "missing from Kit's transform".
- Production forcing via `this.load` must run from `moduleParsed` (`L60-L63`).
- Kit lazily generates the dev runtime; dev capture completes only after a page load
  (`L74-L77`, `run-slice.mjs:L497-L507`).
- Kit 3 config lives inside `sveltekit({...})`: `adapter`, `appDir`, `paths.origin`,
  `experimental.remoteFunctions`, `compilerOptions.experimental.async`
  (`vite.config.ts:L18-L24`). No `svelte.config.js`.
- `page.params.id` is `string | undefined` (`worklog:L57-L59`).

### Kit build capture
- The capture is only trustworthy with `complete: true` and after reconciliation against
  the inventory; a partial capture must fail Stage B rather than register a subset.
- Remote wire format: query `GET ?payload=base64url(devalue)`; command `POST
  {payload, refreshes}`; errors carry `"type":"error"` in the body
  (`run-slice.mjs:L354-L370`). Any direct-HTTP Go test must speak this.

## What the junkyard built instead of the product (do not repeat)

- A ten-case `.mjs` acceptance runner with exact-case-selection, evidence directories,
  git identity stamping, and per-case logs (`run-slice.mjs:L43-L112`); a shell entry
  script (`check-slice.sh`); progress/orientation/review/validation documents per slice;
  Sol review rounds and independent re-runs. All of the proof value collapses into a handful
  of `go test` functions and one Playwright spec (see fixture-harness.md, verdict section).
- A Node/Go dual-serving proxy ("switchboard") and an adapter-node production lane. With Go
  owning the socket these have no analogue.

## Reusable verdicts (summary)

| Item | Verdict |
|---|---|
| Kit-captured IDs, never predicted | KEEP |
| Authored/link path split | KEEP |
| Dot-dir base32 link tree + `src/routes/go.mod` boundary | KEEP (see link-tree.md for the root-route per-file caveat) |
| Rollback of setup on rejected generation | KEEP, as a Go test |
| Strict capture reconciliation | KEEP |
| Authored jsonschema registrations | KEEP |
| `ssr=false` + `onMount` + `$app/state` | KEEP |
| `skgo test` wrapper | KEEP WITH CHANGES (or avoid by using a non-dot link root) |
| Unimported-module forcing in the capture plugin | KEEP WITH CHANGES (drop if no callers means no handler) |
| `.mjs` harness, shell runner, evidence dirs, switchboard proxy, adapter-node lane | DROP |

## Recipes
- To see the whole seam in one page: `orientation.md:L59-L98`.
- To see the frozen contract a fixture author can build against: `core-progress.md:L7-L108`.
- To see which assertions a strict reviewer refused to relax: `worklog:L23-L32`, `L61-L64`.
- To see the two `skgo test` bugs and their fixes: `reviews/sol-01.md:L28-L55`, `reviews/sol-02.md:L25-L27`.
- To see what "validated" meant and on which platform: `validation/final-01.md:L12-L13`, `L89-L99`.
