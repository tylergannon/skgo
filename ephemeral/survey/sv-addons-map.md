# sv add-ons, mapped for `skgo new`

> **Superseded:** skgo and generated apps make zero `npm`/`npx` calls. `devEngines` with `onFail: "download"` stays; recommendations here to replace it or to run `npx sv` are withdrawn. Run sv as `vp dlx sv@next`. Current state: `ephemeral/plans/skgo-new-addons.md`.

sv version: **1.0.0-next.7** — npm dist-tag `next`, and the newest GitHub release
of sveltejs/cli (2026-09-04). `latest` on npm is 0.17.0, the Kit 2 line; the
1.0.0-next line is Kit 3's (its CHANGELOG: "use `#lib` instead of `$lib`",
"SvelteKit 3 adapter versions", a `sveltekit-3` migration).

Pinned source: `/Users/tyler/src/skgo/ephemeral/inspiration/reference/sv@1.0.0-next.7`
(tag `sv@1.0.0-next.7`, commit `3ee22988`). Paths below are relative to its
`packages/` directory. Every run below used `pnpm dlx sv@1.0.0-next.7`, pnpm 12.3.4
globally (11.25.0 inside the app via devEngines), Node 24.21.0, skgo built from
this worktree at `9ea9336`, create-storybook 10.6.0. `$SCRATCH` below is
`/private/tmp/claude-501/-Users-tyler-src-skgo--claude-worktrees-vitest-storybook-integration-8f75ec/d10e1823-f728-46e1-b1d5-6a351e08e282/scratchpad`
(session-local; the captures that matter are copied into
`ephemeral/screenshots/menus/`, gitignored).

Status: complete (2026-09-10).

**Recommendation in one line:** option (c) — a Go menu in `skgo new` that
offers a curated subset of sv's add-ons with sv's own option schema, then runs
`pnpm dlx sv@<pinned> add -C web <every option spelled out> --no-install`
after the template is written. sv is Kit-3-native and applies cleanly to an skgo
app; what must not be delegated is the *menu*, because sv's menu offers five
add-ons that break or contradict an skgo app and has no flag to hide them. One
template fix comes first: `devEngines.packageManager` must go (see §3 A1).

---

## 1. How sv presents its menus

**Library.** `@clack/prompts` 1.5.0, bundled into sv's dist
(`sv/package.json` devDependencies). Flags are `commander` 15; option values are
validated with `valibot`. Every prompt is a plain clack call — no custom widgets.

**The frame.** `sv/src/core/common.ts:179` `runCommand`: prints a hidden
(`\e[8m`) line for AI agents — `HINT: Run "sv --help" ... to one-shot and skip
interactive prompts.` — then `p.intro("Welcome to the Svelte CLI! (v…)")`, the
action, and `p.outro("You're all set!")`. Errors land in `p.log.error` +
"Operation failed."; Ctrl-C at any prompt is `p.cancel('Operation cancelled.')`
and exit 0 (create) / 1 (add).

### `sv create` — `sv/src/cli/create.ts`

Order (`createProject`, lines 175–429):

