---
title: skgo build
description: Build the frontend incrementally.
division: cli
section: Commands
order: 2035
---

Generates and builds the frontend only when its inputs changed. The Justfile's `build` recipe calls it before `go build`, so a plain `just build` is always up to date.

## Usage

```text skgo build -h
```
