---
title: "skgo: environment"
summary: Environment and LoadEnvironment, typed environment variables declared in Go.
---

Kit's `$env` modules, declared in Go. A struct marked with `Environment`
names the variables the application needs and their types; `skgo generate`
writes the declaration the `skgo env` command checks and the typed accessors
the Svelte side imports. `LoadEnvironment` reads and validates the values at
startup and returns the snapshot the server keeps.

> PACKAGE: github.com/tylergannon/skgo
> SYMBOLS: Environment LoadEnvironment EnvironmentSnapshot