| # | Prompt | clack call | Flag that skips it |
|---|---|---|---|
| 1 | "Where would you like your project to be created?" placeholder `(hit Enter to use './')` | `p.text` | positional `[path]` |
| 2 | "Directory not empty. Continue?" (default No; only if non-`.git*` files exist) | `p.confirm` | `--no-dir-check` |
| 3 | "Which template would you like?" — SvelteKit minimal / SvelteKit demo / Svelte library, hint = description, default minimal (`addon` template hidden) | `p.select` | `--template <minimal\|demo\|library\|addon\|svelte>` |
| 4 | "Add type checking with TypeScript?" — Yes, using TypeScript syntax / Yes, using JavaScript with JSDoc comments / No | `p.select` | `--types ts\|jsdoc`, `--no-types` |
| 5 | "What would you like to add to your project? (use arrow keys / space bar)" — official add-ons, label = id, hint = `shortDescription - homepage` shown only on the focused row; `required: false` | `p.multiselect` | `--add <addon…>`; `--no-add-ons` skips (conflicts with `--add`) |
| 6 | per add-on option questions, prefixed `"<id>: "` when >1 add-on is selected (e.g. "vitest: What do you want to use vitest for?") | `confirm`/`select`/`multiselect`/`text` by option type | `--add vitest="usages:unit,component"` |
| 7 | (dependencies) "The X add-on requires Y to also be setup. Include it?" exists at add.ts:607 but is unreachable in this version: a missing official `dependsOn` is pushed silently and the loop `continue`s (add.ts:592–603) | `p.confirm` | — |
| — | files written (`createKit`), "Project created" | `p.log.success` | |
| 8 | "Detected package managers. Which one should we use to install dependencies?" — None + every installed agent, default = detected | `p.select` | `--install <pm>`, `--no-install`; **skipped when stdout is not a TTY** (`core/package-manager.ts:25`) |
| — | add-ons applied (`runAddonsApply`), install, format, "To skip prompts next time, run: `sv@… create --template … --types … --add … --install … <dir>`", then a `p.note` "What's next?" box with project steps and an "🧩 Add-on steps" section | | |

Two structural points to mirror: add-on **questions are asked before any file
is written**, against a *virtual* workspace built from the template's
package.json (`createVirtualWorkspace`, create.ts:468); and the add-ons are
**applied after** the project files and the package-manager choice.

Captured (ANSI + plain text) from a real run in tmux:
`ephemeral/screenshots/menus/sv-create-*.{ansi,txt,png}`.

### `sv add` — `sv/src/cli/add.ts`

`sv add [add-on…] [-C/--cwd <path>] [--no-git-check] [--no-download-check] [--install <pm> | --no-install]`.
Default cwd = nearest ancestor with a package.json (`empathic/package` `pkg.up()`,
add.ts:48); no package.json → "Invalid workspace".

`promptAddonQuestions` (add.ts:344–689), in order:
1. Parse `--add`/positional specifiers. Syntax (`core/common.ts:229`
   `parseAddonOptions`, help text add.ts:970): `<addon>`,
   `<addon>=<opt>:<val>`, `<addon>=<opt1>:<val1>+<opt2>:<val2>`,
   multiselect values comma-joined, `<opt>:none` = empty multiselect,
   booleans `yes`/`no`. An option can be addressed by its **group** name
   (drizzle's `client:` covers the `postgresql`/`mysql`/`sqlite` questions; the
   value selects which). Unknown option/value → error listing valid choices.
   Repeating an add-on → error.
2. `setupAddons` runs every selected add-on's `setup()` (collects `dependsOn`,
   `runsAfter`, `unsupported`, dynamic `addOption`s).
3. **Only if no add-on was named**: the multiselect of official add-ons whose
   `setup()` reported nothing unsupported and that aren't `hidden`.
4. Transitive `dependsOn` closure — official deps are silently added
   (better-auth → drizzle), cycles are errors.
