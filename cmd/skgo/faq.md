---
title: Frequently asked questions
description: Package managers, refused sv options and where the CLI keeps its caches.
division: cli
section: Introduction
order: 1020
---

## Why can't I choose a package manager?

SKGo qualifies one VitePlus release with one pnpm, and runs every frontend command through `vp`, which resolves that pnpm itself. `skgo new` refuses `sv create`'s `--install` and `--no-install` because VitePlus does the install, and `@skgo/sv` refuses to run under any other VitePlus. Install VitePlus at the pinned version and do not select pnpm yourself.

## Why does `skgo new` refuse some sv options?

An SKGo application's server is Go, and its frontend always lives in `web/`. So [`skgo new`](/docs/cli/new) refuses what cannot become such an application: the `library` and `addon` templates, `--from-playground` and `--addon-name`, and any add-on that writes JavaScript server code. [sv add-ons](/docs/cli/sv-add-ons) lists which official add-ons fit.

## Where does SKGo cache things?

| Location | Contents | Reset |
| --- | --- | --- |
| `skgo/generate` under the user cache directory, or `SKGO_CACHE_DIR`, or `--cache-dir` | `skgo generate` plans | `--cache-dir off`, or delete it |
| `.skgo/` in the application root | the freshness record `skgo build` keeps for the frontend | delete `.skgo/` |
| `native/build/` | the native-shell plugin's build output and compiler caches | delete `native/build/` |
| `~/.skgo/plugins/` | plugins you place by hand, managed installs under `hosts/<executable-sha256>/`, and prebuilt downloads under `prebuilt/<executable-sha256>/` | `skgo plugin remove MODULE` for a managed install; delete a manual `.so` yourself |

None of these is needed to run a built application. The `.gitignore` a new application gets covers `bin/`, `.skgo/` and `web/node_modules/`.
