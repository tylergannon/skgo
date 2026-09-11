trap: `devEngines.packageManager: pnpm` in the generated `web/package.json` makes npm 11 refuse every command in `web/` (`EBADDEVENGINES`). That is `npx sv`, sv's hardcoded `npm run test:unit` script, and Storybook's init (`npm config get registry`). `"packageManager": "pnpm@11.25.0"` keeps the pin and lets them run.

trap: `sv` exits 0 having done nothing when an add-on option is missing and stdin is not a TTY, and exits 0 after `ERR_PNPM_IGNORED_BUILDS` under pnpm 11. Never read sv's exit code as evidence an add-on landed.

trap: npm `latest` for `sv` is 0.17.0 (Kit 2 line); Kit 3 is `sv@next` (1.0.0-next.7). Their vitest and storybook add-ons write identical files.

trap: `vitest@latest` is 5.0.0. Storybook 10.6's init installs it and `@storybook/addon-vitest` then runs zero tests (every story times out). vite-plus 0.3.1 bundles 4.1.11; keep vitest on that.

finding: `@storybook/sveltekit` 10.6 strips kit's `vite-plugin-sveltekit-compile` and `-guard` only (`storybook@10.6.0/code/frameworks/sveltekit/src/preset.ts:28`). Kit 3's `-remote` and `-remote-guard` stay in, so `sb.mock` of a `.remote.ts` fails and `storybook build` fails with MISSING_EXPORT. Upstream gap, not skgo's.

finding: a real remote response cannot reach a component under Vitest or Storybook — kit's client remote code needs `app`, set only by `start()`. Mocking is the only route.
