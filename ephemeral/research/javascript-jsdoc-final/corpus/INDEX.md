# Semantic index — JavaScript JSDoc generation for skgo

## Corpus scope

Five research groups, one topic each, covering the full path from kit's
JavaScript module rules through skgo's generator, polytype's projection
capability, the adapter's identity gates, and the disposable probe apps.
Evidence was read-only during Gimbal indexing; no builds or tests were run in
that phase. This tree is the authoritative evidence root. Later operator
reruns are recorded separately in `../findings.md`.

## Source locations

| Layer | Location | What lives there |
|---|---|---|
| Pinned kit | `/Users/tyler/src/skgo/ephemeral/inspiration/reference/kit@3.0.0-next.28` | kit 3.0.0-next.28 sources; byte-identical copies in `topic-001/sources/` |
| Pinned polytype | `/Users/tyler/src/skgo/ephemeral/inspiration/reference/polytype` | **symlink → v1.0.0-rc.9** (stale) |
| Live polytype | `/Users/tyler/go/pkg/mod/github.com/tylergannon/polytype@v1.1.0` | **the version skgo actually requires** (go.mod:8) |
| skgo worktree | `/Users/tyler/.codex/worktrees/ef7a/skgo` @ `657656d` | generator, adapter, example app |
| Probe apps | `/tmp/skgo-jsdoc-{probe,demo-probe,jsproof}-20260926` | disposable built apps; inspect artifacts only |

## Citation conventions

- **topic-001**: `sources/kit-3.0.0-next.28__<path>` — line numbers valid for
  both the pinned tree and the local copy. Clips in `clips/` are verbatim.
- **topic-002**: `sources/skgo-internal-gen-*.txt` — full-file copies with an
  11-line catalog header; origin line `L` sits at copy line `L+11`. Multi-excerpt
  files use `===== BEGIN EXCERPT =====` markers with origin path + line span.
- **topic-003**: `sources/v1.1.0-*.md` — bookmarks are `source-file § section —
  origin L<n>-L<m>`. Prefix `V110-` = module cache v1.1.0; `RC9-` = stale
  symlink. **For current capability claims, V110 is authoritative.**
- **topic-004**: `sources/s01`–`s09` — named excerpts with origin file + line
  range stated in each bullet.
- **topic-005**: `sources/probe-*.md` — verbatim inventories and full file
  contents from the three probe trees.

## Known gaps and conflicts

- **Polytype version pin**: `reference/polytype` is v1.0.0-rc.9; skgo requires
  v1.1.0. `declare.go` differs materially between them; `doc_generate.go` is
  identical. All capability claims in this index rest on v1.1.0.
- **Probe run output at indexing time**: no logs or exit statuses survived in
  the disposable app trees. Later operator reruns verified the JS check,
  negative diagnostics, and production build; see `../findings.md`.
- **readGenerated regexes**: `skgo-adapter.js` validates loads/actions against
  `.server.ts` regexes; the original current-worktree excerpt is now preserved
  in `topic-004/sources/s09-generated-file-comparison-checks.txt`.
- **No build/runtime was executed** during research; all kit-side claims are
  source-derived, not observed output.

---

## Routes

### A. "Does kit@3.0.0-next.28 accept `.remote.js` files?"

**When to follow**: the author needs to know whether the pinned kit version
will discover, compile, and serve a JavaScript remote module — the prerequisite
for any JavaScript-form skgo output.

**Lead**: [topic-001](topic-001/INDEX.md) — Q1–Q3.

- `remote_module_pattern = /[/.]remote\.[^/]+$/` accepts **any** extension
  after `remote.` — `.remote.js` matches ([utils.js:144](topic-001/sources/kit-3.0.0-next.28__src-exports-vite-utils.js)).
- `is_remote_module` returns `true` for any non-`node_modules` path matching
  the pattern ([utils.js:158–164](topic-001/sources/kit-3.0.0-next.28__src-exports-vite-utils.js)).
- `plugin_remote` (active when `experimental.remoteFunctions` is on) registers
  the module, assigns `fn.__.id = hash + '/' + name`, and emits a
  `remote-<hash>` chunk ([plugins/remote.js:110–156](topic-001/sources/kit-3.0.0-next.28__src-exports-vite-plugins-remote.js)).
- When the flag is **off**, `plugin_remote_guard` throws on any
  `.remote.<moduleExtensions>` file — default `['.js', '.ts']`
  ([plugins/remote.js:218–237](topic-001/sources/kit-3.0.0-next.28__src-exports-vite-plugins-remote.js)).
- The manifest records hash→chunk-loader only; names/types go to analysis
  metadata ([generate_manifest/index.js:117–119](topic-001/sources/kit-3.0.0-next.28__src-core-generate_manifest-index.js)).

