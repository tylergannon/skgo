# Mission: stock Vitest and Storybook on an skgo app; sv-style add-on menus

Written 2026-09-10 at main `9ea9336`.

## The question

Does a developer who scaffolds with `skgo new` and then installs Vitest or
Storybook the stock way (the way the Svelte CLI `sv add vitest` / `sv add
storybook`, or `storybook init`, would) get a working setup? If not, what
breaks, and is the break skgo's (adapter, template, generated stubs) or
Kit 3's? Then: can `skgo new` offer those as pluggable add-ons with menus like
`sv create`'s?

## Where things are

- `cmd/skgo/main.go` — `skgo new` flags (`-build-tool mise|just|scripts`,
  `-skgo-version`, `-module`, `-origin`). Plain `flag`, no prompts today.
- `internal/newapp/newapp.go` — `Create(Options)`; template tree under
  `internal/newapp/template/` (web/ holds package.json.tmpl, vite.config.ts.tmpl,
  tsconfig.json, src/routes with a `hello.remote.go.tmpl`).
- `internal/newapp/scaffold_test.go`, `qualification_test.go` — how the project
  already proves a scaffold runs end to end (read these before inventing a way).
- The scaffolded web app uses **vite-plus** (`vite` is aliased to
  `@voidzero-dev/vite-plus-core`, `defineConfig` comes from `vite-plus`, the
  binary is `vp`). Vite+ bundles its own Vitest (`vp test`). Which Vitest is
  "stock" here is part of the question.
- Kit 3 (`@sveltejs/kit 3.0.0-next.27`): no `svelte.config.js` (config is passed
  to `sveltekit({...})` in vite.config.ts), `$lib` is `#lib` via package.json
  `imports`. Read `ephemeral/sveltekit-current/SKILL.md` in this tree first.
- Remote functions: `*.remote.go` → generated `.remote.ts` stubs that **throw**
  in JS. Any component that calls a remote function will hit a throwing stub
  outside the Go server. Expect this to matter for component tests and stories.
- The adapter: `internal/adapter/skgo-adapter.js` + `internal/adapter/skgo-adapter/`
  (adds a fourth Vite environment, `goja`, via `vite.plugins`). Published name
  `@skgo/sveltekit-adapter`.

## Pinned sources

`/Users/tyler/src/skgo/ephemeral/inspiration/reference/` — kit@3.0.0-next.27,
svelte@5.56.10, vite-plugin-svelte@7.3.0, vite-plus@0.3.0,
vite-plus-core-npm@0.3.0. The Svelte CLI (`sveltejs/cli`, package `sv`) and
Storybook are **not** pinned yet; a mission that needs one clones it there at
an exact tag (`<name>@<version>`), read-only afterwards. Never copy or symlink
that directory into a worktree.

## Constraints

- Other sessions run servers on 8080/5173/6006. Pick your own ports; kill by
  pid, never pkill.
- Scratch apps go in your scratchpad, not the repo.
- No PNGs in the repo. Screenshots go to `ephemeral/screenshots/` (gitignored)
  and must be opened and looked at.
- A skipped check is a failure. A zero exit is not evidence.
