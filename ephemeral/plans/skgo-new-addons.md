# Vitest, Storybook and sv add-ons on a generated app — state, not scheduled

Evidence: `ephemeral/survey/vitest-probe.md`, `storybook-probe.md`,
`sv-addons-map.md` (sv 1.0.0-next.7, Storybook 10.6.0, kit 3.0.0-next.27).
Rule: zero `npm`/`npx` calls in skgo or generated projects; `devEngines`
`onFail: "download"` is the guard and stays.

## Works today

`vp dlx sv@next add vitest` (from `web/`, clean tree) → unit and browser
component projects; `vp test` passes. Remote functions are tested with
`vi.mock` and fixtures typed from the generated `types.ts`.

## Known gaps, all upstream

- sv writes `"test": "npm run test:unit -- --run"`
  (`packages/sv/src/addons/vitest-addon.ts:48`; playwright's `playwright.ts:19`)
  though it already has `resolveCommandArray(packageManager, 'run', …)`
  (`core/common.ts:301`). Two-line fix in sv.
- Storybook's init calls `npm config get registry` (`PNPMProxy.ts:179`) only via
  `resolveUsingBranchInstall` (`dirs.ts:78`), because sv runs it with
  `--skip-install`. Pre-installing pinned Storybook packages should avoid it —
  unproven.
- Storybook's dev server runs `npx vitest run` for "ghost stories" once after
  init (`core-server/utils/ghost-stories/run-story-tests.ts:46`). Whether
  `disableTelemetry` suppresses it is unverified.
- `@storybook/sveltekit` strips kit's `compile` and `guard` plugins but not
  `remote` and `remote-guard` (`frameworks/sveltekit/src/preset.ts:28`): remote
  mocks and `storybook build` fail. Plus `__SVELTEKIT_PAYLOAD__` in static
  builds, and init installing vitest 5.0.0 (the addon needs ≤4; hold 4.1.11).
  The storybook probe has the working bridge for each.

## If add-ons in `skgo new` are ever wanted

sv's menu flow and flag grammar, mirrored in Go (huh prototype in the sv map),
applying sv's add-ons through `vp dlx` once the upstream npm calls are gone.
Excluded regardless: sveltekit-adapter, drizzle, better-auth, paraglide.

Also noted: `.github/workflows/release.yml:97,106` call `npm pkg set` and
`npm publish`.
