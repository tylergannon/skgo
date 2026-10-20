---
title: Overview
description: The skgo command, what it owns and what it leaves to vp and go.
division: cli
section: Introduction
slug: overview
order: 1010
---

`skgo` is one Go binary. It creates applications, generates the glue between
the Go server and the SvelteKit app, runs the development server, builds the
frontend, and checks the project. It leaves the frontend tool environment to
VitePlus (`vp`) and the Go build to `go build`; a new application's Justfile
chains them.

Install it with Homebrew, or build it from the module:

```sh
brew install --cask tylergannon/skgo/skgo
```

```sh
go install github.com/tylergannon/skgo/cmd/skgo@latest
```

Inside an application, `go tool skgo` runs the version the application's
`go.mod` pins, which is the one `skgo generate` directives use. `skgo
buildinfo` says which binary is answering.

## Commands

`skgo -h` lists every command, followed by the templates and add-ons this
binary knows:

```text skgo -h
```

Every command accepts `-h` or `--help`, prints its help and exits with status 0. The pages
in this section show that output for each command; the binary you run is the
authority for its own version.
