---
title: "skgo dev"
summary: "Run the development server"
---

One listener serves the Go application with hot reload and Vite's dev server. Edits to Go files regenerate and rebuild the server; edits to Svelte files reach the browser through Vite as usual.

```sh
skgo dev [flags]
```

Runs the development server: the Go application with hot reload of the Go
side and Vite serving the frontend, on one public listener. Generation runs
again whenever a `*.remote.go` or load file changes.

| Flag | Default | Description |
|---|---|---|
| `--root` | `.` | application root containing go.mod |
| `--web` | `web` | frontend root relative to --root |
| `--out` |  | generated bindings directory; detected when omitted |
| `--listen` | `127.0.0.1:8080` | public listener address (next free port when occupied) |
| `--strict-port` | `false` | fail when the requested port is occupied |
| `--go` | `go` | Go executable |
| `--generator-config` |  | optional application generator JSON configuration, relative to --root |
| `--vp` | `vp` | VitePlus executable |
