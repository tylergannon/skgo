---
title: skgo build
summary: Build the frontend incrementally
---

Generates and builds the frontend only when its inputs changed. The Justfile's `build` recipe calls it before `go build`, so a plain `just build` is always up to date.

> COMMAND: skgo build
