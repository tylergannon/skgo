---
title: "skgo env"
summary: "Resolve declared environment variables"
---

Takes the generated environment declaration and a JSON object of raw values on stdin, and writes the resolved, validated values. Deployment tooling runs it before starting the server.

```sh
skgo env [flags]
```

Reads the generated environment declaration and a JSON object of raw values
on stdin, applies defaults and validation, and writes the resolved values as
JSON. Deployment tooling uses it to check an environment before starting the
server.

| Flag | Default | Description |
|---|---|---|
| `--schema` | `skgo.env.json` | generated environment declaration |
