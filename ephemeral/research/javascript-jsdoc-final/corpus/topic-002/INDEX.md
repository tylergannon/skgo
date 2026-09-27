# Topic 002 — skgo generator output inventory: which files it writes today and in what language

**Scope.** Original evidence only, from the worktree at commit
`657656d1161edf832eb99b69cc1636e37877144b` (`codex/javascript-generation-research`,
2026-09-25), retrieved 2026-09-26. No builds, tests or source edits were run.

**How to read a citation.** `sources/<file>` citations quote *origin line
numbers* (the file inside the skgo repo, e.g. `internal/gen/emit.go:119`).
Full-file copies carry an 11-line catalog header, so origin line `L` sits at
copy line `L+11`. Multi-excerpt files (`example-*`, `gen-tests-*`,
`newapp-*`, `gen-prerender-*`, `*-inventory.txt`, `*-import-blocks.txt`)
state each origin path and line span inside a `===== BEGIN EXCERPT =====`
marker; cite those by marker.

---

## Q1 — Which file names and extensions does `internal/gen/emit.go` write today?

**Answer: the destinations below — `.remote.ts` stubs (via `a.stubs` paths),
`skgo_remotes_gen.go`, `skgo_bindings_gen.go`, `skgo_devalue_gen.go`,
`skgo_client_gen.go`, `skgo_client_devalue_gen.go`, `skgo.remotes.json`, plus
`+page.server.ts`/`+layout.server.ts`, `+server.ts` and `types.ts` written by
sibling files in the same package. Every code-bearing output today is `.ts`,
`.go` or `.json`; none is `.js`.**

| Fact | Origin citation | Local evidence |
|---|---|---|
| `goHeader` / `tsHeader` constants | `internal/gen/emit.go:16-17` | `sources/skgo-internal-gen-emit.go.txt` |
| `writeStubs` doc says "one `.remote.ts` per `.remote.go`" | `emit.go:19-24` | same |
| `writeStubs` writes each stub at `stub` (path supplied by scanner) | `emit.go:119` | same |
| `writePackageBindings` writes `filepath.Join(gp.dir, "skgo_remotes_gen.go")` | `emit.go:296` | same |
| `writeAppBindings` writes `filepath.Join(a.cfg.Out, "skgo_bindings_gen.go")` | `emit.go:501` | same |
| `writeRemoteList` writes `filepath.Join(a.cfg.Web, "skgo.remotes.json")` | `emit.go:760` | same |
| Load stubs: `writeLoadStubs` writes `tsHeader` + `export const load` | `internal/gen/loads.go:35, 102, 153` | `sources/skgo-internal-gen-loads.go.txt` |
| Action stubs live in the *same* file as load stubs (`actions` map keyed by module) | `loads.go:41-43, 155-197` | same |
| Endpoint stubs: `writeEndpointStubs` writes `tsHeader` + `export const GET…` | `internal/gen/endpoints.go:88, 101-116` | `sources/skgo-internal-gen-endpoints.go.txt` |
| `endpointStubPath` hardcodes `+server.ts` | `endpoints.go:166-168` | same |
| `loadFileNames` maps `page.server.go → +page.server.ts`, `layout.server.go → +layout.server.ts` | `internal/gen/gen.go:309-312` | `sources/skgo-internal-gen-gen.go.txt` |
| Stub path is `strings.TrimSuffix(path, ".go") + ".ts"` (`.ts` hardcoded) | `internal/gen/scan.go:338` | `sources/skgo-internal-gen-scan.go.txt` |
| Load/server files override that with `loadFileNames[base]` / `endpointStubPath` | `scan.go:339-344` | same |
| Wire types: `projectTypes` writes `filepath.Join(set.tsDir, file.Name)` where `file.Name` is polytype's `types.ts` | `internal/gen/types.go:416-425` | `sources/skgo-internal-gen-types.go.txt` |
| `tsDir` default is `<web>/src/lib/skgo/<pkg.Name()>` | `types.go:306` | same |
| Form clients: `skgo_client_gen.go`, `skgo_client_devalue_gen.go` | `internal/gen/clients.go:122, 125` | `sources/skgo-internal-gen-clients.go.txt` |
| Devalue codecs: `devalueCodecsFile = "skgo_devalue_gen.go"` | `internal/gen/codecs.go:23, 169` | quoted in `sources/gen-write-callsite-inventory.txt` (OUTPUT 1) |
| Call order inside `Run` | `gen.go:165-202` | `sources/skgo-internal-gen-gen.go.txt` |
| Complete list of file-writing call sites in `internal/gen` (non-test) | — | `sources/gen-write-callsite-inventory.txt` OUTPUT 1 |

