# Storybook on a scaffolded skgo app — probe

Probe at main `9ea9336`, 2026-09-10. Scratch app and logs:
`/private/tmp/claude-501/-Users-tyler-src-skgo--claude-worktrees-vitest-storybook-integration-8f75ec/d10e1823-f728-46e1-b1d5-6a351e08e282/scratchpad/sbprobe/`
(`sbapp/` is a git repo; one commit per stage: baseline → A1 → stock `sv add storybook` → fixes).

## Verdict

**The stock install does not work.** `sv add storybook` does not complete on a
fresh skgo app, and it takes two changes to get it through. After that,
Storybook dev works for presentational components and for `$app/state` /
`$app/navigation` / `$app/paths` components. A component that calls a Go remote
function cannot be mocked, and a static `storybook build` fails outright once
any story imports one. The Vitest addon does not run at all: create-storybook
installs a vitest version its own addon does not support. With four
small, generator-writable fixes everything works — dev, static build and
stories-as-tests — including the scaffold's real `+page.svelte` (await-in-markup
+ a Go query + a Go command) against a mocked remote module.

Real Go data cannot reach a component inside Storybook: Kit's client remote
runtime needs `start()`'s `app` (demonstrated below). Mocking is the only
honest route, and it works.

## Versions mapped

| thing | version | where read |
|---|---|---|
| skgo | worktree HEAD `9ea9336` (v0.3.2 + 2; adapter changed since) — `skgo new -origin http://127.0.0.1:6117 -skgo-version v0.3.2`, go.mod `replace` → worktree, adapter = `pnpm pack` of `internal/adapter` as `file:` | |
| template pins | kit 3.0.0-next.27, svelte 5.57.0, vite-plugin-svelte 7.3.0, `vite` = vite-plus-core 0.3.1, vite-plus 0.3.1 (bundles vitest 4.1.11), pnpm 11.25.0, node 24.16.0 | `internal/newapp/template/` |
| sv | 0.17.0 (npm latest; the one run) and 1.0.0-next.7 — `packages/sv/src/addons/storybook.ts` is byte-identical in both: runs `<pm> dlx create-storybook@latest --skip-install --no-dev`, then sets `@types/node` | `/Users/tyler/src/skgo/ephemeral/inspiration/reference/sv@0.17.0`, `sv@1.0.0-next.7` |
| storybook, @storybook/sveltekit, @storybook/addon-vitest, create-storybook | 10.6.0 | `/Users/tyler/src/skgo/ephemeral/inspiration/reference/storybook@10.6.0` (sparse: frameworks/sveltekit, frameworks/svelte-vite, builders/builder-vite, addons/vitest, lib/create-storybook, core/src/{cli,common,mocking-utils,…}) |
| installed by init | @storybook/addon-svelte-csf 5.1.3, @chromatic-com/storybook 5.3.1, vitest **5.0.0**, @vitest/browser-playwright 5.0.0, playwright 1.63.0, @types/node 24.13.4 | node_modules |
| kit | 3.0.0-next.27 | `/Users/tyler/src/skgo/ephemeral/inspiration/reference/kit@3.0.0-next.27` |

## What works stock (once installed)

- sv accepts the Kit 3 project: it detects kit from `sveltekit({...})` in `vite.config.ts`, and neither sv nor create-storybook needs a `svelte.config.js` ("Framework detected: sveltekit").
- `storybook dev` starts with the kit plugin, the skgo adapter and its `goja` environment all loaded.
- Svelte 5 runes components with args, `#lib/...` imports (package.json `imports`) and `compilerOptions.experimental.async` (await in markup) all work in dev and in the static build.
- Storybook's `$app/state`, `$app/navigation` and `$app/stores` mocks still work under Kit 3: `parameters.sveltekit_experimental.state.page` shows in the component, and `navigation.goto` is intercepted. Its alias object wins over kit's `$app` alias array. `$app/paths` `resolve()` resolves in dev.
- `storybook build` does not run kit's adapter or build the `goja` environment. Storybook's `build()` builds only the client environment, and there is no "Using skgo" in the log.
- `mise run build` and `vp run check` (svelte-check: 0 errors, 0 warnings) still pass with Storybook, stories and the `__mocks__` file present.

## Breaks

