# Topic 005 — Probe app artifacts: what was built, what works, what is unproven

Assigned group: **Existing JavaScript app probe results and required future proof.**
Evidence retrieved 2026-09-26 by read-only inspection of three disposable app
trees, the pinned kit source, and the current skgo worktree. **No build or test
was run in this session**; anything a previous command reported is labelled
*previously observed* and, where no exit status or output file survives,
carried as unresolved.

## Source files

| # | File | What it is |
|---|------|------------|
| S1 | [`sources/probe-route-inventories.md`](sources/probe-route-inventories.md) | Verbatim route trees, `*.ts`/`*.js` inventories, and the full text of the generated stubs in all three apps |
| S2 | [`sources/demo-client-and-server-remote-resolution.md`](sources/demo-client-and-server-remote-resolution.md) | Shipped client entry, route node, shared chunk and server manifest of the demo probe — how a remote id is resolved at runtime |
| S3 | [`sources/jsproof-js-remote-and-hash-gate.md`](sources/jsproof-js-remote-and-hash-gate.md) | JS-only probe: `example.remote.js` compiling under kit, hash `1ptltty`→`2b61k`, mtimes, and the operator-reported outcomes |
| S4 | [`sources/adapter-check-and-manifest-source.md`](sources/adapter-check-and-manifest-source.md) | Worktree source of the adapter's hash/id checks, manifest write, `checkInstalledAdapter`, `kithash`, and pinned kit@3.0.0-next.28 `remote_module_pattern` |
| S5 | [`sources/logs-and-absences.md`](sources/logs-and-absences.md) | Full inventory of logs/screenshots/test-output in the three trees and what the one log actually contains |

---

## Q1 — Route files and remote declarations in `/tmp/skgo-jsdoc-probe-20260926/web/src/routes`

**Answer: the probe app declares no remote functions at all, and nothing is `.js`.**

- S1, "probe route tree": only `+layout.svelte` and `+page.svelte`. No
  `.remote.*`, no `+page.server.*`, no `+server.*`, no `.go`.
- S1, probe `skgo.remotes.json` and `build/skgo.manifest.json` lines 114-118:
  `"remotes": []`. S1, probe `skgo_bindings_gen.go`: `Remotes()` returns an
  empty slice.
- The remote declarations in this experiment live in the **demo** app instead:
  S1, demo route listing — `example.remote.ts` (TypeScript) plus
  `example.remote.go`, `types.ts`. No `.remote.js` and no
  `+page.server.{ts,js}` anywhere in either of the first two apps.

*Supported by*: S1 (whole file). *Inference*: none needed.

## Q2 — What `entry/app.*.js` reveals about runtime remote resolution

**Answer: the app entry names no remotes; the client resolves a remote by
string id over HTTP, while the server resolves it by hash-keyed chunk import.**

- S2 §1, `build/client/_app/immutable/entry/app.DyKfhsWA.js` full text: exports
  are `decode, decoders, dictionary, encoders, get_error_template, hash, hooks,
  matchers, nodes, server_loads`; `server_loads` is `[]`; no `remotes` key, no
  remote id, no remote chunk import. `entry/start.VuppPwFt.js` contains no
  `remote` substring.
- S2 §2, `nodes/2.CXgr7ugK.js`: ids appear as literals
  `` ge(`1ptltty/record`) `` / `` ye(`1ptltty/status`) ``, and the request URL
  is `` `${D}/${M}/remote/${e}` ``. S2 §3: `D` = payload base, `M` = `'_app'` →
  `` `${base}/${appDir}/remote/<hash>/<name>` ``.
- S2 §4, **server** `manifest.js` lines 19-21:
  `remotes: { '1ptltty': __memo(() => import('./chunks/remote-1ptltty.js')) }`,
  and `chunks/example.remote.js` line 37:
  `init_remote_functions(example_remote_exports, "src/routes/example.remote.ts", "1ptltty")`.
