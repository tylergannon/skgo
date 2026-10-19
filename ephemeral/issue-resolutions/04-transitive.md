# Reliable transitive-remote production builds — #116

Source: https://github.com/tylergannon/skgo/issues/116

## Original body

## Version

`github.com/tylergannon/skgo v0.2.5` / `@skgo/adapter` from the `v0.2.5` tag, Kit `3.0.0-next.25`, Vite+ `0.3.0`, TypeScript `6.0.3`.

## What happens

The transitive-remote fix from #106 still fails when Rolldown emits a shared remote chunk and the application's normal `tsconfig.json` excludes its adapter output directory.

The application has this ordinary graph:

```text
src/routes/+page.svelte
  -> src/lib/store.svelte.ts
  -> src/lib/api.ts
  -> src/routes/editor.remote.ts
```

Its SvelteKit-style tsconfig is the same shape as the skgo example/scaffold:

```json
{
  "extends": "$app/tsconfig",
  "include": ["src/**/*", "vite.config.ts", ".svelte-kit/types/**/*"],
  "exclude": ["build", "node_modules"]
}
```

The goja environment successfully emits:

```text
build/.goja/bundle.js
build/.goja/assets/editor.remote-B_MFZRZC.js
```

Then the adapter's second Rolldown fold fails:

```text
[RESOLVE_ERROR] Could not resolve './assets/editor.remote-B_MFZRZC.js' in build/.goja/bundle.js
 - Tsconfig not found in build/.goja/bundle.js at 318..354
```

The emitted file exists and the relative path is correct. Calling the same `vite/rolldown` API over the emitted bundle reproduces the failure on `bundle.write()`.

## Narrow workaround

Making the adapter's intermediate output part of the application's tsconfig fixes the build:

```diff
 "include": [
   "src/**/*",
   "vite.config.ts",
+  "build/.goja/**/*.js",
   ".svelte-kit/types/**/*"
 ],
-"exclude": ["build", "node_modules"]
+"exclude": ["node_modules"]
```

That is adapter implementation detail leaking into every application config, and it contradicts the current skgo example/scaffold, which both exclude `build`.

## Expected

The adapter's final fold resolves its own emitted chunks independently of whether the application tsconfig includes `build/.goja`, and the Tractor-shaped graph above is covered by a regression test.


## Comment by tylergannon — 2026-10-18T16:58:32Z

Source: https://github.com/tylergannon/skgo/issues/116#issuecomment-6252604642

**Triage — 2026-10-18**

Checked against current [`main` at `920b6aca`](https://github.com/tylergannon/skgo/tree/920b6aca738a65d6c046a67f4db2373596fafd03). Classification: `bounded-hard`. 

The affected production mechanism still exists: internal/adapter/skgo-adapter/env.js emits an ESM environment and folds its entry through a second app-resolved Rolldown call. That fold does not explicitly isolate tsconfig resolution; current example/scaffold configs still exclude build. The Vite/Rolldown pins have changed considerably since v0.2.5, so this is a candidate regression to reproduce on the current pin, not a confirmed still-failing build. Retain the transitive shared-remote graph and build-excluded tsconfig as the bounded repro; do not close solely because development no longer uses goja.

<!-- skgo-issue-triage-20261018-920b6aca -->

## Cache annotations and execution contract

Cache root: `/Users/tyler/src/skgo-token-cache`; routed from `index/README.md` through `index/dev-framework-runtime.md:47-54` and `index/upstream-docs/kit-reference.md:124-126`. Historical pins and historical workarounds are not current regression results.

**Goal.** A normal application graph `+page.svelte -> store.svelte.ts -> api.ts -> editor.remote.ts` builds for production even when its normal tsconfig excludes `build` and does not include `build/.goja`. When Kit emits shared remote chunks, skgo's final fold resolves those already-emitted JavaScript chunks independently of application tsconfig inclusion/exclusion. No application-config workaround is acceptable.

**Cache findings.** `repos/architecture-20261015/skgo-dev-868c5db/internal/adapter/skgo-adapter/env.js:351-413` preserves the ESM environment followed by a one-entry Rolldown fold. Its fold at lines391-401 sets neutral platform and import.meta lowering without isolating tsconfig. `index/dev-framework-runtime.md:53` routes the cached Kit remote plugin's server metadata initialization and client value introspection; remote semantics must continue using Kit's actual transformed graph. Cached `repos/dev-experience-20261015/frameworks/kit-3.0.0-upstream/documentation/docs/98-reference/20-$app-tsconfig.md:5-23` documents the ordinary Kit config surface, not an obligation to enroll adapter intermediate files. No indexed material consulted establishes that the original excluded-output failure still reproduces at current pins.

**Current pinned mapping.** Installed Kit is **3.1.0**; app Vite resolves to **@voidzero-dev/vite-plus-core1.3.0**, including its own bundled Rolldown. Current `example/web/node_modules/@sveltejs/kit/src/exports/vite/plugins/remote.js:138-149` still emits dedicated remote entries in production server environments. The bundled Rolldown input option at `example/web/node_modules/vite/dist/rolldown/shared/define-config-BLptERXo.d.mts:4183-4189` explicitly controls TypeScript config resolution/usage, defaults to true, and accepts boolean or filename. This is an upstream-owned facility to inspect, not a need for a new resolver. Preserve app-resolved Vite/Rolldown identity; changing application source compilation or introducing another bundler is outside the issue. Current `example/web/tsconfig.json` still excludes build.

**Independent proof.** A focused Go test must run a real production build at installed pins with the exact transitive store/api/remote graph and unchanged build-excluded tsconfig. Establish that the relevant build actually emits and imports a shared remote chunk; a graph that no longer code-splits cannot alone close this historical case. Require the final folded artifact to be usable by the production Go/goja handler and render an independently supplied literal Go remote result through that graph, while generated JavaScript remote bodies remain throwing stubs. This anchors success in observable output, beyond a command exit or matching source option. A bounded direct fold contract can isolate the tsconfig edge if current optimizer behavior makes shared chunks difficult to retain, but cannot replace the real graph build. Reuse build output for all corresponding assertions; no full CI/browser loop is required for bytes/SSR claims unless an additional client claim is introduced.

**HITL.** Executable now; no unanswered design choice. First reproduce/qualify at current pins, then repair only a verified remaining gap or retain a load-bearing regression if upgrades already repaired it. Preserve normal app tsconfig and production goja support. Development's native Kit renderer does not resolve or exclude this production issue.