**Comment vs code (Q1/Q2 boundary):** `tsHeader` is a `//` comment line only;
everything after it in a stub is code except the two `// Every body throws…`
prose blocks at `emit.go:107-109`, `loads.go:132-136`, `endpoints.go:102-105`.

---

## Q2 — What exact TypeScript declarations does `writeStubs` emit?

**Answer: an `import` of kit factories, optional `import type` blocks, an
optional `export type` re-export, one `const unimplemented` helper, then one
`export const <name> = <factory>(…) => unimplemented()` line per function.
Type parameters are inline arrow annotations, never generics; the annotations
are TypeScript-only syntax.**

| Element | Origin citation | Local evidence |
|---|---|---|
| Builder preamble `tsHeader` | `emit.go:71` | `sources/skgo-internal-gen-emit.go.txt` |
| `import { <kinds> } from '$app/server'` (`command`/`query`/`form`, sorted) | `emit.go:59-66, 72` | same |
| `import type { … } from '<hooks>'` for transported classes | `emit.go:74-84` | same |
| `import type { … } from '<types specifier>'`, one per spec, sorted | `emit.go:86-97` | same |
| `export type { … }` re-export (type-only; runtime export would throw in `init_remote_functions`) | `emit.go:98-105` | same |
| `const unimplemented = (): never => { throw … }` — TypeScript `(): never =>` | `emit.go:107-110` | same |
| Per-function loop | `emit.go:112-118` | same |
| `stubSignature`: call selection `query`/`command`/`form`/`query.live` | `emit.go:133-144` | same |
| `query.live` wraps result as `AsyncIterable<T>` | `emit.go:139-143` | same |
| Batch form: `query.batch('unchecked', (_args: T[]): ((arg: T, idx: number) => R) => unimplemented())` | `emit.go:160-161` | same |
| No-argument form: `export const N = F((): R => unimplemented());` | `emit.go:164-170` (line 169) | same |
| One-argument form: `export const N = F('unchecked', (_arg: I): R => unimplemented());` | `emit.go:175-178` (line 177) | same |
| Argument type expression comes from `a.project(fn.in/out)` → `out.expr` (polytype `tsType`) | `emit.go:128-134, 171-174`; `types.go:16-30` | `sources/skgo-internal-gen-emit.go.txt`, `sources/skgo-internal-gen-types.go.txt` |
| `importSpecifier` is relative and **extensionless** | `types.go:429-442` | `sources/skgo-internal-gen-types.go.txt` |
| Tests pinning each emitted line verbatim | `internal/gen/arity_test.go:62-67, 258`; `endpoints_test.go:39-48` | `sources/gen-tests-pinning-stub-syntax.txt` (markers `arity_test.go lines 1-80`, `lines 245-265`, `endpoints_test.go lines 20-95`) |
| Real generated output, 25 lines | `example/web/src/routes/todos/todos.remote.ts:1-25` | `sources/example-app-generated-files.txt` marker `todos.remote.ts lines 1-25` |

**Comment vs code in a generated `.remote.ts`:** comments are lines 1
(`tsHeader`), 8-10 (prose), and `import type`/`export type` lines (code, but
erased at runtime). Code that runs is line 2 and 11-25.

---

## Q3 — How does `writePackageBindings` emit per-package bindings?

**Answer: one Go file per Go package that declares a remote, load, action or
endpoint — never JavaScript — with a `var` block of `Skgo_<name> = <name>`
aliases and an optional `type` block of `SkgoArg_/SkgoOut_` aliases. The import
block is emitted only when an alias names a type from another package.**