- Corroborated by skgo's own adapter doc comment (S4, lines 677-688) which
  cites kit's client runtime for the same URL shape.

*Contradiction check*: the question's premise that the *app entry* reveals the
chunk resolution is itself not supported — the entry is silent; resolution
split between client id-literal→HTTP and server hash→chunk is the observed
behaviour. The adapter's doc comment (S4) states the same thing from source.

## Q3 — Existing probe logs, screenshots, or test-output files

**Answer: none record probe observations.**

- S5 §1: the only `.log` in any of the three trees is `web/debug-storybook.log`
  (~3 KB each). No screenshots, no `test-results/`, no captured check/build
  output.
- S5 §2: its contents are Storybook initialisation lines only.
- S5 §4: consequently every "previously ran and printed X" claim about these
  apps is **unresolved** — see *Previously observed* below.

## Q4 — Which generated files still carry `.ts`, and what must change for valid JS

**Answer: in the demo probe the generated pair is `example.remote.ts` +
`types.ts`; the JS-only probe replaces them with `.js` equivalents whose only
difference is TypeScript syntax vs JSDoc.**

- S1, demo `.ts` inventory: `src/routes/example.remote.ts`,
  `src/routes/types.ts` (plus `src/app.d.ts`).
- S1, demo `example.remote.ts` full text — the TS-only constructs are
  `import type { Status }`, `export type { Status }`,
  `(): never =>`, `(_arg: string): Status`, `(): Status`.
- S1, jsproof `example.remote.js` full text — the same module as JavaScript with
  `/** @typedef {import('./types.js').Status} Status */`,
  `/** @returns {never} */`,
  `/** @type {import('$app/server').RemoteCommand<string, Status>} */` and
  `/** @param {string} _arg @returns {Status} */`.
- S1, jsproof `types.js`: a `@typedef` object literal replacing
  `export type Status = {...}`.
- S1, jsproof `skgo_remotes_gen.go` still *comments* `src/routes/example.remote.ts#record`
  while `internal/skgo/skgo_bindings_gen.go` lines 66/74 spell
  `Module: "src/routes/example.remote.js"` — a generated-comment/path mismatch
  still present on disk.

*Supported*: file contents. *Inference*: that a `.js` stub is "valid" for kit is
supported independently by S4 (kit's `remote_module_pattern` accepts any
extension) and by the jsproof build residue in S3 §1.

## Q5 — Known failure mode for a `.js` stub where kit expects `.ts`, and captured evidence

**Answer: the failure that was hit is the adapter's remote-hash cross-check, not
kit refusing `.js`. No output file captures it.**

- *Previously observed (operator report, 2026-09-26; exit status NOT preserved,
  no stdout file)*: on the jsproof app `pnpm run check` **passed**;
  `record(123)` **failed as desired**; `pnpm run build` **compiled the JS
  remote** but the adapter **rejected "old versus new remote path hash"**.
  Treat all three as unverified prior observations — S3 §4.
- Residue that *is* on disk (S3 §1-§3): kit compiled
  `src/routes/example.remote.js` under hash `2b61k`; the client bundle carries
  `2b61k/record` and `2b61k/status`; the earlier TS build of the same app used
  `1ptltty` (S2 §4). `skgo.remotes.json` and the bindings were regenerated at
  11:58:22, six seconds before the build artifacts at 11:58:28.
- The mechanism (S4): `checkRemoteHashes` compares the hash prefixes in
  `skgo.remotes.json` against kit's `manifest.remotes` keys and throws
  `generated but not compiled: … / compiled but not generated: …` on any
  mismatch; it runs before anything is written to `out`, so a rejection leaves
  no `build/skgo.manifest.json`.
- *Inference (not proof)*: a `.js` remote compiled by kit while
  `skgo.remotes.json` still listed the `.ts` path hash prints exactly
  `1ptltty` vs `2b61k` — that is the "old versus new" rejection. The literal
  error text from the run was **not captured** (S5 §4), so the identity of the
  thrown message remains unresolved.

