---
title: skgo build
description: Build the frontend incrementally.
division: cli
section: Commands
order: 2035
---

Generates and builds the frontend only when its inputs changed. It does not compile the Go binary, so run `go build` after it. A new application's `just build` does not call it: it runs `go generate ./...`, `vp build` and `go build` every time.

## Usage

```text skgo build -h
```
