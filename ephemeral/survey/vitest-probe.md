# Vitest on a scaffolded skgo app — probe

> **Superseded:** skgo and generated apps make zero `npm`/`npx` calls. `devEngines` with `onFail: "download"` stays; recommendations here to replace it or to run `npx sv` are withdrawn. Run sv as `vp dlx sv@next`. Current state: `ephemeral/plans/skgo-new-addons.md`.

Probe at main `9ea9336`, 2026-09-10. sv mapped: `sv@1.0.0-next.7` (newest release
tag; Kit-3 era — its snapshots use `@sveltejs/kit ^3.0.0-next.0`, `#lib`, config in
vite.config). Pinned at `/Users/tyler/src/skgo/ephemeral/inspiration/reference/sv@1.0.0-next.7`.
`sv@0.17.0` (npm `latest`, what a bare `pnpm dlx sv` fetches) was also run
against the same scaffold: its vitest output is byte-identical (same git blob
hashes for package.json and vite.config.ts, identical example files) and its two
example tests pass. Its source was not cloned.

## Verdict

Stock Vitest works on a fresh skgo app with one edit. sv accepts the Kit 3 /
vite-plus project without complaint and writes a config that runs under both
`vitest` and `vp test`; a `#lib` TS module, a runes component in chromium, and a
component that imports a `.remote.ts` (mocked) all pass and are load-bearing.
What breaks is `pnpm test` (sv's script shells out to `npm`, which skgo's
`devEngines` forbids), `npx sv` inside `web/` (same cause), and — by Kit 3's
design — any unmocked remote-function call inside a component test.

## Breaks, owners, fixes

