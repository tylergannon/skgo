---
title: skgo generate
summary: Generate the glue
proof:
  - TestApplicationLanguageMigrationRoundTrip
  - TestLanguageCleanupRespectsGeneratorOwnership
---

The command behind every remote module, page server stub and `skgo_gen.go` in an application. It runs from the `go:generate` directive in the bindings package and again on every change under `skgo dev`.

SKGo selects TypeScript when `tsconfig.json` exists, or JavaScript with JSDoc when only `jsconfig.json` exists. To migrate the generated modules, replace one configuration with the other, keeping Kit's configuration settings, and run `go generate ./...`. Regeneration replaces obsolete SKGo and Polytype modules from the previous language and updates the remote identities and module paths; authored files and other generators' files stay intact. Convert any authored TypeScript separately when moving to JavaScript.

The generated `src/skgo.versions.ts` (or `.js`) exports the qualified toolchain as `versions`. Check it into source control and regenerate it with the application's matching SKGo tool after adopting a release. See [Qualified toolchain](/docs/skgo/skgo-toolchain).

> COMMAND: skgo generate