**Bottom line**: kit's pattern is extension-agnostic. `.remote.js` is
discovered and compiled. The guard's narrower `moduleExtensions` allowlist
only matters when the flag is off.

---

### B. "What does skgo's generator emit today, and what language?"

**When to follow**: the author needs the current emit inventory — file names,
extensions, and the exact TypeScript syntax — before planning any
JavaScript-form output mode.

**Lead**: [topic-002](topic-002/INDEX.md) — Q1–Q2, Q5.

- Every code-bearing output is `.ts`, `.go`, or `.json`. **No `.js` is
  emitted anywhere** in `internal/gen` or `internal/adapter`
  ([gen-write-callsite-inventory.txt](topic-002/sources/gen-write-callsite-inventory.txt)).
- `.ts` is hardcoded in three independent places: `scan.go:338`
  (`TrimSuffix(path, ".go") + ".ts"`), `gen.go:309–312` (`loadFileNames` map),
  `endpoints.go:166–168` (`endpointStubPath`).
- `writeStubs` emits: `tsHeader` comment, `$app/server` import, optional
  `import type` blocks, optional `export type` re-export, `const unimplemented
  = (): never => { throw … }`, then one `export const <name> = <factory>(…)
  => unimplemented()` per function. Type annotations are TypeScript-only
  ([emit.go:59–178](topic-002/sources/skgo-internal-gen-emit.go.txt)).
- The adapter's JavaScript (`skgo-adapter.js`, `skgo-adapter/env.js`,
  `skgo-adapter/polyfill.js`) is **checked-in embedded source**, not generated
  ([adapter.go:44–45](topic-002/sources/skgo-internal-adapter-adapter.go.txt)).
- `kithash.Kit(fn.module)` hashes the full module path **including extension**
  ([kithash.go:19–29](topic-002/sources/skgo-internal-kithash-kithash.go.txt));
  a `.ts`→`.js` switch changes the hash.

**Bottom line**: switching to JavaScript-form output requires coordinated
changes to three hardcoded `.ts` paths, the `stubSignature` templates, and
every consumer of `fn.module` strings — not just file extensions.

---

### C. "Can polytype v1.1.0 project JSDoc for JavaScript-form output?"

**When to follow**: the author needs to know what polytype's TypeScript
backend already does with documentation, and what is structurally missing for
function-level JSDoc.

**Lead**: [topic-003](topic-003/INDEX.md) — Q3, Q4, Q5. **Authoritative
source**: `/Users/tyler/go/pkg/mod/github.com/tylergannon/polytype@v1.1.0`.

**What already works**:
- `writeDoc` (printer.go:130–148) emits `/** … */` JSDoc blocks, driven by
  `Description` strings on typegrammar nodes. Two call sites: type aliases
  (printer.go:24) and object properties (printer.go:116).
- `sanitizeComment` (printer.go:151–175) escapes `*/` → `*\/` and control
  characters — a tested, working JSDoc formatter.
- `Description` is captured from Go doc comments (or a `description` struct
  tag on fields) at `internal/syntax/node_wrappers.go:625–633` and stored on
  `Definition`, `Field`, and `EnumMember` (typegrammar/grammar.go:57,125,163).
- `Result.Names` (generate.go:56–60) maps every definition to its emitted
  identifier — collision-safe names may differ from Go names. **skgo already
  depends on this**: `internal/gen/types.go:401–407` refuses to emit if a name
  was renamed.
- `TestGenerateProjectsCompleteGrammar` asserts Description values (including
  `*/` and control chars) are projected into the output
  (generate_test.go:31–127).

**What is structurally missing**:
- The typegrammar has **no function type constructor**. The projector handles
  Scalar, Time, Enum, Object, Pointer, Slice, Array, and Ref only
  (generate.go:100–171). There is no function-level `Description` field, so
  `writeDoc` has no function call site.
- For skgo's JavaScript-form output — where remotes, loads, actions, and
  endpoints are **functions**, not types — polytype's existing JSDoc
  projection cannot reach them. The grammar has no node to carry a
  function's doc.
- `Declare`/`Declaration` has no doc-attachment method; documentation comes
  only from Go source comments captured during lowering
  ([v1.1.0-public-api-no-doc-attachment.md](topic-003/sources/v1.1.0-public-api-no-doc-attachment.md)).

**Bottom line**: polytype v1.1.0 has a working JSDoc formatter and
`Description` plumbing for types and properties. The gap is structural: the
grammar has no function node, so function-level JSDoc projection does not
exist and cannot be configured into existence — it would require a grammar
extension.

---

### D. "How do the adapter fingerprint and manifest gates work?"

**When to follow**: the author needs to understand the identity checks that
must pass before JavaScript-form output is accepted at runtime.

**Lead**: [topic-004](topic-004/INDEX.md) — Q1–Q6.

