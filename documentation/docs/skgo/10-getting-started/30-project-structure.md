---
title: Project structure
summary: The files `skgo new` creates, which ones you edit, and which ones are generated.
proof:
  - example/e2e/features/colocation.feature
---

An SKGo project is a Go module with a SvelteKit project in `web/`. A new project made with `sv`'s `minimal` template, TypeScript and no add-ons looks like this:

```tree
my-app/
├ cmd/
│ └ main.go
├ internal/
│ ├ app/
│ │ ├ locals.go
│ │ └ skgo_gen.go
│ ├ serverhooks/
│ │ └ handle.go
│ └ skgo/
│   ├ params/
│   │ └ skgo_gen.go
│   ├ prerender/
│   │ └ skgo_gen.go
│   ├ config.go
│   └ skgo_gen.go
├ web/
│ ├ .storybook/
│ │ ├ main.ts
│ │ └ preview.ts
│ ├ .vscode/
│ │ └ extensions.json
│ ├ build/
│ │ └ .gitkeep
│ ├ patches/
│ │ └ skgo-kit-%kit%-queue.patch
│ ├ src/
│ │ ├ lib/
│ │ │ ├ assets/
│ │ │ │ └ favicon.svg
│ │ │ ├ vitest-examples/
│ │ │ └ index.ts
│ │ ├ routes/
│ │ │ ├ +layout.svelte
│ │ │ └ +page.svelte
│ │ ├ stories/
│ │ ├ app.d.ts
│ │ ├ app.html
│ │ └ hooks.server.ts
│ ├ static/
│ │ └ robots.txt
│ ├ .gitignore
│ ├ .npmrc
│ ├ README.md
│ ├ dist.go
│ ├ package.json
│ ├ pnpm-lock.yaml
│ ├ pnpm-workspace.yaml
│ ├ skgo.remotes.json
│ ├ tsconfig.json
│ └ vite.config.ts
├ .gitignore
├ Justfile
├ README.md
├ go.mod
├ go.sum
├ server.go
└ skgo.json
```

`sv` wrote `web/src`, `web/static` and the frontend configuration, as it does for any SvelteKit project; `web/src/lib/vitest-examples` and `web/src/stories` are the sample tests and stories that Vitest and Storybook install. Everything else is SKGo's, and the files named `skgo_gen.go`, `hooks.server.ts` and `skgo.remotes.json` are generated.

Code samples in these docs label files the way Kit's docs do, relative to `web/`: `src/routes/+page.svelte` is `web/src/routes/+page.svelte`. Go files outside `web/` are named from the project root, such as `internal/serverhooks/handle.go`.

## Project files

### go.mod

The application module. It requires `github.com/tylergannon/skgo`, declares `tool github.com/tylergannon/skgo/cmd/skgo` so that `go tool skgo` runs the project's own CLI version, and contains `ignore ./web/node_modules` so that `./...` does not walk into installed packages.

### skgo.json

The application's identity, `{"version":1,"name":"my-app"}`. [`skgo add`](/docs/cli/add) and templates read it.

### server.go

`NewHandler(dist fs.FS, origin string)` builds the production `http.Handler`: it reads the build manifest, registers the generated loads, remote functions, actions and endpoints, builds the renderer, and wraps the whole in your `handle` hook. `NewDevelopmentHandler` does the same for `skgo dev`, with Kit rendering the pages. Both call one private `newHandler`, and the file is yours to change. See Server composition.

### cmd/main.go

The binary's entry point. It serves `web.Build` through `NewHandler` and takes two flags: `-listen`, the address to listen on, and `-origin`, the public browser origin, both defaulting to the origin the project was created with. `just build` bakes that origin into the binary with `-ldflags`.

### internal/app

`locals.go` declares `Locals`, the Go counterpart of `App.Locals`, as an empty struct for you to fill, and `RequestEvent[P]`, the alias that binds it to `skgo.RequestEvent`. See [Hooks](https://svelte.dev/docs/kit/hooks).

### internal/serverhooks

`handle.go` declares `var Handle params.Middleware`, the `handle` hook. It is nil in a new project, and the generated server calls it when you assign one. See [Hooks](https://svelte.dev/docs/kit/hooks).

### internal/skgo

The generated bindings package. `config.go` holds the one line you own, the `go:generate` directive that runs `skgo generate` with this project's web directory, locals package and hook package; everything else in the directory is generated. `params/` is the typed route parameters and the hook types your code imports, `prerender/` is the program that answers Kit's prerender at build time, and `skgo_gen.go` is the registration `server.go` mounts. Once a route directory holds Go files, `links/` and `links.json` appear too: compilable copies of your route packages, under names Go can import.

### web

The SvelteKit project. It behaves like any Kit project, with `@skgo/sveltekit-adapter` configured in `vite.config.ts` as its adapter and remote functions turned on. `dist.go` embeds `build/` into the binary as `web.Build`, and `build/` is where `vp build` writes the client, the prerendered pages, the server-rendering bundle and the manifest Go reads. `skgo.remotes.json` lists the remote functions, loads and actions the generator found, for the adapter.

### web/src

Kit's `src` directory. Go files live beside the Svelte files they serve: `page.server.go`, `layout.server.go`, `server.go` and `*.remote.go` in route directories, and `hooks.go`, `params.go` and `env.go` at the top of `src` when you need transport, [param matchers](routing#Route-parameters-Matching) or [environment variables](environment-variables). Their TypeScript halves, `hooks.ts` and `params.ts`, stay where Kit expects them. `hooks.server.ts` is generated and empty; Kit discovers hooks from a physical entry, and the adapter supplies the server hooks.

### Justfile

`install`, `build`, `serve`, `test` and `storybook` recipes. `just build` is the project's build: `go generate ./...`, `vp build` in `web/`, then `go build` into `bin/`. There is no `dev` recipe; run `go tool skgo dev` from the project root. See Building your app.

## Generated files

`skgo generate`, which `go generate ./...` runs, writes:

- `skgo_gen.go` in each route directory with Go files, defining the typed event for that route, `PageRequestEvent` or `LayoutRequestEvent`, and its `RouteParams` or `LayoutParams`
- `+page.server.ts`, `+layout.server.ts`, `+server.ts` and `*.remote.ts` beside each Go file, the stubs Kit compiles
- `types.ts` for each Go package whose types reach the frontend: beside the package when it lives inside `web/src`, otherwise under `web/src/lib/skgo/`
- `src/params.ts` from `src/params.go`, and `src/env.ts` with `web/skgo.env.json` from `src/env.go`
- everything in `internal/skgo/` except `config.go`, and `internal/app/skgo_gen.go`

Generated files start with `Code generated by skgo. DO NOT EDIT.` Generation refuses to overwrite a `skgo_gen.go`, `hooks.server.ts` or `env.ts` that does not carry that header, so a file you wrote by hand under one of those names stops the run rather than disappearing. Generated code lists every output.

## Other files

The project `.gitignore` ignores `/bin/` and `/web/node_modules/`. `web/.gitignore` is `sv`'s, with `/build/*` and `!/build/.gitkeep` in place of `/build`: `.gitkeep` keeps the embedded directory present, because `//go:embed all:build` fails on a missing directory. Generated files are not ignored, so they are committed with your source. `web/pnpm-workspace.yaml` names the patch in `web/patches/` as a `patchedDependencies` entry for Kit %kit%, and VitePlus applies it during `vp install`; see [`skgo kit-patch`](/docs/cli/kit-patch). As in any Kit project, `web/.svelte-kit/` is Kit's own output and can be deleted at any time.
