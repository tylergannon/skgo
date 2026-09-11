# `skgo new` add-ons: Vitest, Storybook, and sv's menus

Evidence: `ephemeral/survey/vitest-probe.md`, `storybook-probe.md`,
`sv-addons-map.md` (sv 1.0.0-next.7, Storybook 10.6.0, kit 3.0.0-next.27).

## Where stock stands today

- Vitest via `sv add vitest`: works (unit + browser component projects), once the
  template stops blocking npm. `vp test` also works but always runs vite-plus's
  own pinned Vitest.
- Storybook via `sv add storybook`: does not work. Install dies on our
  `devEngines`; the Vitest addon runs 0 tests (init installs vitest 5.0.0, the
  addon supports ≤4); any story touching a `.remote.ts` breaks dev mocking and
  fails `storybook build`, because `@storybook/sveltekit` strips kit's `compile`
  and `guard` plugins but not `remote` and `remote-guard`
  (`storybook@10.6.0/code/frameworks/sveltekit/src/preset.ts:28`). `$app/paths`
  crashes in the static build (`__SVELTEKIT_PAYLOAD__`). Every one has a
  demonstrated fix; with them, dev, static build and 14/14 stories-as-tests pass.
- Remote functions in isolation are mocked, not served: kit's client cannot
  handle a real remote response without `start()`. `vi.mock` / `sb.mock` with
  fixtures typed from the generated `types.ts` is the honest route. No mock
  runtime from skgo — that would reimplement kit's client.

## Mission 1 — a template sv can work on

Capability: in a fresh `skgo new` app, `npx sv add vitest` and
`npx sv add storybook` (run by a developer, by hand) apply and install, and the
results run.

- Keep `devEngines.packageManager` (it is Vite+'s own pin target,
  `vite-plus@0.3.0/rfcs/dev-engines.md`) and run sv as `vp dlx sv@next …`.
  Change only `onFail: "download"` → `"warn"`: npm treats `download` as
  `error`, and two npm calls sit inside what sv applies — Storybook's
  `PNPMProxy.getRegistryURL` shells to `npm config get registry` on purpose
  (`storybook@10.6.0/code/core/src/common/js-package-manager/PNPMProxy.ts:173`)
  and sv's vitest `test` script is `npm run test:unit`. Measured on a clean
  scaffold: `vp dlx sv add storybook` dies with `download`, completes with
  `warn`; `vp` still runs pnpm 11.25.0 (it downloads regardless of `onFail`).
  Cost: the `just`/`scripts` build entry points call bare `pnpm install`,
  which under `warn` warns on a different pnpm instead of switching to it.
- Vitest kept on the version vite-plus bundles regardless of what sv or
  Storybook's init asks for (pnpm `overrides` in the template's workspace file
  is the candidate; prove it).
- Ignore lines for Vitest's failure artifacts and Storybook's output.
- Scaffold's `+page.svelte`: error snippet reads kit's `body.message`; input
  gets a label.

Acceptance: scenarios that scaffold, run sv by hand, and show a passing browser
component test and a rendered story (screenshots). CI pin for pnpm unchanged in
effect.

## Mission 2 — Storybook understands remote functions

Capability: a story can render a component that calls a Go remote function,
with a mocked answer, in dev, in `storybook build`, and under the Vitest addon.

The defect is upstream (Storybook strips too few of kit's plugins; it hits any
Kit app with remote functions). File it upstream, and ship the bridge from the
skgo adapter package so it lives in one place skgo owns rather than in every
app's `.storybook/main.ts` and `vite.config.ts`. Includes the
`__SVELTEKIT_PAYLOAD__` define for static builds.

Acceptance: a story for the scaffold's own page with mocked Go data and a
"Go is down" story, both rendering in dev and static build (screenshots), and
the stories-as-tests failing when the component or fixture is broken.

## Mission 3 — add-ons in `skgo new`, with sv's menus

Shape (sv map, option c): Go asks, sv edits.

- `skgo new` presents sv's flow in Go — directory, build entry point, add-on
  multiselect, then each chosen add-on's own options — styled like clack
  (huh themed; prototype in the sv map shows parity except minor glyphs).
- Curated list: prettier, eslint, vitest, playwright?, tailwindcss, mdsvex,
  storybook, ai-tools. Excluded: sveltekit-adapter (rips out skgo), drizzle,
  better-auth, paraglide (server-side TypeScript that never runs here),
  experimental.
- Flags mirror sv exactly: `--add vitest="usages:unit,component"`,
  `--no-add-ons`, `--install pnpm`. Non-TTY without complete flags is an error,
  not sv's silent exit 0.
- sv runs non-interactively at one pinned version with `-C web` after the
  template is written. skgo verifies each add-on landed (sv's exit code is not
  evidence) and fixes up what sv misplaces at the `web/` root (playwright's
  ignore lines, ai-tools' `.claude/`).

Acceptance: `skgo new --add vitest=... --add storybook` produces an app whose
`vitest` run passes and whose Storybook renders the page story (screenshots);
the interactive menu captured and looked at beside sv's.

## Decisions for Tyler

1. `devEngines.packageManager.onFail` → `"warn"` (and whether the `just`/`scripts`
   entry points should call `vp install` so the pin still holds there).
2. huh themed, or clack-exact prompts on bubbletea.
3. Which add-ons are on the menu (is sv's playwright redundant with the
   generated BDD suite?).
4. Filing the Storybook plugin-stripping issue upstream (outward-facing).