| Fact | Origin citation | Local evidence |
|---|---|---|
| Grouping by `goPkg` for remotes/loads/actions/endpoints | `emit.go:241-261` | `sources/skgo-internal-gen-emit.go.txt` |
| Skip packages with nothing to publish | `emit.go:262-264` | same |
| Type aliases rendered *first* because rendering discovers imports | `emit.go:265-269` | same |
| `b.WriteString(goHeader)` + `package <name>` + `imports.writeTo(&b)` | `emit.go:271-274` | same |
| `var (` block: comment line then `Skgo_x = x` for remotes, loads, actions, endpoints | `emit.go:275-294` | same |
| Write to `filepath.Join(gp.dir, "skgo_remotes_gen.go")` | `emit.go:296` | same |
| `writeTypeAliases` emits `SkgoArg_` (form/batch), `SkgoOut_` (form/live) with `// … is the type … takes/returns/yields` comments | `emit.go:309-333` | same |
| Import block construction: `fileImports.typeExpr` adds one aliased import per cross-package type; `writeTo` writes `import (\n\t<alias> "<path>"\n)` sorted | `emit.go:338-383` | same |
| `exportedName` prefix is `"Skgo_"` | `emit.go:385-388` | same |
| No git-tracked example currently emits an `import (` block in `skgo_remotes_gen.go` | recorded grep, 0 matches | `sources/example-remotes-gen-import-blocks.txt` |
| Examples with `var`+`type` blocks only | `example/web/src/routes/todos/skgo_remotes_gen.go:1-26`; `contact:1-20`; `actions:1-31`; `lib:1-15` | `sources/example-app-generated-files.txt` markers for `todos`, plus origin paths for `contact`/`actions`/`lib` |

Note: `writeAppBindings` (`emit.go:408-502`) is a *separate* Go file,
`skgo_bindings_gen.go`, and `links.sync` copies each route package's
`skgo_remotes_gen.go` into `example/internal/skgo/links/<hash>/`
(`internal/gen/links.go:276-311`, `.go` files only at `links.go:508-517`) —
`sources/skgo-internal-gen-gen.go.txt` (Run at 178-186), write-callsite OUTPUT 1.

---

## Q4 — Which example-app files are checked in, and what do they show?

**Answer: 126 tracked generated/route files, including 63
`skgo_remotes_gen.go`, all `.remote.ts` / `types.ts` / `+*.server.ts` /
`+server.ts`. Their contents show the emit path is fixed to `.ts` and that the
Go side records `.remote.ts` module paths as strings.**

| Fact | Origin citation | Local evidence |
|---|---|---|
| Full `git ls-files` listing with counts | command output, commit `657656d` | `sources/example-git-tracked-generated-files.txt` |
| 63 tracked `skgo_remotes_gen.go`, all `.remote.ts` tracked | same | same (tail counts) |
| `todos.remote.ts` content: `tsHeader`, `$app/server` import, two `import type`, `export type`, `(): never =>`, six `export const` | `example/web/src/routes/todos/todos.remote.ts:1-25` | `sources/example-app-generated-files.txt` |
| `src/lib/skgo_remotes_gen.go` content: comments name `src/lib/auth.remote.ts#signIn` — the **module path string embeds `.remote.ts`** | `example/web/src/lib/skgo_remotes_gen.go:1-15` | same |
| per-route `todos/skgo_remotes_gen.go`: same shape + `SkgoOut_watchCount = int` | `example/web/src/routes/todos/skgo_remotes_gen.go:1-26` | same |
| `+server.ts` endpoint stub | `example/web/src/routes/api/todos/+server.ts:1-14` | same |
| `+page.server.ts` load+action stub (`import { building } from '$app/env'`, `export const load`, `export const actions`) | `example/web/src/routes/actions/+page.server.ts:1-29` | same |
| polytype `types.ts` already carries `/** … */` JSDoc on types and fields | `example/web/src/routes/todos/types.ts:1-33` | same |
| `skgo_bindings_gen.go` (1554 lines) and `skgo_devalue_gen.go` (3379) are Go | `example/internal/skgo/…` | counts recorded in this session; origin paths listed above |

