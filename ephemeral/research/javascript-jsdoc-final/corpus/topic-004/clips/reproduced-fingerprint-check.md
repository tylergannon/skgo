# Clip — reproduced fingerprint computation (this research run, not source text)

**Classification: REPRODUCTION / previously-reproduced observation.** Everything below
was computed during this research run on 2026-09-26 by re-implementing the algorithm
read in `identity.js` (sources/s02) and `fingerprintFS` (sources/s01) in Python over
files on disk. No skgo build, no `go test`, no npm build was run. The source files were
only read. Nothing in this clip is a quote of skgo source.

## What was computed

SHA-256 over: the bytes of `skgo-adapter.js`, then for each file under `skgo-adapter/`
in sorted order the bytes `<slash-path>\0` followed by the file bytes; first 12 hex
digits of the digest.

## Result 1 — the probe app's installed adapter

Tree: `/tmp/skgo-jsdoc-probe-20260926/web/node_modules/@skgo/sveltekit-adapter`
(symlink resolves into `.pnpm/@skgo+sveltekit-adapter@0.9.0_...`).

- files hashed: `skgo-adapter/app-paths.js`, `skgo-adapter/app-server.js`,
  `skgo-adapter/entry.js`, `skgo-adapter/env.js`, `skgo-adapter/identity.js`,
  `skgo-adapter/polyfill.js`
- computed fingerprint: `17f12e0497d6`
- `package.json` name/version read: `@skgo/sveltekit-adapter` `0.9.0`
- manifest field written by that build (sources/s08): `"skgoAdapter": "17f12e0497d6"`

→ The algorithm as read in source reproduces the value the adapter stamped into the
built manifest. The demo probe app's manifest carries the same `17f12e0497d6`.

## Result 2 — this worktree's own adapter package

Tree: `/Users/tyler/.codex/worktrees/ef7a/skgo/internal/adapter` (the `//go:embed`
root, sources/s01:44).

- same six runtime files, same sorted paths
- computed fingerprint: `17f12e0497d6`
- `ls` of the probe's installed `skgo-adapter/` and this worktree's
  `internal/adapter/skgo-adapter/` produce identical listings

→ On 2026-09-26 the worktree's embedded adapter and the adapter installed in both
probe apps fingerprint identically, so a manifest from either probe build would pass
the `m.SkgoAdapter != adapter.Fingerprint()` comparison at remote.go:507 for this
worktree's binary. The manifest's `skgo` value (`0.9.0`) is *not* part of that
comparison; only the fingerprint is.

## Limits of this reproduction

- It does not prove what `adapter.Fingerprint()` returns at runtime — that was not
  executed. It shows the documented algorithm applied to the same bytes yields the
  recorded value.
- It says nothing about generated app files: the computation never opens a route stub,
  `skgo.remotes.json`, or anything outside `skgo-adapter.js` + `skgo-adapter/`.
- The two probes' `node_modules` trees were inspected read-only; nothing was rebuilt.
