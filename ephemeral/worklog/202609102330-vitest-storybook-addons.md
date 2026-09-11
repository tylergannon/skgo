trap: `devEngines.packageManager` with `onFail: "download"` in the generated `web/package.json` makes npm 11 refuse every command in `web/` (`EBADDEVENGINES`). That is `npx sv`, sv's hardcoded `npm run test:unit` script, and Storybook's init (`npm config get registry`). correction: do not drop `devEngines` — it is Vite+'s pin target. Run sv as `vp dlx sv@next …` (resolves to `pnpm dlx`), and set `onFail: "warn"`: npm maps `download` to `error` (`npm-install-checks/lib/dev-engines.js`), while `vp` downloads the pinned pnpm regardless of `onFail`. Measured: `vp dlx sv add storybook` fails under `download` at Storybook's deliberate `npm config get registry` (PNPMProxy.ts:173), completes under `warn`.

trap: `CI=1` makes sv's closing install use a frozen lockfile and fail after the add-on applied. sv also refuses a dirty tree (and, without a TTY, exits 0 at that prompt) — `vp install` edits the lockfile, so commit before `sv add`.

trap: `sv` exits 0 having done nothing when an add-on option is missing and stdin is not a TTY, and exits 0 after `ERR_PNPM_IGNORED_BUILDS` under pnpm 11. Never read sv's exit code as evidence an add-on landed.

trap: npm `latest` for `sv` is 0.17.0 (Kit 2 line); Kit 3 is `sv@next` (1.0.0-next.7). Their vitest and storybook add-ons write identical files.

trap: `vitest@latest` is 5.0.0. Storybook 10.6's init installs it and `@storybook/addon-vitest` then runs zero tests (every story times out). vite-plus 0.3.1 bundles 4.1.11; keep vitest on that.

finding: `@storybook/sveltekit` 10.6 strips kit's `vite-plugin-sveltekit-compile` and `-guard` only (`storybook@10.6.0/code/frameworks/sveltekit/src/preset.ts:28`). Kit 3's `-remote` and `-remote-guard` stay in, so `sb.mock` of a `.remote.ts` fails and `storybook build` fails with MISSING_EXPORT. Upstream gap, not skgo's.

finding: a real remote response cannot reach a component under Vitest or Storybook — kit's client remote code needs `app`, set only by `start()`. Mocking is the only route.