**Not** a failure mode: kit itself. S4 —
`remote_module_pattern = /[/.]remote\.[^/]+$/` matches `example.remote.js`, and
S3 §1 shows such a module compiling to `chunks/remote-2b61k.js`.

## Q6 — Future-proofing required before JavaScript-form output is accepted

**Answer: three extension-sensitive surfaces exist today; only two are gates.**

1. **Adapter identity (already a gate, extension-blind).** S4: the manifest
   carries `skgo: SKGO.version` and `skgoAdapter: SKGO.adapter` (adapter
   fingerprint over its own files); `internal/gen/adapter.go:31`
   `checkInstalledAdapter` compares `adapter.FingerprintOf(installed)` with
   `adapter.Fingerprint()` and errors
   `skgo: the %s installed in this app is not the one this skgo publishes.…`;
   the unbypassable gate is `skgo.ReadManifest` (`remote.go:498`) at startup
   (S4, comment lines 12-30). This covers *which adapter wrote the build*, not
   *what extensions the generator emitted*.
2. **Remote-hash cross-check (gate, extension-sensitive only through the path).**
   S4: `checkRemoteHashes` compares hashes, not extensions; hashes are over the
   full module path including extension (`internal/kithash`, S4), so the `.ts`
   → `.js` switch necessarily changes the id on both sides at once. Anything
   that emits one side before the other fails here first. Its remediation
   sentence hard-codes `A generated .remote.ts …` (S4 lines 653-673) — wording,
   not enforcement.
3. **`skgo.remotes.json` shape validation (`readGenerated`) — extension
   allowlists that do name `.ts`.** The current-worktree source is preserved
   in [`topic-004/sources/s09`](../topic-004/sources/s09-generated-file-comparison-checks.txt)
   (origin `internal/adapter/skgo-adapter.js` lines 198-262): loads must
   match `/\/\+(page|layout)\.server\.ts$/` and actions
   `/\/\+page\.server\.ts$/`. If a JavaScript-form generator emits
   `+page.server.js`, this gate rejects it outright.

*Inference*: a version field or extension allowlist sufficient for
JavaScript-form output would have to live where extension strings are actually
compared — `readGenerated`'s regexes and `checkServerLoads`' node paths (S4
lines 76, 756-770) — not in `skgoAdapter`'s fingerprint, which is
extension-blind.

---

## Previously observed vs. read this session

| Claim | Status |
|---|---|
| probe app has no remotes (`remotes: []`) | **Read** (S1) |
| demo app remote ids `1ptltty/{record,status}` and how client/server resolve them | **Read** (S2) |
| jsproof compiled `example.remote.js` under hash `2b61k` | **Read** (S3) |
| jsproof `skgo.remotes.json`/bindings regenerated at 11:58:22, build artifacts 11:58:28 | **Read** (S3) |
| `pnpm run check` passed on jsproof | *Previously observed* — exit status not preserved (S3 §4) |
| `record(123)` failed as desired (and whether Go or the stub produced the failure) | *Previously observed* — unresolved (S3 §4) |
| `pnpm run build` → adapter rejected old vs new remote path hash | *Previously observed* — mechanism inferred from source (S4), message not captured (S5) |
| probe/demo were built 2026-09-26 with `skgo generate` + `vp build` | *Previously observed* — consistent with mtimes and artifact shape, no command log |

## Explicit gaps

- No captured output or exit status for any prior `check`/`build`/runtime run in
  the three directories (S5).
- `readGenerated`'s `.server.ts` regexes were independently verified from the
  current worktree after indexing; see topic-004/sources/s09.
- What `record(123)` actually returned (Go 400 vs stub throw) — nothing on disk
  distinguishes them.
- `checkRemoteIds` passed for the manually aligned `.js` sample in the later
  operator rerun: the adapter calls it during `adapt`, and `pnpm run build`
  exited 0. Automatic JavaScript generation remains untested.
