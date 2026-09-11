rule: zero `npm`/`npx` calls in skgo or generated projects (Tyler, 2026-09-10). `devEngines.packageManager` `onFail: "download"` makes npm refuse in `web/`; that is the guard. Do not loosen it — I proposed `onFail: "warn"` and was wrong.

finding: sv cannot apply vitest or storybook without npm. vitest-addon.ts:48 writes `npm run test:unit -- --run`; storybook.ts:15 runs unpinned `create-storybook@latest`, whose PNPMProxy.ts:173 shells to `npm config get registry`. skgo writes these add-ons from its own templates.

trap: sv refuses a dirty tree and, without a TTY, exits 0 at that prompt; it also exits 0 when an add-on option is missing, and after `ERR_PNPM_IGNORED_BUILDS`. `CI=1` makes its closing install frozen-lockfile. Never read sv's exit code as evidence.

trap: npm `latest` for `sv` is 0.17.0 (Kit 2 line); Kit 3 is `sv@next` (1.0.0-next.7). Their vitest and storybook add-ons write identical files.

trap: `vitest@latest` is 5.0.0. Storybook 10.6's init installs it and `@storybook/addon-vitest` then runs zero tests (every story times out). vite-plus 0.3.1 bundles 4.1.11; keep vitest on that.

finding: `@storybook/sveltekit` 10.6 strips kit's `vite-plugin-sveltekit-compile` and `-guard` only (`storybook@10.6.0/code/frameworks/sveltekit/src/preset.ts:28`). Kit 3's `-remote` and `-remote-guard` stay in, so `sb.mock` of a `.remote.ts` fails and `storybook build` fails with MISSING_EXPORT. Upstream gap, not skgo's.

finding: a real remote response cannot reach a component under Vitest or Storybook — kit's client remote code needs `app`, set only by `start()`. Mocking is the only route.
