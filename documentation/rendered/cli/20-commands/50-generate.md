---
title: "skgo generate"
summary: "Generate the glue"
---

The command behind every remote module, page server stub and `skgo_gen.go` in an application. It runs from the `go:generate` directive in the bindings package and again on every change under `skgo dev`.

SKGo selects TypeScript when `tsconfig.json` exists, or JavaScript with JSDoc when only `jsconfig.json` exists. To migrate the generated modules, replace one configuration with the other, keeping Kit's configuration settings, and run `go generate ./...`. Regeneration replaces obsolete SKGo and Polytype modules from the previous language and updates the remote identities and module paths; authored files and other generators' files stay intact. Convert any authored TypeScript separately when moving to JavaScript.

The generated `src/skgo.versions.ts` (or `.js`) exports the qualified toolchain as `versions`. Check it into source control and regenerate it with the application's matching SKGo tool after adopting a release. See [Qualified toolchain](/docs/skgo/skgo-toolchain).

```sh
skgo generate [flags]
```

Reads every `*.remote.go`, `page.server.go`, `layout.server.go` and
`server.go` under the application's `src/routes` and writes the
`.remote.ts` modules Kit compiles, the TypeScript declarations their callers
see, and the Go registration the server mounts. New applications run it from a
`go:generate` directive in the bindings package:

```go
//go:generate go tool skgo generate --web ../web
```

| Flag | Default | Description |
|---|---|---|
| `--web` | `../web` | the vite root: the directory holding src/ and package.json |
| `--out` | `.` | the directory of the generated bindings package |
| `--package` |  | the name of the generated bindings package; defaults to the base name of --out |
| `--locals-package` |  | application locals Go import path (required) |
| `--locals-type` | `Locals` | application locals named struct |
| `--hook-package` |  | optional request hook Go import path |
| `--hook-symbol` | `Handle` | selected request hook symbol |
| `--cache-dir` |  | generation cache directory; defaults to the user cache; off disables caching |
| `--quiet` | `false` | do not list the files written |
| `--swift-out` |  | generated Swift source file |
| `--swift-remote` |  | select module#export for Swift and the shared safe-number contract (ordinary query/command; repeatable) |
