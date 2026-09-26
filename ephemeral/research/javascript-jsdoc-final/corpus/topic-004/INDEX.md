# Topic-004 — Adapter fingerprint, manifest identity, and the checkInstalledAdapter gate

Evidence root: `corpus/topic-004/`. Originals read from the worktree
`/Users/tyler/.codex/worktrees/ef7a/skgo` at git
`657656d1161edf832eb99b69cc1636e37877144b` (main), retrieved 2026-09-26; probe
artifacts read read-only from `/tmp/skgo-jsdoc-*-20260926`. No build or test was run.
Line ranges below are the originals' own lines; the local file that carries the
verbatim text is named in each bullet.

## Question → evidence

**Q1 — How `Fingerprint`/`FingerprintOf` compute identity; what bytes they hash**
- `sources/s01` = `internal/adapter/adapter.go:78-164`. `Fingerprint()` at
  `adapter.go:87-97` memoizes `fingerprintFS(files)`; `FingerprintOf(dir)` at
  `adapter.go:104-109` is `fingerprintFS(os.DirFS(dir))`.
- The one definition, `fingerprintFS` (`adapter.go:131-164`): SHA-256 over the bytes of
  `entryFile` = `"skgo-adapter.js"` (`adapter.go:52`, read at `:133`), then every file
  under `filesDir` = `"skgo-adapter"` (`adapter.go:53`) found by `fs.WalkDir`, sorted
  (`:152`), each written as `path.Clean(name)+"\x00"` then its bytes (`:160-161`), and
  `hex.EncodeToString(...)[:12]` returned at `:163`. Package doc `adapter.go:78-86`
  states every adapter file goes in, "not only the entry". Embedded root:
  `//go:embed skgo-adapter.js skgo-adapter` (`adapter.go:44`).
- JS mirror: `sources/s02` = `internal/adapter/skgo-adapter/identity.js:1-75`
  (`createHash('sha256')` at `:64`, entry at `:65`, sorted `runtimeFiles` at `:63-61`,
  `name+'\0'` at `:69`, `.slice(0, 12)` at `:74`).
- Reproduction (not source): `clips/reproduced-fingerprint-check.md`.

**Q2 — What `Identity` produces, and how it is embedded in the manifest**
- `sources/s01` = `adapter.go:203-213`: `Identity(version, fingerprint)` returns a
  *display string* — `"an adapter too old to record which skgo it came from"` when the
  fingerprint is empty, `"an unnamed skgo (adapter <fp>)"` when the version is empty,
  else `"skgo <version> (adapter <fingerprint>)"`.
- It is **not** embedded in the manifest. The manifest carries the two halves
  separately: `sources/s02` = `skgo-adapter.js:118-140`, with `skgo: SKGO.version,
  skgoAdapter: SKGO.adapter` at `skgo-adapter.js:126-127` and `const SKGO = identity()`
  at `:21`; `identity()` returns `{version: pkg.version, adapter: <12-hex>}`
  (`identity.js:63-74`). Go models them as `static.go:22-32` (`Skgo` json `skgo`,
  `SkgoAdapter` json `skgoAdapter`), quoted in `sources/s03`.
- `Identity()` is used only to format refusals: `internal/gen/adapter.go:67-68`,
  `remote.go:518`. Observed values: `sources/s08` (probe manifests,
  `"skgo": "0.9.0"`, `"skgoAdapter": "17f12e0497d6"`).

**Q3 — How `ReadManifest` validates identity; the mismatch error**
- `sources/s03` = `remote.go:484-527`. Read at `remote.go:500`
  (`fs.ReadFile(build, "skgo.manifest.json")`), parse error at `:503-506`, gate at
  `:507` — `if m.SkgoAdapter != adapter.Fingerprint()` (fingerprint only; `m.Skgo` is
  not compared). Error returned at `:512-518`:
  `"skgo: the frontend build and this program come from different skgo versions.\n\tthe @skgo/sveltekit-adapter that built it: %s\n\tthis program:                    %s\nThe adapter and the Go that reads what it writes are one contract, so they have to be the same skgo.\nInstall the one this program publishes — \`%s\` — then run \`go generate ./...\` and build the frontend again."`
- Callers: `example/server.go:106` (`NewHandler`), `document.go:241` (`ReadDevManifest`).
- Tests pinning the message contents: `sources/s05` =
  `manifest_version_test.go:104-135, 123-135, 137-145`.

**Q4 — What `checkInstalledAdapter` compares; exact error text**
- `sources/s04` = `internal/gen/adapter.go:1-70`. Directory at `:32`
  (`<cfg.Web>/node_modules/@skgo/sveltekit-adapter`), `FingerprintOf` at `:34`,
  `fs.ErrNotExist → nil` at `:35-37`, equality against `adapter.Fingerprint()` at `:41`,
  version only for the message at `:48`. Error built at `:58-69`:
  `"skgo: the %s installed in this app is not the one this skgo publishes.\n\tinstalled in %s: %s\n\tthis program:  %s\nThey are one contract in two languages, released together, so the frontend this would build could not be served by the program that generated it.\nInstall the matching one — %s — and run \`go generate ./...\` again."`
