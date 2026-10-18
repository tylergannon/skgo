---
title: Frequently asked questions
summary: Package managers, refused sv options and where the CLI keeps its caches.
---

## Why can't I choose a package manager?

SKGo qualifies one VitePlus release, %vp%, with one pnpm, %pnpm%, and runs every frontend command through `vp`, which resolves that pnpm itself. `skgo new` refuses `sv create`'s `--install` and `--no-install` because VitePlus does the install, and `@skgo/sv` refuses to run under any other VitePlus. Install VitePlus at the pinned version and do not select pnpm yourself.

## Why does `skgo new` refuse some sv options?

An SKGo application's server is Go, and its frontend always lives in `web/`. So [`skgo new`](new) refuses what cannot become such an application: the `library` and `addon` templates, `--from-playground` and `--addon-name`, and any add-on that writes JavaScript server code. [sv add-ons](sv-add-ons) lists which official add-ons fit.

## Where does SKGo cache things?

| Location | Contents | Reset |
| --- | --- | --- |
| `skgo/generate` under the user cache directory, or `SKGO_CACHE_DIR`, or `--cache-dir` | `skgo generate` plans | `--cache-dir off`, or delete it |
| `.skgo/` in the application root | the freshness record `skgo build` keeps for the frontend | delete `.skgo/` |
| `native/build/` | native build output and compiler caches | delete `native/build/` |
| `~/.skgo/plugins/` | plugins you place by hand, and managed installs under `hosts/<executable-sha256>/` | `skgo plugin remove MODULE` for a managed install; delete a manual `.so` yourself |

None of these is needed to run a built application. The `.gitignore` a new application gets covers `bin/` and `web/node_modules/` but not `.skgo/`, so add that line yourself.
