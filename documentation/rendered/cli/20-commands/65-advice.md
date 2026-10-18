---
title: "skgo advice"
summary: "Show the analyzer rules"
---

Each rule `skgo check` and `go vet` enforce has a code, a consequence, a repair and an example. This prints them, for a person or an agent that hit one.

```sh
skgo advice [flags] [SKGO001..SKGO008]
```

Prints the analyzer rules this executable enforces, each with its consequence,
repair and an example of valid skgo code. Name one code to see one rule.

| Flag | Default | Description |
|---|---|---|
| `--json` | `false` | write structured JSON guidance |