| # | what breaks | exact error | whose | minimal fix | where in skgo |
|---|---|---|---|---|---|
| 1 | `pnpm test` / `vp run test` after `sv add vitest` | `npm error EBADDEVENGINES Invalid name "pnpm" does not match "npm" for "packageManager"` | sv hardcodes `"test": "npm run test:unit -- --run"`; skgo template's `devEngines.packageManager: pnpm` makes npm 11 refuse | script `"test": "vitest --run"` (demonstrated: `pnpm test` → 5 files / 6 tests pass) | generator-written web/package.json (`internal/newapp/template/web/package.json.tmpl` or an add-on overlay). Not fixable in skgo if the developer runs sv themselves — document it |
| 2 | `npx sv add vitest` run inside `web/` | same EBADDEVENGINES, before sv starts | skgo template `devEngines` | run `pnpm dlx sv add vitest` in web/, or `npx sv add vitest -C web` from the project root (both demonstrated) | README / docs of the generated app (`internal/newapp/template/README.md.tmpl`) |
| 3 | version drift between stock vitest and vite-plus's vitest (latent) | demonstrated by pinning 4.1.10: `vp test` prints `Loaded vitest@4.1.11 and @vitest/browser@4.1.10. Running mixed versions is not supported and may lead into bugs`; `vitest --run` stays consistent | sv's caret ranges (`^4.1.8`) vs vite-plus 0.3.1's exact `vitest 4.1.11` dep and `@vitest/browser-playwright 4.1.11` peer | pin `vitest`, `@vitest/browser-playwright` exactly to vite-plus's vitest (4.1.11 for vite-plus 0.3.1) and bump them with vite-plus; today's resolution happens to match only because vitest's `V4` tag is 4.1.11 | `package.json.tmpl` (skgo already pins every dep exactly) |
| 4 | component calling a remote function, unmocked | browser project: stub runs in vitest's Node dev server → `Error: skgo: implemented in Go`; with `/_app/remote` proxied to the real Go binary: `Cannot read properties of undefined (reading 'decoders')` then `Unhandled Rejection TypeError: … (reading 'hooks')`, test hangs to 15s timeout | **Kit 3**: its client remote runtime reads `app.decoders`/`app.hooks`, set only by kit's `start()`, which no component test runs. Stubs are working as designed | `vi.mock('./x.remote', factory)` with a fixture typed by the generated `types.ts` | optional example spec in the scaffold (e.g. `internal/newapp/template/web/src/routes/page.svelte.spec.ts`); no generated mock runtime (would be reimplementing kit's client) |
| 5 | calling a remote function from a server-project spec | `Error: Could not get the request store. This is an internal error.` | Kit 3 (remote functions need a request) | none needed — the server half is Go; test it with `go test` | — |
| 6 | failing browser tests leave `src/**/__screenshots__/` and `web/.vitest-attachments/` in the tree | (untracked files after any failure) | vitest behaviour; sv adds no ignore lines | add both to the ignore file | `internal/newapp/template/dot-gitignore` |

Cosmetic, no fix: every run prints Kit 3's `The following plugins may not work
correctly because they use the transformIndexHtml hook which is not supported: -
vitest:browser:transform-tester-html` (kit `exports/vite/index.js`
`configResolved`); `pnpm peers check` reports `unmet peer vite … Installed:
0.3.1` for kit, vite-plugin-svelte, vitest — pre-existing from the vite-plus
alias, harmless.

Not breaks: sv needs no `svelte.config.js` (reads kit options out of the
`sveltekit({...})` call); `#lib` needs no alias; vite-plus's `defineConfig`
type-checks the `test.projects` block (svelte-check 0 errors); `mise run build`
still exits 0 with the add-on and specs present, and `go generate` ignores the
spec files (`skgo.remotes.json` unchanged); the adapter's extra `goja`
environment does not disturb either vitest project.

## Which Vitest

Stock `vitest` — what sv installs — is the one to run. It and vite-plus share a
single `vitest` install as long as the versions match (pnpm links both to the
same store dir today), and vitest's `vite` peer resolves to the project's
aliased `@voidzero-dev/vite-plus-core`, so there is one Vite realm. `vp test`
reads the same config and passes today, but it always runs vite-plus's pinned
vitest, so it is the runner that breaks on drift (#3). Pinning exactly makes the
two equivalent. vite-plus's own route (`vite-plus/test*` imports, no `vitest`
dep) does not remove stock vitest from the graph: `vitest-browser-svelte` and
`@vitest/browser-playwright` both declare a `vitest` peer (the latter exact
`4.1.11`).

## What a generator should write (working setup, demonstrated)

`web/package.json` — scripts `"test:unit": "vitest"`, `"test": "vitest --run"`;
devDependencies (the resolved, passing versions):
`"vitest": "4.1.11"`, `"@vitest/browser-playwright": "4.1.11"`,
`"playwright": "1.63.0"` (same browser revision, chromium-1243, as the e2e
suite's `@playwright/test 1.63.0`, so one download serves both),
`"vitest-browser-svelte": "2.2.1"`.

`web/vite.config.ts` — keep `defineConfig` from `vite-plus`; add
`import { playwright } from '@vitest/browser-playwright';` and, after `plugins`:

```ts
test: {
	expect: { requireAssertions: true },
	projects: [
		{
			extends: './vite.config.ts',
			test: {
				name: 'client',
				browser: { enabled: true, provider: playwright(), instances: [{ browser: 'chromium', headless: true }] },
				include: ['src/**/*.svelte.{test,spec}.{js,ts}'],
				exclude: ['src/lib/server/**']
			}
		},
		{
			extends: './vite.config.ts',
			test: {
				name: 'server',
				environment: 'node',
				include: ['src/**/*.{test,spec}.{js,ts}'],
				exclude: ['src/**/*.svelte.{test,spec}.{js,ts}']
			}
		}
	]
}
```

(This is sv's output verbatim; no adaptation was needed.)

Ignore file: `web/src/**/__screenshots__/`, `web/.vitest-attachments/`.

One-time: `pnpm exec playwright install chromium` in web/ (sv does not do it;
the template's `mise run e2e` already does it for e2e/).

Examples: sv's `src/lib/vitest-examples/{greet.ts,greet.spec.ts,Welcome.svelte,
Welcome.svelte.spec.ts}` work unchanged. An skgo-specific one worth adding is a
mocked-remote spec for the scaffold's `+page.svelte` (the probe's is below).

## What sv's vitest add-on writes (sv@1.0.0-next.7, packages/sv/src/addons/vitest-addon.ts)

- devDeps: `vitest ^4.1.8`; component usage adds `@vitest/browser-playwright ^4.1.8`,
  `vitest-browser-svelte ^2.1.1`, `playwright ^1.60.0`.
- scripts: `test:unit: vitest`, `test: npm run test:unit -- --run` (prepended).
- files: `src/lib/vitest-examples/{greet.ts,greet.spec.ts,Welcome.svelte,Welcome.svelte.spec.ts}`.
- vite config: `test: { expect: { requireAssertions: true }, projects: [client, server] }`
  where client = `{ extends: './vite.config.ts', test: { name: 'client', browser: { enabled,
  provider: playwright(), instances: [{ browser: 'chromium', headless: true }] },
  include: ['src/**/*.svelte.{test,spec}.{js,ts}'], exclude: ['src/lib/server/**'] } }`,
  server = `{ extends, test: { name: 'server', environment: 'node', include:
  ['src/**/*.{test,spec}.{js,ts}'], exclude: ['src/**/*.svelte.{test,spec}.{js,ts}'] } }`.
  Adds `import { playwright } from '@vitest/browser-playwright'`. Swaps `defineConfig`
  import from `'vite'` to `'vitest/config'` **only if** it is imported from `'vite'`;
  skgo's comes from `'vite-plus'`, so it is left alone.
- sv has no knowledge of vite-plus anywhere in the source (grep: zero hits).

## How the probe app was made

`go build ./cmd/skgo` from the worktree, `skgo new -skgo-version v0.3.2 -origin
http://127.0.0.1:18731 vtapp` in the scratchpad, then (because main's template is
ahead of the v0.3.2 release) `@skgo/sveltekit-adapter` pointed at `pnpm pack` of
`internal/adapter` and a `replace` to the worktree in go.mod — the probe-side
equivalent of scaffold_test.go's file proxy. `mise run build` then exits 0.

Side finding, not vitest: a checkout-built `skgo new` with no `-skgo-version`
pairs main's template (kit 3.0.0-next.27) with the latest release (v0.3.2), and
that pairing does not build — first the fingerprint gate (`the
@skgo/sveltekit-adapter installed in this app is not the one this skgo
publishes`), then with the npm 0.3.2 adapter, kit next.27 throws `The
generateManifest adapter API has been removed`. Only affects checkout builds
between releases.

## Result of `pnpm dlx sv@1.0.0-next.7 add vitest=usages:unit,component --install pnpm` (run in web/)

- **sv accepts the Kit 3 project.** It detected kit from package.json, found
  `vite.config.ts`, read kit options out of the `sveltekit({...})` call, wrote the
  four example files under `src/lib/vitest-examples/`, the `test` block, the
  playwright import, the deps and the scripts, and installed with pnpm. Diff is
  exactly what the add-on source says (see above); nothing skgo-specific was
  touched or broken. `defineConfig` stays imported from `vite-plus`.
- Installed: `vitest@4.1.11`, `@vitest/browser-playwright@4.1.11`,
  `playwright@1.63.0`, `vitest-browser-svelte@2.2.1`.
- **Stock vitest and vite-plus coexist as one copy.** vite-plus 0.3.1 depends on
  `vitest` 4.1.11 exactly; sv's `^4.1.8` resolves to the same 4.1.11 (npm's `V4`
  tag), and pnpm links project `node_modules/vitest` and vite-plus's `vitest` to the
  *same* store directory. vitest's own `vite` dependency resolves to
  `@voidzero-dev/vite-plus-core@0.3.1` — the project's aliased vite, one realm.
  This is coincidence, not contract: the day vitest publishes 4.1.12, `^4.1.8`
  picks it while vite-plus still pins 4.1.11, and the project gets two vitests.
  (vitest `latest` is already 5.0.0; sv's `^4` keeps it off that.)
- `npx sv add vitest` (the command sv's docs print) **fails in web/** before sv
  runs: `npm error EBADDEVENGINES Invalid name "pnpm" does not match "npm" for
  "packageManager"` — npm 11 enforces the template's `devEngines.packageManager`.
  `pnpm dlx sv ...` works.

## Running it

| command (in web/) | result |
|---|---|
| `pnpm test` (sv's script: `npm run test:unit -- --run`) | **fails before vitest starts**: `npm error EBADDEVENGINES Invalid name "pnpm" does not match "npm" for "packageManager"` |
| `vp run test` | same EBADDEVENGINES failure (runs the same script) |
| `pnpm run test:unit --run` (= `vitest --run`) | passes — both projects: `✓ \|server\| …greet.spec.ts`, `✓ \|client (chromium)\| …Welcome.svelte.spec.ts` |
| `vp test` (vite-plus built-in) | passes, same two tests, `RUN v4.1.11` — reads the same `test` block; stock `vitest` / `@vitest/browser-playwright` imports resolve fine under it |
| `pnpm run check` (svelte-check, includes vite.config.ts and the specs) | `0 ERRORS 0 WARNINGS` — vite-plus's `defineConfig` accepts the `test.projects` block sv writes |

Noise on every run, harmless: `The following plugins may not work correctly
because they use the transformIndexHtml hook which is not supported: -
vitest:browser:transform-tester-html`. It is Kit 3's own warning
(`@sveltejs/kit/src/exports/vite/index.js` `configResolved`, lists every plugin
with `transformIndexHtml`), tripped by vitest's browser plugin; it would print in
any Kit 3 app with browser-mode vitest, skgo or not.

Chromium: sv does not run `playwright install`; the probe used browsers already
in `~/Library/Caches/ms-playwright` (chromium-1243 matches playwright 1.63). A
clean machine needs `pnpm exec playwright install chromium` once.

## (a) plain TS module under `#lib`, (b) runes component in the browser project

Both work stock, no config beyond what sv wrote.

- `src/lib/format.ts` (`plural(n, word)`), tested by `src/lib/format.spec.ts`
  importing **`#lib/format.ts`** (package.json `imports`, Kit 3's `$lib`
  replacement) — runs in the `server` project. Vite resolves `#lib/*` through
  package.json `imports` in both projects; no alias needed. (Extension required:
  `#lib/format.ts`, per Kit 3.)
- `src/lib/Counter.svelte` — `$props`, `$state`, `$derived`, `onclick`, and an
  import of `#lib/format.ts` from inside the component — tested by
  `src/lib/Counter.svelte.spec.ts` in the `client (chromium)` project: renders with
  `step: 5`, clicks three times, asserts `3 clicks` and `Total: 15`.
  Screenshot: `ephemeral/screenshots/vitest/counter-after-3-clicks.png` (shows
  "3 clicks / Total: 15 / Add 5").
- The app's `compilerOptions: { experimental: { async: true } }` (inside
  `sveltekit({...})`) reaches the test projects through `extends:
  './vite.config.ts'`, so `+page.svelte`'s `{@const s = await status()}` compiles
  and runs in the browser project.

Screenshot mechanics (not a skgo issue, worth knowing for any generator that
wants evidence): `page.screenshot({ path })` must be a path inside the project —
an absolute path outside it fails `Access denied to "…" … server.fs.strict`. With
vitest's default `browser.ui` (on unless CI) the headless orchestrator's
container measures 0px tall and vitest scales the test iframe to 0.38, so shots
come out ~158px wide; `--browser.ui=false` plus `page.viewport(480, 360)` gives a
1:1 picture.

## (c) components that import a `.remote.ts`

Measured on the scaffold's own `src/routes/+page.svelte` (query `status` in a
boundary, command `greet(name).updates(status())`).

**Unmocked, browser project** — fails, but not where one would guess. Kit's
remote plugin runs in vitest's dev server exactly as in `vp dev`: the client
module becomes kit's fetch proxy, the call hits `GET /_app/remote/1086sy4/status`
**on the vitest server**, kit's dev middleware executes the generated stub in
Node, the stub throws `Error: skgo: implemented in Go` (printed in the vitest
log), and the boundary renders `data-testid="status-failed"` with an empty
message. No Go, no value — the stub did its job.

**Unmocked, server project** (calling `status()` from a `.spec.ts`) — `Error:
Could not get the request store. This is an internal error.` from
`@sveltejs/kit/src/exports/internal/server/event.js`. Kit's server remote
functions only run inside a request. Irrelevant for skgo anyway: the server side
of a remote function is Go and is tested with `go test`.

**Real Go behind the test server** (client project `server.proxy:
{'/_app/remote': {target: ORIGIN, headers: {origin: ORIGIN}}}`, binary running)
— the proxy works (`fetch('/_app/remote/1086sy4/status')` from the test returns
200 with Go's devalue payload, `"vtapp"`, `"go1.27.1"`), but **kit's client
remote runtime cannot consume a successful answer in a component test**:
`remote_request` does `devalue.parse(result.data, app.decoders)` and `app` is
only assigned by kit's `start()` (`runtime/client/client.js` `_start`), which a
component test never calls. Demonstrated directly: kit's `remote_request` on
Go's good answer rejects with `Cannot read properties of undefined (reading
'decoders')`; through the component, the query's catch calls `handle_error`,
which throws again on `app.hooks` (`Unhandled Rejection TypeError: Cannot read
properties of undefined (reading 'hooks')`), and the boundary never settles
(test times out at 15s). This is **Kit 3's** — the same code path runs for any
adapter; in a stock adapter-node app the dev middleware would answer with the
real TS function and die at the same `app.decoders` (by code reading; not run).
Kit ships no remote-function test utilities (package exports: none).
Experiment files kept at
`scratchpad/vitest-probe/go-proxy-experiment/` — not part of the working setup.

**What works: `vi.mock` the `.remote` module.** `src/routes/page.svelte.spec.ts`
mocks `./hello.remote` with a test-owned fixture; a query mock is a function
returning a promise, a command mock returns a promise with `.updates()`
attached:

```ts
const fixture = vi.hoisted(() => ({ name: 'Fixture App', goVersion: 'go-fixture-1.0', greetings: 2, lastGreeting: 'Ada' }));
vi.mock('./hello.remote', () => ({
	status: vi.fn(() => Promise.resolve(fixture)),
	greet: vi.fn((name: string) => {
		const done = Promise.resolve({ ...fixture, greetings: 3, lastGreeting: name });
		return Object.assign(done, { updates: vi.fn(() => done) });
	})
}));
```

Two tests pass: the page renders the fixture (title, `Served by
go-fixture-1.0.`, greetings `2`, `Last greeting: Ada`), and submitting "Grace"
calls `greet('Grace')` and `.updates(...)` once. Screenshot:
`ephemeral/screenshots/vitest/page-with-mocked-remote.png`.

Limit, stated plainly: a `vi.fn` query is not kit's reactive query instance, so
single-flight refresh (the greet response updating `status` on screen) cannot
be observed in a component test — the page still shows `2` after greeting. That
behaviour is covered only where kit's client is booted: the generated Playwright
e2e suite against the Go binary.

Should skgo generate something? Not a mock runtime — faking kit's
`RemoteQuery`/`RemoteCommand` objects (`.current`, `.loading`, `.refresh`,
`.updates`, `.withOverride`) is reimplementing kit's client, and kit itself
offers nothing to mirror. Typing: the generated `types.ts` beside each stub
types the fixture (`satisfies Status` — used in the probe, svelte-check clean).
The typed module form `vi.mock(import('./hello.remote'), …)` does **not**
type-check with these mocks: `Property 'pending' is missing in type … but
required in type 'RemoteCommand<string, Status>'` — vitest's factory must return
`Partial<typeof module>`, and a `vi.fn` is not a `RemoteCommand`. Use the string
form. The useful help is one example spec in the scaffold showing this pattern
(see generator section).

## Evidence

Final run, `vp test --reporter=verbose` (CI=1), 0 skipped:

```
 RUN  v4.1.11 …/vtapp/web
 ✓ |server| src/lib/format.spec.ts > plural (imported through #lib) > adds an s except for one
 ✓ |server| src/lib/vitest-examples/greet.spec.ts > greet > returns a greeting
 ✓ |client (chromium)| src/lib/vitest-examples/Welcome.svelte.spec.ts > Welcome.svelte > renders greetings for host and guest
 ✓ |client (chromium)| src/routes/page.svelte.spec.ts > +page.svelte with hello.remote mocked > renders what the status query returned
 ✓ |client (chromium)| src/routes/page.svelte.spec.ts > +page.svelte with hello.remote mocked > sends the typed name to the greet command, asking for status in the same flight
 ✓ |client (chromium)| src/lib/Counter.svelte.spec.ts > Counter.svelte (runes, rendered in chromium) > counts clicks with $state and derives the total with $derived
 Test Files  5 passed (5)
      Tests  6 passed (6)
```

`pnpm test` (with the fixed script) and `vp run test`: `Test Files 5 passed (5)
/ Tests 6 passed (6)`.

Load-bearing — each mutation applied, the spec run, the file restored
(script `scratchpad/vitest-probe/mutate.sh`, log `mutations.log`):

| mutation | spec | result |
|---|---|---|
| M1 `format.ts`: `n === 1 ? '' : 's'` → `'s'` | format.spec.ts (server) | × `expected '1 clicks' to be '1 click'` |
| M2 `Counter.svelte`: `$derived(count * step)` → `count * step` | Counter.svelte.spec.ts | × times out waiting for `Total: 15` |
| M3 `Counter.svelte`: `$state(0)` → `0` | Counter.svelte.spec.ts | × `expect(element).toHaveTextContent()` did not succeed |
| M4 `+page.svelte`: title `{s.name}` → `{s.goVersion}` | page.svelte.spec.ts | × both tests (title never reads `Fixture App`) |
| M5 `+page.svelte`: `greet(name).updates(status())` → `greet(name)` | page.svelte.spec.ts | × `expected "vi.fn()" to be called 1 times, but got 0 times` (render test still ✓) |
| M6 spec: `vi.mock` removed | page.svelte.spec.ts | × `Error: skgo: implemented in Go`, title never found |
| M7 sv's `greet.ts`: `'Hello, '` → `'Hi, '` | vitest-examples | × `expected 'Hi, Svelte!' to be 'Hello, Svelte!'` and Welcome times out |

Screenshots (opened and checked):
- `ephemeral/screenshots/vitest/counter-after-3-clicks.png` — the runes
  component in chromium after three clicks: "3 clicks", "Total: 15", button
  "Add 5".
- `ephemeral/screenshots/vitest/page-with-mocked-remote.png` — the scaffold's
  `+page.svelte` rendering the mock fixture: "Fixture App", "Served by
  go-fixture-1.0.", "Greetings so far: 2", "Last greeting: Ada", input "Grace",
  Greet button.

Probe app and its git history (baseline → sv add → probe specs):
`/private/tmp/claude-501/-Users-tyler-src-skgo--claude-worktrees-vitest-storybook-integration-8f75ec/d10e1823-f728-46e1-b1d5-6a351e08e282/scratchpad/vtapp`.
