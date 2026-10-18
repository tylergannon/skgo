---
title: Creating a project
summary: Install the CLI, create an SKGo application with `skgo new`, and run it.
proof:
  - TestCreateWithoutATerminalSettlesTheMinimalTypeScriptApplication
  - TestDemoStarterGeneratesAndCompiles
---

## Prerequisites

SKGo needs Go, Node, VitePlus and `just` on your machine:

- Go %go%. Production builds accept this release or newer; [`skgo dev`](/docs/cli/dev) requires exactly this release.
- Node %node% or newer.
- VitePlus %vp%, the `vp` command. It owns the frontend tool environment and resolves pnpm %pnpm% itself; do not install or select pnpm separately.
- [`just`](https://just.systems), which runs the recipes in the project's `Justfile`.

On Apple Silicon macOS, install the signed CLI with Homebrew:

```sh
brew install --cask tylergannon/skgo/skgo
```

On Linux, download the AMD64 or ARM64 archive from [GitHub Releases](https://github.com/tylergannon/skgo/releases), check it against `checksums.txt`, and put the `skgo` executable on your `PATH`. Later, `skgo update` downloads the newest release, verifies it, and brings an application's module, adapter and frontend tool environment to the versions that release was built with.

## Creating a project

```sh
skgo new my-app
cd my-app
go tool skgo dev
```

The first command creates the project in `my-app`. SKGo hands frontend creation to `sv create` through VitePlus, so in a terminal you answer `sv`'s usual questions: the template, TypeScript, and any add-ons. Without a terminal, the answers are the `minimal` template, TypeScript, and no add-ons. Options after `--` are passed to `sv create` unchanged:

```sh
skgo new my-app -- --template demo --types jsdoc --add tailwindcss=plugins:none
```

Two kinds of answer are refused, because the result would not be an application Go can serve. `sv`'s `library` and `addon` templates are packages, not applications. Add-ons that write JavaScript server code, such as a `hooks.server.ts`, a `+server.ts` or `src/lib/server`, are reported by file name, and you create the project again without them. `--install` and `--no-install` are not available: VitePlus installs the project.

After `sv` has written the frontend, SKGo adds what every SKGo application has. `sv add` applies Vitest, unless you chose it, and the `@skgo/sv` integration, which sets `@skgo/sveltekit-adapter` as the adapter and turns on remote functions. Storybook is installed unless you chose it. Kit is pinned to %kit% and the project-local patch to its prerender queue is configured ([`skgo kit-patch`](/docs/cli/kit-patch) explains why). Then `vp install` installs the dependencies, the Go files are written, `go generate ./...` produces the bindings, `go mod tidy` settles the module, `vp check --fix` formats the frontend, and `vp build` makes the first frontend build. When it finishes, `skgo new` prints what to do next:

```sh
skgo: created the minimal project in my-app. Now:

	cd my-app
	just install
	just serve

Then open http://127.0.0.1:8080. Run `just storybook` for Storybook and `just test` for Vitest.
```

If you chose `sv`'s `demo` template, the starter is `examples` instead of `minimal`: `sv`'s demo server routes are replaced by a Go query and command in `web/src/routes/example.remote.go`, and the home page calls them.

`skgo new` takes a few flags of its own. `--module` sets the Go module path, which otherwise is the project name; `--name` sets the application name, which otherwise is the directory name; `--origin` sets the public browser origin the build is fixed to, `http://127.0.0.1:8080` unless you say otherwise. `--template` and `--set` apply an application template registered by an installed [plugin](/docs/cli/plugin) after the ordinary minimal TypeScript scaffold. [`skgo new`](/docs/cli/new) has the full table.

## Running it

`go tool skgo dev` starts the development server: Vite with HMR for the frontend, and your Go application beside it, updated in place for compatible edits and rebuilt for the rest. It requires exactly Go %go% and Node while it runs, and accepts macOS ARM64 and Linux AMD64 hosts; other platforms are rejected at startup. `go tool skgo` runs the CLI version that the project's `go.mod` requires, so a project and its tooling move together.

To build and run the production binary instead, run `just serve` and open `http://127.0.0.1:8080`. The binary embeds the frontend build and serves it without Node. Building your app explains the build.

## Editor setup

Use the [Svelte extension](https://marketplace.visualstudio.com/items?itemName=svelte.svelte-vscode) for the frontend and gopls for Go. Once a route directory holds Go files, `skgo generate` writes a `go.mod` at `web/src/routes` so that `go build ./...` does not walk into directories named `[id]` or `(group)`, which Go cannot import; gopls cannot resolve imports inside that tree. Run [`skgo check`](/docs/cli/check) for diagnostics over the whole application, Go and Svelte together, or serve the same check to an editor or agent with [`skgo mcp`](/docs/cli/mcp).