**Inference (not source fact):** because `fn.module` strings and the comments
in generated Go name `.remote.ts`, switching output language changes identifiers
consumed by `kithash.Kit(fn.module)` (`emit.go:745`) and by the adapter's
remotes comparison, not only file names.

---

## Q5 — Does any code path in `internal/gen` or `internal/adapter` emit a `.js` file?

**Answer: No. Every emitted file is TypeScript, Go or JSON. `internal/adapter`
writes no files at all — it only reads embedded bytes.**

| Fact | Evidence |
|---|---|
| Complete non-test write call sites in `internal/gen`: `emit.go:119, 296, 501, 760, 794`; `endpoints.go:116`; `loads.go:199`; `types.go:421`; `clients.go:122, 125`; `codecs.go:169`; `links.go:307, 419, 472`; `gen.go:247, 272` — destination names are `.ts`, `_gen.go`, `.json` | `sources/gen-write-callsite-inventory.txt` OUTPUT 1 |
| `internal/adapter` Go code contains **no** `WriteFile`/`os.Create`; only `ReadFile` (`adapter.go:114, 225`) | same file, OUTPUT 2 |
| No `.remote.js` / `+server.js` stub-path literal in non-test Go under `internal/` or `cmd/`; only `prerender.go:161` reading *authored* `+page.server.js`, `newapp_test.go:219` fixture, `kithash_test.go:16` golden | same file, OUTPUT 3 |
| The prerender scanner already recognises authored `.js` route files (`+page.js`, `+page.server.js`, `+layout.server.js`, suffix list `.ts`/`.js`/`.server.ts`/`.server.js`) | `internal/gen/prerender.go:161, 220, 247` — `sources/gen-prerender-authored-js-recognition.txt` |
| `kithash` golden includes `{"src/routes/data.remote.js", "mxe8u8"}` described as "observed from a running kit build" | `internal/kithash/kithash_test.go:13-16` — `sources/skgo-internal-kithash-kithash_test.go.txt`; algorithm `kithash.go:19-29` in `sources/skgo-internal-kithash-kithash.go.txt` |
| The only "jsdoc" switch today is `sv`'s `--types` flag, defaulted to `ts`; it is a scaffolding argument, not a generator mode | `internal/newapp/newapp.go:302-306, 326-328`; `cmd/skgo/main.go:4, 144` — `sources/newapp-types-flag.txt` |

---

## Q6 — What does `internal/gen/adapter.go` emit for `skgo-adapter.js`, `skgo-adapter/env.js`, `skgo-adapter/polyfill.js`?

**Answer: nothing — the question's premise is wrong. `internal/gen/adapter.go`
is a 70-line gate (`checkInstalledAdapter`) with no file writes. The three
JavaScript files are checked-in ES-module sources in `internal/adapter/`,
embedded with `//go:embed` and shipped as the `@skgo/sveltekit-adapter` npm
package.**