| # | what breaks | exact error | whose | minimal fix | where in skgo |
|---|---|---|---|---|---|
| 1 | `npx sv add storybook` in `web/` (the command the sv docs show) | `npm error EBADDEVENGINES Invalid name "pnpm" does not match "npm" for "packageManager"` | skgo template: `devEngines.packageManager {name: pnpm, onFail: download}`, which npm 11 enforces | replace `devEngines` with `"packageManager": "pnpm@11.25.0"` (see #3) | `internal/newapp/template/web/package.json.tmpl` |
| 2 | `pnpm dlx sv add storybook` | `ERR_PNPM_IGNORED_BUILDS … Ignored build scripts: esbuild@0.28.2` from sv's inner `pnpm dlx create-storybook@latest`. Reproduces in an empty dir with pnpm 11.25 and 12.3. sv prints "Operation failed" **but exits 0** | sv (runs `pnpm dlx` without `--allow-build`), given pnpm ≥11's strict dep builds | developer: `pnpm_config_strict_dep_builds=false pnpm dlx sv add storybook` (demonstrated), or run `pnpm dlx --allow-build=esbuild create-storybook@latest` directly | not skgo's to fix. A skgo add-on that writes the files itself avoids it |
| 3 | create-storybook, after #2 is worked around | `SB_CLI_0003 (ExecaCommandFailedError): Command failed: npm config get registry --workspaces=false --include-workspace-root` → `EBADDEVENGINES` again. Storybook's `PNPMProxy.getRegistryURL` shells out to **npm** in the project dir (`core/src/common/js-package-manager/PNPMProxy.ts:173`) while copying framework templates | Storybook × skgo template `devEngines` | same as #1. With `packageManager: "pnpm@11.25.0"`, `npm config get registry` succeeds and pnpm 12 still switches to 11.25.0 (demonstrated). The developer cannot work around this without editing package.json | `package.json.tmpl` |
| 4 | Storybook Vitest addon: every story, stock | `Error: The iframe "…/Button.stories.svelte" did not become ready within 60000ms`; 0 tests run | Storybook: `AddonVitestService.collectDependencies` adds `vitest` / `@vitest/browser-playwright` / `@vitest/coverage-v8` unversioned (`"latest"`), which resolves to 5.0.0 (released 2026-09-03). `@storybook/addon-vitest@10.6.0` peers `vitest ^3 \|\| ^4` and `@vitest/browser-playwright ^4`, and its `@vitest/runner` resolved to vite-plus's 4.1.11 | pin `vitest`, `@vitest/browser-playwright` and `@vitest/coverage-v8` to **4.1.11**, vite-plus 0.3.1's vitest. After that, the 12 stock and probe tests pass under both `vitest run` and `vp test run`. (Storybook reuses a declared vitest range, so adding vitest first — sv's `runsAfter('vitest')` — also avoids this.) | the add-on's package.json overlay; bump with vite-plus |
| 5 | a component calling a remote function, unmocked, stock dev | no error shown. `status()` becomes Kit's client fetch to `/_app/remote/1086sy4/status`; Kit's dev middleware, still mounted in Storybook's Vite, **executes the generated stub in Node**, logs `Error: skgo: implemented in Go`, returns `{"type":"error","error":{"status":500,"message":"Internal Error"}}`; the boundary's `failed` snippet renders **empty** | @storybook/sveltekit strips only `vite-plugin-sveltekit-compile` and `-guard` (`frameworks/sveltekit/src/preset.ts`), not Kit's remote plugins. The stub behaves as designed | fix #7 plus a mock (#9) | — |
| 6 | `sb.mock(import('../src/routes/hello.remote.ts'))` + `__mocks__/hello.remote.ts`, stock plugins | dev: `Vite Internal server error: __STORYBOOK_MODULE_TEST__ is not defined — Plugin: vite-plugin-sveltekit-remote` (Kit loads the mocked module in the SSR runner to learn its exports); the story shows "Failed to fetch dynamically imported module". Vitest: `` `fixture` exported from src/routes/hello.remote.ts is invalid — all exports from this file must be remote functions `` | Storybook (does not strip Kit's remote plugins) × Kit 3 remote functions | #7 | — |
| 7 | stock `storybook build` once any story imports a `.remote.ts`, even through `+page.svelte` | `[MISSING_EXPORT] "status" is not exported by "src/routes/hello.remote.ts"` (×4), build fails. Kit's remote plugin, with no server build metadata (the compile plugin is stripped), emits a module with zero exports | Storybook × Kit 3 | drop `vite-plugin-sveltekit-remote` and `vite-plugin-sveltekit-remote-guard` from the resolved config, in **both** Storybook (`viteFinal`) and the Vitest `storybook` project. The addon applies `viteFinal` to an empty config and inherits the root plugins through `extends: true` (`addons/vitest/src/vitest-plugin/index.ts:214-252`), so a `viteFinal` filter alone does not reach Vitest. A `configResolved` splice works in both places (file below) | generated `.storybook/without-kit-remote.ts` + its use in `.storybook/main.ts` and `vite.config.ts`. Alternative: the adapter drops them itself when a Storybook plugin (`storybook:sveltekit-mock-stores` / `vite-plugin-storybook-test`) is present (`internal/adapter/skgo-adapter.js`); not tried. Upstream: add both names to @storybook/sveltekit's strip list (covers dev and build, not Vitest) |
| 8 | `storybook build` + any component importing `$app/paths` | builds, then the story crashes: `ReferenceError: __SVELTEKIT_PAYLOAD__ is not defined` (in kit's `runtime/client/payload.js`) | Storybook × Kit 3: kit 3 defines `__SVELTEKIT_PAYLOAD__` for builds only inside `vite-plugin-sveltekit-compile`'s per-environment config (`exports/vite/build/index.js:344`), which Storybook strips. Dev defines it in `-setup` (`exports/vite/index.js:514`), so dev works | in `viteFinal`, when `configType === 'PRODUCTION'`: `define.__SVELTEKIT_PAYLOAD__ = 'undefined'` | generated `.storybook/main.ts` (upstream: @storybook/sveltekit) |
| 9 | mocking a generated stub without a hand-written mock file | automock (`sb.mock` with no `__mocks__`), after #7: `SyntaxError: … kit/src/runtime/invalid-import.js … does not provide an export named 'prerendering'` (the stub imports `$app/server`). The automocker also emits the type-only `export type { Status }` as a value (`["Status"]: Status`) | generated stubs (browser-unloadable by design) × Storybook automocker (does not skip type-only exports) | hand-write `src/routes/__mocks__/<name>.remote.ts` (below). A query mock must be awaitable; a command mock must return a promise that also has `.updates()` | see "Should skgo generate mocks" |
| 10 | live Go behind Storybook: `server.proxy['/_app/remote'] = <Go binary>` with Kit's remote plugins left in | Go answers (`curl` returns the devalue payload for `1086sy4/status`), but the component stays on "Pending async component..." forever and the page throws `Cannot read properties of undefined (reading 'hooks')` | Kit 3: the client remote runtime reads `app.hooks` / `app.decoders`, which only kit's `start()` sets. Not fixable without reimplementing kit's client | none: mock instead | — |
| 11 | scaffold `+page.svelte`'s `failed` snippet | renders `<p data-testid="status-failed"></p>`, empty, for a real remote error: kit's `HandledHttpError` has `.body.message`, not `.message` (observed: `e.message` falsy, `e.body` = `{"status":500,"message":"Internal Error"}`) | skgo template | render the kit error's `body.message`, falling back to `message` | `internal/newapp/template/web/src/routes/+page.svelte` |
| 12 | `storybook dev` on a tree where `go generate` has not run | `Error: ENOENT: no such file or directory, open './skgo.remotes.json'` at `skgo-adapter/env.js:675` (`configureServer` of `skgo-goja-dev-environment`), and Storybook exits | skgo adapter: its dev environment starts in every Vite dev server, Storybook's included | tolerate a missing file, or skip the goja dev environment when kit's SSR host is not the consumer. Low severity: `vp dev` needs the file too | `internal/adapter/skgo-adapter/env.js` |
| 13 | stock a11y addon on the scaffold's home page | Accessibility panel: `Form label — Critical — 1` (the name `<input>` has no label). It is informational under init's `a11y.test: 'todo'` and would fail tests under `'error'` | skgo template | label the input | `internal/newapp/template/web/src/routes/+page.svelte` |
| 14 | untracked files | `web/storybook-static/` after `pnpm run build-storybook`; `web/debug-storybook.log` after a failed init | template `.gitignore` has no entries for them, and create-storybook added none | add `web/storybook-static/` and `web/*storybook.log` | `internal/newapp/template/dot-gitignore` |

Cosmetic, no fix needed:

- Every start prints `[vite-plugin-svelte] no Svelte config found … using default configuration` four times. addon-svelte-csf calls `loadSvelteConfig()` for `preprocess` only, so a `preprocess` passed to `sveltekit({...})` would not reach `.stories.svelte` files (Kit 3 × addon-svelte-csf; the scaffold has none).
- Kit prints `The following plugins may not work correctly because they use the transformIndexHtml hook…` for Storybook's and Vitest's plugins, from kit's `configResolved`, which is still mounted.
- Vitest's dependency scan prints a non-fatal `[UNRESOLVED_ENTRY] Entry module ".../@fs/.../__mocks__/hello.remote.ts" cannot be external`.
- addon-vitest's postinstall rewrote `vite.config.ts`: imports went in under the ORIGIN comment, it was reformatted to 2 spaces and lost its trailing newline.
- sv changed `@types/node` from 22.20.1 to `^24`.
- Init's "install Playwright Chromium?" prompt is skipped without a TTY, so a fresh machine needs `pnpm exec playwright install chromium` (chromium-1243, the same one e2e's Playwright 1.63 uses).
- create-storybook switches to an "agentic installation flow" when `AI_AGENT` / `CLAUDECODE` is set. The successful run unset them to get the human path.

Kit 3 note: in the stock setup Storybook's Vite server still mounts Kit's SSR/remote dev middleware and skgo's `goja` dev environment. Neither is needed. #5 and #12 are consequences of that.

## Should skgo generate mocks for remote functions

The 20-line hand mock below covers `await query()` and `command(arg).updates(query())`. A component that uses the rest of Kit's `RemoteQuery` surface (`.current`, `.loading`, `.refresh()`, `.set()`, `.withOverride()`) needs more. Generating a faithful fake means re-implementing kit's client query object, which the project rules forbid (#10 shows the real one cannot run outside `start()`). What the generator does know is each function's kind and types. The most it could honestly emit is typed `fn()` spies with those shapes, whose default implementation rejects with a "set a fixture" message. That is optional, and it would add files to the route tree that `scaffold_test.go` enumerates. My recommendation: no generated mocks now; ship the pattern as an example in a Storybook add-on.

## A working setup (every file a generator would write)

Demonstrated end state is the commit tree of `sbapp/` above. Starting from the scaffold:

`web/package.json`: replace `devEngines` with `"packageManager": "pnpm@11.25.0"`. Add scripts
`"storybook": "storybook dev -p 6006"` and `"build-storybook": "storybook build"`. Add devDependencies
(init wrote `latest` / `^` for these; pin them exactly, as the template does for everything else):
`storybook 10.6.0`, `@storybook/sveltekit 10.6.0`, `@storybook/addon-svelte-csf 5.1.3`,
`@storybook/addon-vitest 10.6.0`, `@storybook/addon-a11y 10.6.0`, `@storybook/addon-docs 10.6.0`,
`@chromatic-com/storybook 5.3.1` (optional), `vitest 4.1.11`, `@vitest/browser-playwright 4.1.11`,
`@vitest/coverage-v8 4.1.11`, `playwright 1.63.0`.

`web/.storybook/without-kit-remote.ts`:

```ts
import type { Plugin } from 'vite';

const KIT_REMOTE = ['vite-plugin-sveltekit-remote', 'vite-plugin-sveltekit-remote-guard'];

export function withoutKitRemote(): Plugin {
	return {
		name: 'skgo:storybook-without-kit-remote',
		configResolved(config) {
			const plugins = config.plugins as Plugin[];
			for (let i = plugins.length - 1; i >= 0; i--) {
				if (KIT_REMOTE.includes(plugins[i].name)) plugins.splice(i, 1);
			}
		}
	};
}
```

`web/.storybook/main.ts`:

```ts
import type { StorybookConfig } from '@storybook/sveltekit';
import { withoutKitRemote } from './without-kit-remote';

const config: StorybookConfig = {
  stories: ['../src/**/*.mdx', '../src/**/*.stories.@(js|ts|svelte)'],
  addons: ['@storybook/addon-svelte-csf', '@chromatic-com/storybook', '@storybook/addon-vitest',
           '@storybook/addon-a11y', '@storybook/addon-docs'],
  framework: '@storybook/sveltekit',
  async viteFinal(config, { configType }) {
    return {
      ...config,
      plugins: [...(config.plugins ?? []), withoutKitRemote()],
      define: configType === 'PRODUCTION'
        ? { ...config.define, __SVELTEKIT_PAYLOAD__: 'undefined' }
        : config.define
    };
  }
};
export default config;
```

`web/.storybook/preview.ts` is init's file, plus one `sb.mock(import('../src/routes/<name>.remote.ts'))` per mocked
remote module (`import { sb } from 'storybook/test'`).

`web/vite.config.ts`: keep `defineConfig` from `vite-plus` and the `sveltekit({...})` plugin. Add:

```ts
/// <reference types="vitest/config" />
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { storybookTest } from '@storybook/addon-vitest/vitest-plugin';
import { playwright } from '@vitest/browser-playwright';
import { withoutKitRemote } from './.storybook/without-kit-remote';
const dirname = path.dirname(fileURLToPath(import.meta.url));
// … in defineConfig({ plugins: [sveltekit(...)], test: { … } }):
test: {
  projects: [{
    extends: true,
    plugins: [withoutKitRemote(), storybookTest({ configDir: path.join(dirname, '.storybook') })],
    test: { name: 'storybook', browser: { enabled: true, headless: true,
            provider: playwright({}), instances: [{ browser: 'chromium' }] } }
  }]
}
```

`web/vitest.shims.d.ts`: `/// <reference types="@vitest/browser-playwright" />` (written by init).

Per remote module, `src/routes/__mocks__/hello.remote.ts` (the file kit's build never sees, because nothing imports it):

```ts
import { fn } from 'storybook/test';
import type { Status } from '../types';
export type { Status };
export const fixture: Status = { name: 'Storybook fixture', goVersion: 'go (mocked in Storybook)', greetings: 7, lastGreeting: 'Grace Hopper' };
export const status = fn((): Promise<Status> => Promise.resolve(fixture)).mockName('status');
export const greet = fn((name: string) => {
	const result = Promise.resolve({ ...fixture, greetings: fixture.greetings + 1, lastGreeting: name });
	return Object.assign(result, { updates: (..._queries: unknown[]) => result });
}).mockName('greet');
```

A story overrides it per case with `beforeEach: () => { mocked(status).mockRejectedValue(new Error('…')) }`.

`.gitignore`: `web/storybook-static/`, `web/*storybook.log`.

## Evidence

Screenshots in `/Users/tyler/src/skgo/.claude/worktrees/vitest-storybook-integration-8f75ec/ephemeral/screenshots/storybook/` (all opened and checked):

- `dev-probe-*` / `static-probe-*`: six stories each, in Storybook dev (:6116) and in the static build served on :6118. Every interactions panel shows PASS.
  - counter--initial: Count 3 / Doubled 6.
  - counter--clicked-to-max: 3 clicks → Count 9, Doubled 18, button disabled, "Reached the maximum of 9".
  - navstatus--mocked-page-state: path `/dashboard/42`, route `/dashboard/[id]`, param 42, user "Ada Lovelace".
  - navstatus--goto-is-intercepted: the goto spy is called with `/about`.
  - home-page-go-remote--with-mocked-go: the scaffold's real `+page.svelte` shows "Storybook fixture", 7 greetings, and `greet('Ada')` is asserted.
  - home-page-go-remote--go-is-down: the failed snippet shows "Go is unreachable (mocked)".
- `dev-ui-run-tests.png`: Storybook's own "Run tests" button. Every component shows a green tick ("Ran 14 tests just now").
- Failure evidence:
  - `dev-BROKEN-sbmock-with-kit-remote-plugin-*`: #6.
  - `static-BROKEN-no-payload-define-*`: #8, `__SVELTEKIT_PAYLOAD__ is not defined`.
  - `dev-BROKEN-live-go-proxy-*`: #10, "Pending async component...".

Vitest addon (logs in the scratch dir):

- `vitest-final.log`: `pnpm exec vitest run --project=storybook` → 6 files, 14 tests passed.
- `vp-test-final.log`: `vp test run --project=storybook` → vitest 4.1.11, 14 passed.
- `vitest-mutated.log`: component changed to `count * 3` and mock fixture changed to `greetings: 8` → 3 failed, each with an Expected/Received line: `Doubled: 6` vs `Doubled: 9`, `Doubled: 18` vs `Doubled: 27`, `7` vs `8`.
- Stock failures:
  - `vitest-stock.log`, `vitest-plain.log`: #4.
  - `vitest-pinned-remote.log`: #6 in Vitest.
  - `KEEP-sb-build-stock-missing-export.log`: #7.
  - `sv-add.log`, `sv-add2.log`, `sv-add3.log`: #1–#3.
  - `sv-add4.log`: the successful install.

## Not done

- sv@1.0.0-next.7 was not run against the app. Its storybook add-on source is identical to 0.17.0's.
- `$app/forms` / `enhance`, `$app/env` and `$app/manifest` inside stories were not exercised.
- The adapter auto-detect alternative in #7 was not tried.
