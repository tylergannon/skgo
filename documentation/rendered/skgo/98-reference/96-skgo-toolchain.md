---
title: "Qualified toolchain"
summary: "Read the versions qualified by your adopted SKGo module."
---

Use `skgo.Toolchain()` to read the versions qualified by your application's SKGo module. The Go value is the qualified release, and the Node value is the supported range.

`skgo generate` writes the same six fields as `versions` in `src/skgo.versions.ts`, or `src/skgo.versions.js` for a JavaScript application. Import `versions` into your frontend and check the generated module into source control as a build input.

After adopting a release with `go get github.com/tylergannon/skgo@<version>`, run generation with the application's matching tool, normally `go generate ./...`. A generator with a different release identity or qualified pin source refuses generation instead of mixing versions. Checkouts and replaced modules report `devel`.

## ToolchainVersions

```go
type ToolchainVersions struct {
	SKGo     string
	Go       string
	Kit      string
	VitePlus string
	PNPM     string
	Node     string
}
```

ToolchainVersions describes the toolchain qualified by the adopted SKGo module. Go is the qualified pin, rather than the running binary's compiler; Node is the supported range, rather than the installed Node version.

### Toolchain

```go
func Toolchain() ToolchainVersions
```

Toolchain returns the versions qualified by this application's SKGo module. SKGo comes from Go build information: it is the module version for a released dependency, or "devel" for a checkout, replaced module or unavailable build information. It never reports the application's own version as SKGo.

skgo generate exports the same values as versions in src/skgo.versions.ts (or .js in a JavaScript application). Check that generated build input into source control and regenerate it when adopting a new SKGo release.