- `Fingerprint` = SHA-256 over `skgo-adapter.js` + sorted `skgo-adapter/*`
  files (name + `\x00` + bytes), truncated to 12 hex chars
  ([adapter.go:131–164](topic-004/sources/s01-adapter-fingerprint-and-identity-go.txt)).
  **Extension-blind**: covers only adapter package files, not generated app files.
- Two gates compare it: generate-time `checkInstalledAdapter`
  ([internal/gen/adapter.go:31–70](topic-004/sources/s04-checkinstalledadapter-gate.txt))
  and startup `ReadManifest` ([remote.go:484–527](topic-004/sources/s03-manifest-struct-and-readmanifest.txt)).
  The startup gate is documented as unbypassable.
- The manifest carries `skgo` (version) and `skgoAdapter` (fingerprint) as
  separate fields ([skgo-adapter.js:118–140](topic-004/sources/s02-identity-js-and-manifest-write.txt)).
  Only the fingerprint is compared; version mismatch with matching fingerprint
  passes both gates.
- Generated app files are checked by **separate comparisons**, not the
  fingerprint: `readGenerated` validates `skgo.remotes.json` shape,
  `checkRemoteHashes` compares hash prefixes, `checkRemoteIds` walks client
  bundles for id literals ([s07](topic-004/sources/s07-fingerprint-scope-and-generated-file-checks.txt),
  [s09](topic-004/sources/s09-generated-file-comparison-checks.txt)).

**Bottom line**: the fingerprint gate is extension-blind and will not block
JavaScript-form output. The extension-sensitive gates are `readGenerated`'s
`.server.ts` regexes and `checkRemoteHashes`' path-hash comparison.

---

### E. "What did the probe apps actually prove?"

**When to follow**: the author needs to distinguish observed facts from
inference and identify what remains unproven about JavaScript remote modules.

**Lead**: [topic-005](topic-005/INDEX.md) — Q1–Q6.

- **Probe app** (`/tmp/skgo-jsdoc-probe-20260926`): no remote functions at all
  (`remotes: []`); only `+layout.svelte` and `+page.svelte`.
- **Demo app** (`/tmp/skgo-jsdoc-demo-probe-20260926`): `example.remote.ts`
  compiled under hash `1ptltty`. Client resolves `1ptltty/record` and
  `1ptltty/status` by string id over HTTP; server resolves by hash-keyed chunk
  import. SSR bundle contains `init_remote_functions(example_remote_exports,
  "src/routes/example.remote.ts", "1ptltty")`.
- **JS-proof app** (`/tmp/skgo-jsdoc-jsproof-20260926`): `example.remote.js`
  compiled under hash `2b61k` (different from the TS build's `1ptltty` because
  the hash covers the full path including extension). `pnpm run check` passed;
  `pnpm run build` compiled the JS remote but the adapter rejected the
  old-vs-new hash mismatch. **No output captured** — these are operator
  reports, not verified observations.
- The failure mechanism: `checkRemoteHashes` compares hash prefixes in
  `skgo.remotes.json` against kit's `manifest.remotes` keys. A `.ts`→`.js`
  switch changes the hash on both sides; if one side is regenerated before the
  other, the gate fires.
- **Unresolved**: whether `readGenerated`'s `.server.ts` regexes remain in the
  current worktree; what `record(123)` actually returned; whether
  `checkRemoteIds` would pass on a `.js` build.

**Bottom line**: kit compiles `.remote.js` successfully. The adapter's
hash cross-check is the first gate that fires when the extension changes.
No end-to-end JavaScript-form build has been captured.

---

## Cross-cutting themes

### Extension-sensitive surfaces

Where `.ts` is hardcoded or regex-matched — the places a `.js` switch must
touch:

| Surface | Location | Kind |
|---|---|---|
| Stub path | `scan.go:338` | hardcoded `.ts` suffix |
| Load/server file names | `gen.go:309–312` | hardcoded map |
| Endpoint stub path | `endpoints.go:166–168` | hardcoded `+server.ts` |
| `fn.module` strings | `emit.go:745` via `kithash.Kit` | hashed path includes extension |
| `readGenerated` regexes | `skgo-adapter.js:235,245` | `.server.ts` allowlist (unresolved) |
| `checkRemoteHashes` | `skgo-adapter.js:653` | hash comparison (extension-sensitive via path) |

### Hash/path coupling

`kithash.Kit` hashes the full module path including extension. A `.ts`→`.js`
switch changes the hash. Both sides of every hash comparison must switch
atomically or the gate fires.

### Version pin discrepancy

`reference/polytype` → v1.0.0-rc.9; skgo `go.mod` → v1.1.0. For any current
capability claim about polytype, read the module cache at
`/Users/tyler/go/pkg/mod/github.com/tylergannon/polytype@v1.1.0`. The rc.9
`declare.go` must not be quoted as v1.1.0.
