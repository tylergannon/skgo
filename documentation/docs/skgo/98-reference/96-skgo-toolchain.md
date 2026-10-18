---
title: Qualified toolchain
summary: Read the versions qualified by your adopted SKGo module.
proof:
  - TestQualifiedVersionsConsumerAndGeneration
  - TestVersionIdentifiesAdoptedSKGo
---

Use `skgo.Toolchain()` to read the versions qualified by your application's SKGo module. The Go value is the qualified release, and the Node value is the supported range.

`skgo generate` writes the same six fields as `versions` in `src/skgo.versions.ts`, or `src/skgo.versions.js` for a JavaScript application. Import `versions` into your frontend and check the generated module into source control as a build input.

After adopting a release with `go get github.com/tylergannon/skgo@<version>`, run generation with the application's matching tool, normally `go generate ./...`. A generator with a different release identity or qualified pin source refuses generation instead of mixing versions. Checkouts and replaced modules report `devel`.

> PACKAGE: github.com/tylergannon/skgo
> SYMBOLS: ToolchainVersions