| Fact | Origin citation | Local evidence |
|---|---|---|
| `internal/gen/adapter.go` contains exactly one function, `checkInstalledAdapter`; no `write`/`WriteFile` | `internal/gen/adapter.go:1-70` (fn at 31-70) | `sources/skgo-internal-gen-adapter.go.txt` |
| It only calls `adapter.FingerprintOf(dir)`, compares to `adapter.Fingerprint()`, and formats an error | `adapter.go:34-43, 58-69` | same |
| `//go:embed skgo-adapter.js skgo-adapter` — the JS is embedded, not generated | `internal/adapter/adapter.go:44-45` | `sources/skgo-internal-adapter-adapter.go.txt` |
| `entryFile = "skgo-adapter.js"`, `filesDir = "skgo-adapter"` | `adapter.go:51-54` | same |
| `Fingerprint` / `FingerprintOf` / `fingerprintFS` hash entry bytes + sorted runtime files, 12 hex digits | `adapter.go:87-164` | same |
| `Polyfill()` reads `skgo-adapter/polyfill.js` out of the embed | `adapter.go:224-231` | same |
| npm package ships `skgo-adapter.js` + 6 `skgo-adapter/*.js`, `"type": "module"`, exports `./skgo-adapter.js` | `internal/adapter/package.json:1-53` | `sources/skgo-internal-adapter-package.json.txt` |
| Syntax of `skgo-adapter.js`: ESM `import … from 'node:fs'` / `'node:path'`, top-level `const`, JSDoc `/** … */` block on `skgo()` | `skgo-adapter.js:1-40` (imports at 1-12, JSDoc at 29-37) | `sources/skgo-internal-adapter-skgo-adapter.js.txt` |
| Syntax of `skgo-adapter/env.js`: block JSDoc header then ESM imports from `node:module`/`node:fs`/`node:path`/`node:url` | `skgo-adapter/env.js:1-28` | `sources/skgo-internal-adapter-env.js.txt` |
| Syntax of `skgo-adapter/polyfill.js`: `//` comment header, then plain global assignments (`globalThis.process = …`, `class Headers` guard) — a script, not a module | `skgo-adapter/polyfill.js:1-30` | `sources/skgo-internal-adapter-polyfill.js.txt` |
| Syntax of `skgo-adapter/identity.js`: JSDoc header, ESM imports from `node:crypto`/`node:fs`/`node:path`/`node:url` | `skgo-adapter/identity.js:1-21` | `sources/skgo-internal-adapter-identity.js.txt` |
| Example app links the adapter locally: `"@skgo/sveltekit-adapter": "link:../../internal/adapter"` | `example/web/package.json:17` | noted here (origin read directly, not copied) |

---

## Supported facts (summary)

1. Emit destinations today: `.remote.ts`, `+page.server.ts`, `+layout.server.ts`,
   `+server.ts`, `src/lib/skgo/<pkg>/types.ts`, `skgo.remotes.json` (all
   non-Go, non-JSON are TypeScript) and `skgo_remotes_gen.go`,
   `skgo_bindings_gen.go`, `skgo_devalue_gen.go`, `skgo_client_gen.go`,
   `skgo_client_devalue_gen.go`, plus `links/<hash>/*.go` copies.
2. `.ts` is hardcoded in three independent places: `scan.go:338`,
   `gen.go:309-312`, `endpoints.go:166-168`.
3. `writeStubs` output shape is pinned verbatim by `arity_test.go` and
   `endpoints_test.go`.
4. `writePackageBindings` emits Go only; its import block appears only for
   cross-package types, and no checked-in example has one.
5. No `.js` is emitted anywhere in `internal/gen` or `internal/adapter`.
6. `internal/gen/adapter.go` emits nothing; the adapter's JS is embedded
   checked-in source in ESM form (polyfill is a classic script).

## Inference (kept separate)

- A JavaScript-form output mode would have to change `scan.go:338`,
  `gen.go:309-312`, `endpoints.go:166-168`, `emit.go:107-110` and the three
  `stubSignature` templates (`emit.go:160, 169, 177`) together, and would
  change `fn.module` strings that `kithash.Kit` hashes at `emit.go:745`.
- `kithash_test.go`'s `.remote.js` golden suggests kit hashes such paths, but
  that is a hash test, not proof kit's plugin accepts a `.remote.js` file.

## Unresolved / gaps

- Whether kit@3.0.0-next.28's `remote_module_pattern` accepts `.remote.js` is
  **not established here** — it belongs to topic-001 (pinned kit sources).
- Whether polytype's `typescript.Generate` can emit JavaScript+JSDoc rather
  than `types.ts` is **not established here** — topic-003. `skgo` passes
  `typescript.Options{}` (`types.go:395`); whether that struct has a language
  field was not read.
- Whether the adapter manifest or a runtime check needs a version field or
  extension allowlist before `.js` output is accepted — topic-004/005; only
  `checkInstalledAdapter`'s fingerprint gate was read here.
- `example/web/package.json:17` was read but not copied as a source file.
- The exact content of `prerender.go` outside lines 150-260 was not collected.
