---
title: Overview
summary: The skgo command, what it owns and what it leaves to vp and go.
---

`skgo` is one Go binary. It creates applications, generates the glue between
the Go server and the SvelteKit app, runs the development server, builds the
frontend, and checks the project. It leaves the frontend tool environment to
VitePlus (`vp`) and the Go build to `go build`; a new application's Justfile
chains them.

Install it with Homebrew, or build it from the module:

```sh
brew install tylergannon/tap/skgo
```

```sh
go install github.com/tylergannon/skgo/cmd/skgo@latest
```

Inside an application, `go tool skgo` runs the version the application's
`go.mod` pins, which is the one `skgo generate` directives use. `skgo
buildinfo` says which binary is answering.

## Commands

> COMMANDS:

Every command accepts `--help` and prints its flags. The pages in this
section are rendered from the same table the command prints, so they cannot
disagree with the binary at the version this documentation describes.