5. Verifications: "clean working directory" (`git status --short` non-empty →
   `p.note` "Verifications not met" + confirm "Verifications failed. Do you wish
   to continue?", default No; skip with `--no-git-check`), and unsupported add-ons
   (hard error listing reasons).
6. Every still-unanswered question, per add-on in selection order, skipping any
   whose `condition(values)` is `false`. Defaults are the *initial value* of the
   prompt, not an auto-answer: **"To skip prompts, explicitly set ALL options"**.

`runAddonsApply` (add.ts:691): `applyAddons` → per-add-on cancel warnings →
"Successfully setup add-ons: …" → package-manager prompt **only if package.json
changed** → "To skip prompts next time, run: `sv@… add <reconstructed args>`" →
install → format with prettier if present → "Next steps" note.

**The option schema drives prompts, flags and help from one definition.**
`sv/src/core/options.ts`: a question is `{question, type: boolean|string|number|select|multiselect, default, options?[{value,label?,hint?}], required? (multiselect), placeholder?/validate? (string/number), group?, condition?(answersSoFar)}`.
The same record is (a) rendered as the clack prompt of that type
(add.ts:644–685), (b) parsed/validated from `id=opt:val+…` (add.ts:366–491),
(c) printed as the "Official Add-Ons" help table with defaults
(`getOptionChoices`, add.ts:983), and (d) re-serialised into the
"run this next time" command (add.ts:779–823). `condition` is what makes
drizzle's client question follow its database answer.

**Non-TTY.** Only the package-manager prompt checks `isTTY`. Any other
unanswered question still renders; with stdin at EOF the process **exits 0
having applied nothing** — measured: `sv add vitest --no-install </dev/null` in
the skgo app → exit 0, `git status` clean. A caller that drives sv
non-interactively must pass every option, and must not trust the exit code.

**Hidden AI-agent behaviour.** Not in sv — in create-storybook: when
`AI_AGENT`/`CLAUDECODE` are in the environment it prints "This command is
running via an AI agent … Proceeding with agentic installation flow" and skips
its own prompts. Any capture of the storybook flow must run under a clean env.

---

## 2. The add-on system

**Definition** — `sv/src/core/config.ts:60` `Addon`:
`{ id, alias?, shortDescription?, homepage?, hidden?, options, setup?(ws & {dependsOn, runsAfter, unsupported, addOption}), run(ws & {options, sv, cancel}), nextSteps?(ws & {options}) }`,
built with `defineAddon` + `defineAddonOptions().add(key, question).build()`.

**Workspace** (`sv/src/core/workspace.ts`): `cwd`, `language` (tsconfig vs
jsconfig), `file.viteConfig`, `file.typeConfig`, `file.stylesheet`
(`<routes>/layout.css` for kit), `isKit` (`@sveltejs/kit` in any package.json up
to the workspace root), `directory.{src,lib,kitRoutes}` (read from the kit config
wherever it lives), `dependencyVersion(pkg)`, `packageManager`. Rebuilt before
each add-on runs so later add-ons see earlier edits (engine.ts:152).

**Editing API** (`SvApi`, config.ts:21; implemented `core/engine.ts:337`):
`sv.file(path, edit)` (create-or-edit, return `false` to abort),
`sv.files({include, exclude, where}, edit)`, `sv.removeFile`,
`sv.dependency`/`sv.devDependency` (merged into package.json at the end of the
add-on, keeping a stricter existing range, alphabetised — engine.ts:41),
`sv.execute(args, stdio)` (runs `<pm> dlx|npx …` in cwd). Edits are AST-level via
`@sveltejs/sv-utils` `transforms.{script,json,text,svelte,css}` and the `js.*`
helpers (`js.vite.getConfig`, `js.imports.addNamed`, `js.object.property`…).
Kit config edits go through `svelteConfig.edit` (`sv-utils/src/svelte-config.ts`),
which finds the config in `svelte.config.{js,ts}` **or** the object passed to
`sveltekit()` in `vite.config.{ts,js}` and routes kit-level vs svelte-level keys
— this is Kit 3 native, not a shim.

**Dependencies between add-ons**: `dependsOn(id)` (adds the dep and orders
after it), `runsAfter(id)` (ordering only), topo-sorted in `orderAddons`
(engine.ts:452). A `run` that throws aborts the whole command **after earlier
add-ons have already written their files** — no rollback (observed, §3).
`cancel(reason)` skips just that add-on and its dependents.

**Community add-ons**: any npm package (`sv add foo`, `@scope` → `@scope/sv`,
`@scope/pkg@ver`) or `file:<path>`; downloaded, warned about ("Svelte maintainers
have not reviewed community add-ons"), confirm unless `--no-download-check`
(add.ts:140–342). The `addon` template of `sv create` scaffolds one.

**Programmatic API**: yes. `sv` exports (`sv/src/index.ts`) `create`, `add`
(`core/engine.ts:94`: `add({cwd, addons, options, packageManager})` → setup +
apply, no prompts, no install), `officialAddons`, `defineAddon`,
`defineAddonOptions` and all the types. `add()` skips `dependsOn` closure,
verifications and prompts — the caller supplies every option.

### Official add-ons in 1.0.0-next.7 (`sv/src/addons/index.ts` order = menu order)

| id (alias) | options (`--add` form) | writes |
|---|---|---|
| prettier | — | `prettier.config.js`, `.prettierignore`, `format`/`lint` scripts, `.vscode/extensions.json`; eslint-config-prettier if eslint present |
| eslint | — | `eslint.config.js` (svelte + ts-eslint + globals), `lint` script, `.vscode/extensions.json`, `@types/node` |
| vitest | `usages:unit,component` (multiselect, default both) | `vitest` ^4.1.8 (+ `@vitest/browser-playwright`, `vitest-browser-svelte`, `playwright` for component); `test:unit`/`test` scripts; `test.projects` [client (browser, chromium), server (node)] in the vite config; swaps `defineConfig` import from `vite` to `vitest/config`; examples in `<lib>/vitest-examples/` |
| playwright | — | `@playwright/test`, `playwright.config.ts` (webServer `npm run build && npm run preview` :4173), `test:e2e` script, `<routes>/demo/playwright/` page + `page.svelte.e2e.ts`, `test-results` in `.gitignore` |
| tailwindcss (tailwind) | `plugins:typography,forms` (default none) | `tailwindcss`, `@tailwindcss/vite` plugin in vite config, `<routes>/layout.css` imported from `+layout.svelte`, `.vscode` settings |
| sveltekit-adapter (adapter) | `adapter:auto\|node\|static\|vercel\|cloudflare\|netlify`, `cfTarget:workers\|pages` | removes `@sveltejs/adapter-*`, adds the chosen one, rewrites `adapter:` in the kit config; cloudflare adds wrangler config |
| drizzle | `database:postgresql\|mysql\|sqlite\|d1`, `client:…`, `docker:yes\|no` | `drizzle.config.ts`, `src/lib/server/db/{index,schema}.ts`, `src/env.ts`, `.env`/`.env.example`, db scripts, optional docker-compose |
| better-auth | `demo:password\|github` (dependsOn drizzle) | `src/lib/server/auth.ts`, `src/hooks.server.ts`, `app.d.ts` locals, auth schema, `/demo/better-auth` routes with `+page.server.ts` actions |
| mdsvex | — | `mdsvex` preprocessor + `.svx`/`.md` extensions in the kit config |
| paraglide | `languageTags:<text>`, `demo:yes\|no` | `project.inlang/settings.json`, `messages/*.json`, vite plugin, `src/hooks.server.ts` (middleware), `src/hooks.ts` (reroute), `%paraglide.lang%` in app.html, demo route |
| storybook | — (runsAfter vitest, eslint) | runs `<pm> dlx create-storybook@latest --skip-install --no-dev` (storybook's own prompts, inherited stdio); adds `@types/node` |
| ai-tools | `ide:…`, `delivery:plugin\|tools`, `tools:…`, `mcpSetup:local\|remote` (conditional) | agent config for the chosen clients (e.g. `.claude/settings.json`, MCP config, skills/agents files) |
| experimental | `features:async,remoteFunctions,forkPreloads` | sets `compilerOptions.experimental.async` / `experimental.<flag>` in the kit config |

---

## 3. `sv add` against a `skgo new` app

Setup: `go build ./cmd/skgo && skgo new -origin http://127.0.0.1:8431 app1`,
adapter spec pinned to the published `@skgo/sveltekit-adapter@0.3.2` (the
worktree's pseudo-version isn't on npm), `pnpm install` in `web/`, committed.
Control: `sv create ctrl --template minimal --types ts --no-add-ons --install pnpm`
(stock Kit 3, vite 8, adapter-auto, no devEngines). Every add-on was applied to
its own fresh copy, non-interactively, all options given, `--no-install`, clean env.

**Detection works.** sv recognises the skgo `web/` directory as a Kit project:
the interactive multiselect lists all 13 add-ons (none filtered as unsupported),
`svelteConfig` finds `sveltekit({...})` inside `vite.config.ts` even though
`defineConfig` comes from `vite-plus`, `experimental="features:async,remoteFunctions"`
is a correct no-op (skgo already sets both), and mdsvex/tailwind edits land in
the right objects. There is no `svelte.config.js` assumption left in 1.0.0-next.7.

Where it breaks, by cause:

**A. skgo-template causes**

1. **`devEngines.packageManager: pnpm` breaks everything that shells to `npm`
   inside `web/`.** `npx sv add` fails immediately (`EBADDEVENGINES Invalid name
   "pnpm" does not match "npm"`). `pnpm dlx sv` works. Worse, create-storybook
   (under `sv add storybook`) calls `npm config get registry` to fetch the
   framework template when `--skip-install` left `@storybook/sveltekit` unresolved;
   that npm call hits the same devEngines error, create-storybook exits 1, sv
   aborts ("Add-on 'storybook' failed during run"). **Proved causal**: the same
   run on a copy with only `devEngines` deleted from `web/package.json` succeeds
   ("Successfully setup add-ons: vitest, storybook", install done), and the stock
   control succeeds. Capture: `ephemeral/screenshots/menus/skgo-sv-add-storybook-fail.{txt,png}`.
   **A fix that keeps the pnpm pin**: replacing `devEngines` with
   `"packageManager": "pnpm@11.25.0"` — `npm config get registry` then exits 0,
   `pnpm --version` inside `web/` still reports 11.25.0, and the full
   `sv add vitest storybook --install pnpm` succeeds and installs
   (`ephemeral/screenshots/menus/skgo-sv-add-storybook-ok-packageManager-field.txt`).
   Why the template chose `devEngines` is in `ephemeral/worklog/202609061600-ci.md`
   (pinning pnpm for CI); that decision is the lead's to revisit.
2. **The Kit root is `web/`, not the repo root.** `sv add` must be run with
   `-C web` (repo root has no package.json → "Invalid workspace"). Consequences:
   playwright's `.gitignore` edit is silently skipped (it only edits an existing
   `web/.gitignore`; skgo's is at the root); ai-tools writes `web/.claude/settings.json`
   where an agent opened at the repo root won't read it; create-storybook, by
   contrast, found and edited the *root* `.gitignore`.
3. **`defineConfig` from `vite-plus`.** vitest's swap from `vite` →
   `vitest/config` (vitest-addon.ts:193) finds no `vite` import, so the config keeps
   `defineConfig` from `vite-plus` and gains a `test:` block; sv adds standalone
   `vitest ^4.1.8` beside vite-plus's bundled one. Whether that runs is the other
   agents' question; sv applies it without error.
4. **`sveltekit-adapter` destroys the app**: it adds `@sveltejs/adapter-node` and
   rewrites `adapter: skgo()` → `adapter: adapter()`, leaving a dead
   `import skgo`. Its cleanup only deletes `@sveltejs/adapter-*`, so the skgo
   adapter package stays too.
5. **Server-JS add-ons contradict skgo's core rule.** drizzle writes
   `src/lib/server/db/*.ts`, `src/env.ts`, `.env`; better-auth writes
   `src/hooks.server.ts`, `src/lib/server/auth.ts` and hand-written
   `+page.server.ts` actions (skgo *generates* `+page.server.ts` from
   `page.server.go`, `internal/gen/gen.go:272`); paraglide writes
   `src/hooks.server.ts` middleware. All apply cleanly; none of it would ever run
   in an skgo app, because no application I/O executes in JavaScript.
6. `@types/node` pin `22.20.1` is replaced by `^24` (storybook/eslint/drizzle ask
   for `getNodeTypesVersion()`; engine.ts:58 keeps an existing range only if it is
   *within* the requested one). Cosmetic, but it un-pins a template pin.

**B. Upstream causes (hit the stock app too)**

7. **`pnpm dlx create-storybook@latest` fails on a cold dlx cache under pnpm ≥ 11**:
   `ERR_PNPM_IGNORED_BUILDS … esbuild@0.28.2` (reproduced in an empty directory
   with pnpm 12.3.4 and inside `web/` with 11.25.0). Once any successful
   `pnpm dlx --allow-build=esbuild create-storybook@latest` has populated the cache,
   plain `pnpm dlx create-storybook@latest` succeeds. sv passes no `--allow-build`.
   No matching sveltejs/cli issue found (searched 2026-09-10).
8. **No rollback.** When storybook failed, vitest's edits to `package.json` and
   `vite.config.ts` and create-storybook's partial `.storybook/` and
   `src/stories/` stayed on disk, nothing installed, exit non-zero.
9. create-storybook rewrites the whole `vite.config.ts` (2-space indent, imports
   inserted, trailing newline dropped); tailwindcss re-prints `+layout.svelte`
   markup. Same in the control; formatting only runs if prettier is installed.

**What sv wrote (successful runs)**, `git status` per add-on on the skgo app:

- vitest: `package.json`, `vite.config.ts`, `src/lib/vitest-examples/{greet.ts,greet.spec.ts,Welcome.svelte,Welcome.svelte.spec.ts}`
- vitest + storybook (devEngines removed): above + root `.gitignore`,
  `.storybook/{main,preview}.ts`, `src/stories/*` (Button/Header/Page +
  `.stories.svelte`, Configure.mdx, css, assets), `vitest.shims.d.ts`, a third
  `storybook` project (storybookTest plugin, browser) in `test.projects`,
  `storybook`/`build-storybook` scripts
- prettier: `.prettierignore`, `prettier.config.js`, `.vscode/extensions.json`, `package.json`
- eslint: `eslint.config.js`, `.vscode/extensions.json`, `package.json`
- tailwindcss: `vite.config.ts`, `src/routes/layout.css`, `+layout.svelte`, `.vscode/*`, `package.json`
- playwright: `playwright.config.ts`, `src/routes/demo/+page.svelte`, `demo/playwright/{+page.svelte,page.svelte.e2e.ts}`, `package.json`
- mdsvex: `vite.config.ts`, `package.json`
- experimental (async, remoteFunctions): nothing (already set)
- sveltekit-adapter=node: `vite.config.ts` (skgo adapter replaced), `package.json`
- drizzle: `.env`, `.env.example`, `drizzle.config.ts`, `src/env.ts`, `src/lib/server/db/{index,schema}.ts`, `tsconfig.json`, `package.json`
- better-auth: drizzle's files + `src/app.d.ts`, `src/hooks.server.ts`, `src/lib/server/auth.ts`, `src/lib/server/db/auth.schema.ts`, `src/routes/demo/better-auth/**` (incl. `+page.server.ts`)
- paraglide: `messages/{en,es}.json`, `project.inlang/settings.json`, `src/app.html`, `src/hooks.server.ts`, `src/hooks.ts`, `+layout.svelte`, demo routes, `vite.config.ts`
- ai-tools (claude-code, plugin): `web/.claude/settings.json`

Scratch apps: `$SCRATCH/app1` (vitest applied, storybook failed, agent env),
`app3` (clean env, same failure), `app4` (devEngines removed, both succeeded),
`ctrl` (stock), `each/<addon>/` + `each/<addon>.{log,status,diff}`.

---

Also measured: sv applies add-ons to a **fresh `skgo new` output with no
`node_modules` and no git repo** (`vitest`, `prettier`, `tailwindcss` →
files written, exit 0). So add-ons can be applied straight after the template is
written and before the first install — the same order `sv create` uses.

### Which add-ons make sense for skgo

| add-on | verdict | why (from §3) |
|---|---|---|
| prettier, eslint, tailwindcss, mdsvex | offer | pure frontend/tooling edits, applied cleanly |
| vitest | offer | applied cleanly; whether its projects *run* under vite-plus is the other agents' finding |
| storybook | offer once A1 is fixed | fails today on the skgo template only because of `devEngines` |
| playwright | offer with care | applies, but duplicates skgo's own `e2e/` Playwright suite and its `webServer` is `npm run build && npm run preview` (vite preview, not the Go binary); its `.gitignore` edit is lost |
| ai-tools | offer with care | writes under `web/`, which is not where an agent opened at the repo root looks |
| experimental | hide | skgo already sets `async` + `remoteFunctions`; the only other flag is `forkPreloads` |
| sveltekit-adapter | never | replaces `adapter: skgo()` |
| drizzle, better-auth, paraglide | never (today) | write server-side TS (`src/lib/server/**`, `hooks.server.ts`, hand-written `+page.server.ts`) that never executes in an skgo app; the skgo equivalents are Go |

---

## 4. Go options and recommendation

Constraint check common to all three: **nothing here requires skgo to own
JavaScript.** Running `pnpm dlx sv@<ver> add …` is running kit's tooling, the way
the build already runs `vp`. What *would* violate the rule is importing sv's
programmatic `add()` (§2) from a Node script skgo ships — so the CLI is the only
acceptable interface to sv, and its flag syntax is the contract.

Scaffold-time dependency common to (a) and (c): today `skgo new` needs only Go;
Node arrives with the first build gesture (`mise.toml` pins `node = "24.16.0"`).
Applying any sv add-on at `skgo new` time needs Node + pnpm + network *then*.
Only when at least one add-on is chosen; with the mise build tool that can be
`mise x -- pnpm dlx …` after `mise trust`, otherwise pnpm on PATH, failing loudly
when absent.

### (a) Delegate: `skgo new` writes the template, then execs `pnpm dlx sv@<pin> add -C web` interactively

- Cost: ~20 lines of Go (exec with inherited stdio).
- Feel: the user gets sv's real menus, but as a second program — a second
  "Welcome to the Svelte CLI!" intro/outro after skgo's own output, and a
  "To skip prompts next time, run: `npx sv@… add …`" hint that fails in the skgo
  app while `devEngines` stands (npx → EBADDEVENGINES).
- **Disqualifier (measured):** sv's multiselect lists all 13 add-ons in an skgo
  app (`skgo-sv-add-multiselect.txt`) — including `sveltekit-adapter`, which
  rewrites `adapter: skgo()` away, and drizzle/better-auth/paraglide, whose
  output can never run. sv has no flag to hide official add-ons (`hidden` is a
  property of the add-on definition, add.ts:517). Filtering would mean not
  delegating the menu.
- When sv moves: nothing to update in skgo, which is its one real strength.

### (b) Mirror menus *and* add-ons in Go (huh + Go-templated overlays per add-on)

- Menus: see the prototype verdict below — close enough.
- Applying: for a *fresh* scaffold skgo owns every file, so each add-on could be
  a template overlay (extra devDependencies, a `test:` block in
  `vite.config.ts.tmpl`, example files) with no AST editing.
- Cost: skgo re-authors and maintains sv's add-on outputs and dependency ranges
  per add-on and follows every sv change by hand — reimplementation of kit's
  tooling, which CLAUDE.md rules out in spirit. Storybook cannot be an overlay at
  all (its output is create-storybook@latest's, re-generated per release), so
  it would shell out anyway.
- When sv/Kit moves: skgo's overlays silently go stale; nothing fails.

### (c) Hybrid — Go menu, sv applies non-interactively (recommended)

- Flow, mirroring `sv create` (§1): directory → skgo's own questions
  (`-build-tool`) → add-on multiselect (curated list above, label = id, hint =
  `shortDescription - homepage`) → each selected add-on's questions in selection
  order, prefixed `"<id>: "` when more than one, skipping any whose `condition`
  is false → write the template → `pnpm dlx sv@<pin> add -C web
  vitest="usages:unit,component" storybook … --no-install --no-git-check` →
  install via the build gesture (or a confirm, as sv's pm prompt) → a "What's
  next?" block that includes each add-on's next steps.
- Flags, mirroring sv exactly so there is one syntax to learn:
  `skgo new --add vitest="usages:unit" tailwindcss="plugins:none" DIR`,
  `--no-add-ons`. The value after `=` is passed through verbatim to sv, so skgo
  does not parse what sv already validates; sv's error ("Invalid 'vitest' add-on
  option … Available options: …") is the error.
- The Go side needs a **data copy of the curated add-ons' option schemas**
  (vitest `usages`, tailwindcss `plugins`, ai-tools' four conditional
  questions), pinned to the same sv version as the `dlx` spec. That table is the
  one thing that drifts. It must be complete: sv prompts for any option it was
  not given, and under a non-TTY it then **exits 0 having done nothing** (§1
  Non-TTY). So after the sv run, skgo should check that the add-on actually
  landed (e.g. the devDependency is now in `web/package.json`) instead of
  trusting sv's exit code — and treat the pin bump as the moment to re-diff the
  table against `sv@<new> add --help`'s "Official Add-Ons" section.
- Storybook's own clack prompts (configuration, Playwright install, crash
  reports) still appear mid-flow, inherited stdio — exactly as they do inside
  `sv create`. Accept that; it is sv's behaviour too.
- Failure semantics are sv's: an add-on whose `run` throws aborts with earlier
  add-ons' files already written (§3 B8). On a fresh scaffold that's
  tolerable; skgo should report it as a failed scaffold (non-zero exit), not
  paper over it.
- When sv moves: bump the pin deliberately; renamed/removed options fail loudly
  in sv; added options are caught by the post-run check. When Kit moves: sv's
  `svelteConfig`/`#lib` handling moves with it — which is the point of letting
  sv do the editing.
- Upstream bug to expect: `pnpm dlx create-storybook@latest` fails on a cold dlx
  cache under pnpm ≥ 11 (§3 B7); skgo can't fix it from Go and shouldn't route
  around it silently. Worth a sveltejs/cli issue.

### How close huh gets to clack (prototype, measured)

Prototype: `$SCRATCH/huhproto/main.go` (`charm.land/huh/v2` v2.0.3, ~160 lines):
one `huh.Form` per question with a clack-imitating `ThemeFunc`; the active
header `◆  Title` is printed outside the form and, once answered, erased with
`ESC[1A ESC[2K` and replaced by `◇  Title / │  answer / │` — which is how clack
leaves its history on screen (huh's own view returns "" on exit, so without
this every answered prompt vanishes). Side-by-side captures, same terminal size,
real keystrokes through tmux, rendered to PNG and inspected:
`ephemeral/screenshots/menus/compare-{1-select,2-addons-multiselect,3-addon-option,4-confirm}.png`
(raw `sv-create-*.ansi` / `huh-go-*.ansi` alongside).

Same: intro `┌`, gray `│` spine, `◇`/`◆` states, cyan gutter and `└` under the
active prompt, `◼`/`◻` checkboxes, green `●` on the current select row,
collapsed dim answers, space toggles, enter submits, arrows and j/k move, Ctrl-C
cancels (`huh.ErrUserAborted`), confirm accepts y/n and ←/→.

Not the same, and not reachable through a huh theme:
- select: clack draws `○` on every non-current row; huh renders blank padding
  there (`field_select.go:692`).
- multiselect: clack marks the cursor by brightening the focused label; huh can
  only mark it with a selector glyph column (`field_multiselect.go:596`), so the
  prototype uses a cyan `›` and every row is indented two columns further.
- hints: clack shows an option's hint on the focused (and selected) rows only;
  huh options are static strings, so the hint is on every row.
- confirm: clack `● Yes / ○ No`; huh two "buttons" (`Yes No`, focused in green).
- clack's `a` (toggle all) / `i` (invert) in multiselect; huh has ctrl+a, but its
  keymap is replaceable (`WithKeyMap`).

Non-TTY: huh refuses to run without a TTY (`bubbletea: could not open TTY`,
exit 1) — loud, unlike sv's exit 0 — so `skgo new` should check
`term.IsTerminal` up front and, without one, require `--add …`/`--no-add-ons`
the way sv's help says ("Provide --template, --types, --add, and --install to
skip prompts entirely"). `TERM=dumb` switches huh to numbered "accessible"
prompts, which is screen-reader support, not a scripting interface: piped
answers across separate forms desynchronise (observed). lipgloss v2 `Render`
does not strip colour when stdout is a pipe; print through lipgloss's writer.

If the remaining gaps matter, clack's four prompts are small enough to write
directly on bubbletea + lipgloss (huh's own dependencies) for pixel parity; huh
is the cheaper first step and already reads as "the same CLI family" in the
captures. `github.com/orochaa/go-clack` (a Go port of clack) exists but has 8
stars and no release — not a dependency to take.
