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
brew install tylergannon/tap/skgo
```

```sh
go install github.com/tylergannon/skgo/cmd/skgo@latest
```

Inside an application, `go tool skgo` runs the version the application's
`go.mod` pins, which is the one `skgo generate` directives use. `skgo
buildinfo` says which binary is answering.

## Commands

`skgo --help` lists every command, followed by the templates and add-ons this
binary knows:

```text
usage:
	skgo new [flags] DIR [-- SV_CREATE_OPTIONS]
		create a SvelteKit application served by Go
	skgo plugin install MODULE[@QUERY] | remove MODULE
		install or remove trusted plugin source for this executable
	skgo add [flags] ADDON
		apply a reusable application change
	skgo update [flags]
		align CLI and application toolchain
	skgo native build [flags]
		build the macOS or iOS application
	skgo build [flags]
		incrementally generate and build the frontend
	skgo buildinfo [flags]
		identify this executing CLI and its Go build
	skgo dev [flags]
		develop with Go and Vite
	skgo generate [flags]
		generate the glue between the Go server and the SvelteKit app
	skgo check [flags]
		check Go, Svelte, lint and formatting without edits
	skgo kit-patch [flags] (--apply | --check)
		configure or verify Kit's declared-Inputs queue correction
	skgo advice [flags] [SKGO001..SKGO008]
		show installed rule guidance and repair examples
	skgo env [flags]
		resolve declared environment variables from JSON on stdin
	skgo mcp
		serve check and advice tools over stdio MCP

Run skgo COMMAND --help for the command's flags.
```

Every command accepts `--help` and prints its flags. The pages in this
section show that output for each command; the binary you run is the
authority for its own version.
