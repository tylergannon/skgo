---
title: "skgo add"
summary: "Apply an add-on to an application"
---

An add-on is a reusable change to an existing application: the built-in `native-app` add-on, or one a plugin provides. `skgo add --help` lists what this binary knows.

```sh
skgo add [flags] ADDON
```

Applies an add-on registered by an installed plugin to the application at
`--root`. `skgo add --help` lists the add-ons the executable knows.

| Flag | Default | Description |
|---|---|---|
| `--root` | `.` | skgo application root |
| `--name` |  | explicit application identity for older projects; must match an existing identity |
| `--set` |  | add-on option NAME=VALUE (repeatable) |