- Call sites: `internal/gen/gen.go:77`, `internal/gen/check.go:27` (both in `sources/s04`).
- Tests: `sources/s05` = `internal/gen/adapter_test.go:45-71` (refusal names
  `theirVersion, adapter.Fingerprint(), adapter.Version(), adapter.Package, "pnpm add"`),
  `:106-141` (this module's adapter allowed), `:73-104` (empty app needs none).

**Q5 — Where the manifest is written, and where the runtime reads it**
- Written during `adapt`: `sources/s06` = `skgo-adapter.js:118-140`,
  `write(\`${out}/skgo.manifest.json\`, ...)` at `:120-121`; `out` defaults to `'build'`
  at `skgo-adapter.js:51`. (`example/web/vite.config.ts:8` calls `skgo()` with no `out`.)
- Read path: `skgo.manifest.json` at the root of the build FS
  (`remote.go:500`; also `static.go:330`). Example wiring, `sources/s06`:
  `//go:embed all:build` at `example/web/dist.go:15-16`, `fs.Sub(web.Build, "build")`
  at `example/cmd/main.go:35`, `skgo.ReadManifest(dist)` at `example/server.go:106`.

**Q6 — Does the hash cover only adapter package files, or generated app files too**
- **Only adapter package files.** `fingerprintFS` opens exactly two roots —
  `entryFile` and the `filesDir` walk (`adapter.go:131-164`); `package.json` is never
  hashed (it is read only for the version, `adapter.go:113-125` /
  `identity.js:72-73`). `sources/s01`, `sources/s02`, `sources/s07`
  (= `internal/adapter/package.json`, `package_test.go:20-40` shipping list).
- Generated app files are covered by **separate comparisons**, not the fingerprint:
  `readGenerated` validating `skgo.remotes.json` (`sources/s09` =
  `skgo-adapter.js:198-262`), `checkRemoteHashes` (`:653`) and `checkRemoteIds`
  (`:701`) in `sources/s07`, `checkServerLoads` (`:756`) in `sources/s09`.
- Reproduced check of both halves: `clips/reproduced-fingerprint-check.md`.

## Supported facts
1. The fingerprint is SHA-256 over `skgo-adapter.js` + sorted `skgo-adapter/*` files,
   name-and-bytes per file, truncated to 12 hex chars (`adapter.go:131-164`,
   `identity.js:63-74`).
2. Two gates compare it: at generate time (`internal/gen/adapter.go:41`) and at
   startup (`remote.go:507`); the startup one is documented as unbypassable
   (`internal/gen/adapter.go:15-19`).
3. The manifest stores `skgo` and `skgoAdapter` as separate fields; `Identity()` is
   formatting for error text only (`skgo-adapter.js:126-127`, `static.go:27-32`).
4. `NewStaticHandler` reads `skgo.manifest.json` (`static.go:330`) without comparing
   `SkgoAdapter`; in `example/server.go:106` `ReadManifest` runs first, so the gate
   still applies on that path.
5. Generated app files are not in the hash; they are checked by set comparison against
   kit's compiled output.

## Inference (not stated by a source)
- `NewStaticHandler` called without a prior `ReadManifest` would serve an
  unverified adapter manifest; nothing inside it enforces the identity.
- No manifest field or runtime check found here keys off the extension of generated
  stubs — but `readGenerated` does enforce `.server.ts` on the `loads`/`actions` lists
  in `skgo.remotes.json` (`skgo-adapter.js:235, 245`), so a JavaScript-form emit would
  have to satisfy or change those regexes.

## Contradictions / tensions
- Two different messages for one failure class: generate-time "is not the one this
  skgo publishes" (`internal/gen/adapter.go:59`) vs startup "come from different skgo
  versions" (`remote.go:513`).
- `adapter.Version()` may be `"devel"` while the installed package reports `0.9.0`
  (`sources/s08`); only the fingerprint is compared, so a matching fingerprint with a
  differing version string passes both gates.

## Unresolved / gaps
- What must change in the manifest or a runtime check before JavaScript-form output is
  accepted: no version field, extension allowlist, or language marker exists in the
  identity path today — confirming absence across all of `internal/gen` was outside
  this topic's reads (only `adapter.go`/`check.go`/`gen.go` call sites were read).
- Whether `nodes`/`loads`/`actions` strings would change under `.js` stubs: their
  values come from kit's `server_id` (`sources/s09`, `skgo-adapter.js:496-506`), not
  from a skgo constant — but how `skgo generate` writes them was not read here.
- Whether any consumer besides `ReadManifest`/`checkInstalledAdapter` validates
  `SkgoAdapter`: grep of `SkgoAdapter` found only `remote.go:507,518` and
  `static.go:27,32`; `NewStaticHandler` is the one reader that skips it.

## Research floor
9 sources (`sources/s01`–`s09`) + 1 clip; minimum was 1. All six questions have
original-source line ranges above; citation targets were re-read to confirm the line
numbers resolve.
