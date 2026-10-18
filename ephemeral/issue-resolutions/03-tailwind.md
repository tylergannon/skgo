# Tailwind integration without lost frontend state — #350

Source: https://github.com/tylergannon/skgo/issues/350

## Original body

## Outcome

A developer can add Tailwind to a new or existing SKGO application through `skgo add`, using the existing add-on/plugin contract. The integration should configure Tailwind correctly for an application that colocates Go with SvelteKit frontend source. Extract the reusable setup from the Gimbal lesson; do not require each app to discover it independently.

`skgo add` and `templateapi` already exist in current main. Reuse that foundation and the upstream Tailwind/SvelteKit integration rather than inventing a separate plugin system or forking Tailwind. Related framework/template work: #293.

## Concrete failure motivating this

[tylergannon/gimbal#605](https://github.com/tylergannon/gimbal/issues/605) was isolated on Gimbal `f8372914` with SKGO `02e6417` and Tailwind `4.3.3`. Its unscoped `@import "tailwindcss"` let the Tailwind scanner include 79 backend Go files under `web/`. Editing `web/development.go` made the Tailwind Vite plugin send a full browser reload before Go replacement was ready, discarding an unsent draft and an open workspace tab. A captured runtime stack identified Tailwind as the sender. Plain Kit refresh and an equivalent Go edit outside the frontend root retained state.

Gimbal's local repair is Tailwind's public `@source not "../**/*.go";` directive relative to its `web/src/app.css`. The pinned scanner then excludes all Go files while retaining the same 336 Svelte/TypeScript/HTML sources. The reusable add-on must derive the correct scope from the application's actual stylesheet/layout rather than blindly copying Gimbal's relative path.

## Required behavior

- Install/configure compatible Tailwind dependencies and the standard Vite/CSS integration through the supported VitePlus-owned tooling.
- Exclude backend Go files from Tailwind content scanning, including Go files at the frontend root and colocated route/remote Go under frontend source. Preserve frontend class discovery and ordinary CSS/Svelte HMR.
- Preserve existing handwritten configuration/styles when applied to an existing app; repeated application must not duplicate setup. Report genuine conflicts through the existing add-on contract.
- Keep Go files visible to SKGO and application source watchers. Do not fix this by globally ignoring Go in Vite, suppressing arbitrary frontend reloads, adding application persistence, or adding Tailwind-specific behavior to the Go replacement protocol.
- Keep the add-on optional: ordinary applications need no Tailwind runtime dependency. Compose it through the same add-on operations used by application templates.

## Proof

Use a fresh SKGO app and an existing app with edited CSS. In the running application, leave a literal unsent draft and another local view open, edit Go both at the frontend root and beside a route, and observe the correct new Go behavior without a document reload or lost frontend state. Then add/change a frontend utility class and CSS rule and observe their styles update normally. Verify repeated add-on application and preservation of the edited existing stylesheet. Successful installation or a green build alone does not prove the development behavior.

Track with [Dev server #349](https://github.com/tylergannon/skgo/issues/349). This issue is the reusable integration follow-up; Gimbal #605 owns its immediate application repair.


## Comment by tylergannon — 2026-10-18T16:57:35Z

Source: https://github.com/tylergannon/skgo/issues/350#issuecomment-6252595032

**Triage — 2026-10-18**

Checked against current [`main` at `920b6aca`](https://github.com/tylergannon/skgo/tree/920b6aca738a65d6c046a67f4db2373596fafd03). Classification: `bounded-hard`. 

Current add-on/template infrastructure exists (internal/templates and templateapi); the requested reusable Tailwind integration remains a separate follow-up. The endpoint is explicit, including existing-app preservation, idempotence, Go source exclusion and ordinary frontend HMR. Requires real app/browser qualification and content-scope correctness; not a blind copy of Gimbal CSS.

<!-- skgo-issue-triage-20261018-920b6aca -->

## Cache annotations and execution contract

Cache root: `/Users/tyler/src/skgo-token-cache`; entered through `index/README.md`, then `index/upstream-docs/cli-sv.md:95` and `index/promise-audit.md`. The cache holds historical upstream setup and historical add-on observations, not proof of the new integration.

**Goal.** Optional `skgo add` Tailwind setup, composable through existing template operations, produces ordinary Tailwind/Vite/SvelteKit integration with Go-safe scanning. Existing handwritten styles/configuration survive, repeated application is idempotent, genuine conflicts use the existing add-on error/result contract, and backend edits preserve frontend state while frontend styles retain normal HMR.

**Upstream setup.** Cached upstream sv 1.1.1 implementation is `repos/svelte-docs-20261017/cli-sv-1.1.1/packages/sv/src/addons/tailwindcss.ts:38-99`: declare Tailwind plus its Vite plugin dependencies, prepend the Vite plugin, add the CSS import, and import the actual stylesheet from the root layout using a computed relative path. Its historical versions (`^4.3.0`) are context; the builder must qualify compatible installed versions. The description is at `documentation/docs/30-add-ons/50-tailwind.md:13-18` under the same cached source tree. This supports using the established integration, not a new plugin system. Installed worktree Kit remains **3.1.0**, and the tracked current-Kit instructions apply; the builder must map current source itself.

**Gimbal lesson, checked live.** [Gimbal #605](https://github.com/tylergannon/gimbal/issues/605) now records repair [PR #606](https://github.com/tylergannon/gimbal/pull/606), squash `43428ebd5dff1b3db93db093c77d93fa0d409e3f`. Tailwind 4.3.3 scanned 79 Go files and sent the full reload; the public `@source not` directive excluded those while retaining the same 336 frontend source files. It records a separated released-SKGO route-only Go edit retaining literal draft `DRAFT605 RELEASED ROUTE ONLY` and both tabs, followed by independently observed frontend class/CSS changes. This establishes the configuration lesson. The precise `../**/*.go` string is relative to Gimbal's `web/src/app.css`; derive exclusion from the actual application stylesheet and frontend root, including Go at that root and below routes. Do not copy that path blindly. #347's separate handoff problem and Gimbal publication are outside this issue.

**Existing contract.** Current `templateapi/template.go` requires preservation of edits, incompatible-file rejection before writes, cancellation, partial changed paths on error, and development-only plugin loading. `internal/templates/registry.go` resolves add-on operations for both direct application and template steps. Reuse these behaviors. Cached `promise-audit-2026-10-17/facts/matrix/add-ons.md` describes older `skgo new` pass-through tests with tool shims; those historical checks cannot establish current `skgo add` behavior or browser state retention.

**Independent proof.** Focused Go contracts cover fresh setup, a non-default stylesheet/layout relationship, existing handwritten CSS/configuration, idempotence, genuine conflicts, optional dependency/composition behavior, and correct backend exclusion without narrowing frontend discovery. Real running fresh and edited-existing applications must demonstrate a supplied literal unsent draft and a second local view survive separate Go edits at the frontend root and beside a route, while literal changed Go behavior becomes visible and the document lifetime remains unchanged. Separately change a frontend utility class and authored CSS rule and inspect their computed visible effects through ordinary HMR. Verify the existing authored CSS remains effective and repeated add-on application adds no duplicate setup. No green installation/build alone meets this proof. Reuse one qualified browser session per fixture rather than repeat full suites.

**HITL.** No missing design choice: the issue supplies goal, constraints, integration boundary and browser proof. Executable now. Choosing safe source transforms, resolving actual stylesheet/layout relationships and verifying compatible dependencies are implementation work. No global Vite Go ignore, persistence feature, special Go replacement protocol, unrelated styling plugins or new architecture is authorized.
