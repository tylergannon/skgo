# `skgo new` add-ons: Vitest, Storybook, and sv's menus

Evidence: `ephemeral/survey/vitest-probe.md`, `storybook-probe.md`,
`sv-addons-map.md` (sv 1.0.0-next.7, Storybook 10.6.0, kit 3.0.0-next.27).

## Rule

Zero calls to `npm` or `npx` in skgo or in any generated project. pnpm and
`vp` only. `devEngines.packageManager` with `onFail: "download"` stays: npm
refusing to run in `web/` is the guard working.

That rules out letting third-party installers write the add-ons:

- sv's vitest add-on writes `"test": "npm run test:unit -- --run"`
  (`sv@1.0.0-next.7/packages/sv/src/addons/vitest-addon.ts:48`); playwright's
  writes `npm run test:e2e` (`playwright.ts:19`).
- sv's storybook add-on runs `create-storybook@latest` (unpinned,
  `storybook.ts:15`), whose pnpm proxy shells to `npm config get registry` on
  purpose (`storybook@10.6.0/code/core/src/common/js-package-manager/PNPMProxy.ts:173`).

So skgo owns each add-on it offers as Go templates: the files sv's add-on
would write, mirrored from sv's source at a pinned version, with pnpm/vp
scripts and exact dependency versions, installed with `vp install`. Kit/sv
stay the spec for *what* is written; skgo writes it.

## Where stock stands (unchanged findings)

- Vitest (sv's output): unit + browser component projects pass under `vitest`
  and `vp test` once the `test` script is not npm. Keep `vitest` and
  `@vitest/browser-playwright` on the version vite-plus bundles (4.1.11).
- Storybook: needs (1) vitest held at 4.1.11 (init pulls 5.0.0, the addon runs
  0 tests), (2) kit's `vite-plugin-sveltekit-remote` and `-remote-guard`
  stripped (Storybook 10.6 strips only `compile` and `guard`, `preset.ts:28`),
  applied in `.storybook/main.ts` and the Vitest `storybook` project, (3) a
  `__SVELTEKIT_PAYLOAD__` define for static builds. With those: dev, static
  build and 14/14 stories-as-tests pass.
- Remote functions in isolation are mocked (`vi.mock` / `sb.mock` with fixtures
  typed from the generated `types.ts`); kit's client cannot take a real remote
  response without `start()`.

## Mission 1 — Vitest and Storybook as skgo add-ons

Capability: `skgo new --add vitest="usages:unit,component" --add storybook
DIR` produces an app where `vp test` passes unit, component and story tests,
Storybook dev and `storybook build` render the scaffold's page story with
mocked Go data, and nothing anywhere invokes npm.

- The Storybook remote-plugin bridge lives in `@skgo/sveltekit-adapter`, not
  pasted into each app.
- A scaffold test fails if any generated file invokes `npm`/`npx`.
- Scaffold `+page.svelte`: error snippet reads kit's `body.message`; input gets
  a label (a11y addon flags it).

Acceptance: scenarios with screenshots of the component test and of each story
in dev and static build; each test broken once to show it fails.

## Mission 2 — sv's menus in Go

Capability: interactive `skgo new` walks sv's flow — directory, build entry
point, add-on multiselect, each chosen add-on's options — styled like clack
(huh themed; prototype in the sv map). Flags mirror sv's grammar (`--add
id="opt:val+val"`, `--no-add-ons`). Non-TTY with incomplete flags is an error.

Acceptance: menu captures beside sv's, looked at; non-interactive flags produce
the same tree as the menu.

## Open

1. Which add-ons beyond vitest and storybook (each is a template skgo maintains).
2. huh themed vs clack-exact prompts on bubbletea.
3. `.github/workflows/release.yml:97,106` still call `npm pkg set` and
   `npm publish` (trusted publishing). Replacing them needs pnpm to publish
   natively with OIDC provenance — verify before changing the release path.
4. Filing Storybook's plugin-stripping gap and sv's hardcoded `npm run` upstream.
