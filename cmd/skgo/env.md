---
title: skgo env
description: Resolve declared environment variables.
division: cli
section: Commands
order: 2070
---

Takes the generated environment declaration and a JSON object of raw values on stdin, and writes the resolved, validated values. Deployment tooling runs it before starting the server.

## Usage

`skgo env --help` prints:

```text
Usage of env:
  -schema string
    	generated environment declaration (default "skgo.env.json")
```
