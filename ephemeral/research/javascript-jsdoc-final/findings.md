# skgo JavaScript JSDoc generation — source-cited implementation map

Research corpus: `corpus/`, five topics. Commit: `657656d`. Retrieved 2026-09-26.
No builds or tests were run by the read-only researcher; no application source was
edited. Operator reruns are labelled as such.

Citations: `path.go:LINE` for skgo; `kit@3.0.0-next.28/<path>:LINE` for kit;
`polytype@v1.1.0/<path>:LINE` for the module cache (authoritative) or
`polytype@rc.9/<path>:LINE` for the stale symlink.

---

## Group 1 — Pinned SvelteKit 3.0.0-next.28 JavaScript module rules

**Source facts.** `remote_module_pattern = /[/.]remote\.[^/]+$/`
(`kit@3.0.0-next.28/src/exports/vite/utils.js:144`) — extension-agnostic;
`.remote.js` matches. `is_remote_module` (`utils.js:158-164`) returns `true`
for any non-`node_modules` path matching the pattern; for `node_modules` paths
it walks upward looking for `peerDependencies['@sveltejs/kit']`
(`utils.js:170-193`).

`plugin_remote` (`plugins/remote.js:22`) is active only when
`experimental.remoteFunctions` is on (`:53-55`). Server branch (`:110-156`):
registers the module, appends `init_remote_functions`, sets
`fn.__.id = hash + '/' + name` (`:129-134`), emits `remote-<hash>` chunk in
prod (`:139-150`). Client branch (`:158-209`): builds stub calling
`__remote.<type>('<hash>/<name>')` per export (`:185-199`).

`plugin_remote_guard` (`plugins/remote.js:218-237`) is active only when the
flag is **off** (`:222-224`). Filter matches
`.remote(${moduleExtensions.join('|')})$` where `moduleExtensions` defaults to
`['.js', '.ts']` (`config/options.js:164`); handler throws
`error_for_missing_config('remote functions', 'experimental.remoteFunctions', 'true')`
(`:233`). Does not consult `is_remote_module`.

Externalization gate (`vite/index.js:468-490`): `if (kit.experimental.remoteFunctions)`
(`:469`) adds a plugin returning `{ id: to_fs(resolved.id), external: 'absolute' }`
for remote modules (`:479-485`). Flag off: plugin not added.

Manifest records hash→chunk-loader only:
`remotes: { '<hash>': __memo(() => import('…/chunks/remote-<hash>.js')) }`
(`generate_manifest/index.js:117-119`). Hash is djb2→base36 over the
vite-root-relative posix path **including extension**
(`plugins/remote.js:104-107`, `utils/hash.js:5-19`). Names/types go to
analysis metadata (`core/postbuild/analyse.js:141-158`).

Runtime: client resolves by string id over HTTP —
`` `${base}/${appDir}/remote/${id}` `` where `id = <hash>/<name>`
(`runtime/client/remote-functions/query/index.js:27`). Server resolves by
hash-keyed chunk import (`runtime/server/remote-functions.js:165-175`).

**Inference.** The browser never fetches a remote JS chunk; the chunk import
is server-side.

---

## Group 2 — Current skgo generated file and language paths

**Source facts.** Every code-bearing output is `.ts`, `.go`, or `.json`; no
`.js` is emitted anywhere (`corpus/topic-002/sources/gen-write-callsite-inventory.txt`).
`.ts` hardcoded in three places: `scan.go:338`, `gen.go:309-312`
(`loadFileNames` map), `endpoints.go:166-168` (`endpointStubPath`).

`writeStubs` (`emit.go:25-124`, templates `emit.go:127-179`): `tsHeader`
comment, `import { command, query, form } from '$app/server'`, optional
`import type` blocks, optional `export type { … }`, prose comment,
`const unimplemented = (): never => { throw … }`, then one line per function:
- `export const n = F((): R => unimplemented())` (`emit.go:169`)
- `export const n = F('unchecked', (_arg: I): R => unimplemented())` (`emit.go:177`)
- `query.batch('unchecked', (_args: I[]): ((arg: I, idx: number) => R) => …)` (`emit.go:160`)
- `query.live` wraps as `AsyncIterable<T>` (`emit.go:139-143`)

Import specifiers extensionless (`types.go:429-442`). Lines pinned by
`arity_test.go:62-67,258` and `endpoints_test.go:39-48`.

`writePackageBindings` (`emit.go:241-301`) emits Go only: `var` block of
`Skgo_<name> = <name>` aliases with `// … published as <module>#<name>` comments,
plus `type` aliases `SkgoArg_`/`SkgoOut_`. Import block only for cross-package
types; no checked-in example has one.

Example app (126 tracked files; 63 `skgo_remotes_gen.go`): `todos.remote.ts`
shows full stub shape; Go bindings embed `.remote.ts` module strings in
comments; `routes/todos/types.ts` shows polytype already emits `/** … */`
JSDoc on types and fields.

No `.js` path exists. `internal/adapter` performs no writes. The `--types jsdoc`
switch (`newapp.go:302-306,326-328`) is scaffolding-only. Prerender scanner
tolerates *authored* `.js` route files (`prerender.go:161,220,247`). `kithash`
golden for `src/routes/data.remote.js` (`kithash_test.go:16`) — suggestive,
not proof.

`internal/gen/adapter.go` emits nothing — it is `checkInstalledAdapter`
(`adapter.go:31-70`). Adapter JS is checked-in source embedded by
`//go:embed skgo-adapter.js skgo-adapter` (`internal/adapter/adapter.go:44-45`).
ESM with JSDoc in `skgo-adapter.js`, `env.js`, `identity.js`; `polyfill.js`
is a classic script.

**Inference.** A JS-form mode must change `scan.go:338`, `gen.go:309-312`,
`endpoints.go:166-168`, `emit.go:107-110`, the three `stubSignature` templates
(`emit.go:160,169,177`), and `fn.module` strings hashed by `kithash.Kit`
(`emit.go:745`).

---

## Group 3 — Polytype v1.1.0 JSDoc projection requirement

**Source facts.** `reference/polytype` is a symlink to `polytype@v1.0.0-rc.9` —
**not** v1.1.0. skgo `go.mod:8` requires v1.1.0. All capability claims rest on
the module cache.

`declare.go` has no JSDoc handling. `DeclarationSpec` has no `Doc`/`Comment`/
`Description` field (`configuration.go:76-112`). No doc-attachment method on
`*Declaration[T]`.

`doc_generate.go` is a `go:generate` directive emitting a Markdown examples
reference — not HTML, not TypeScript.

Documentation is a plain `string` on three typegrammar nodes:
`Definition.Description` (`grammar.go:54-59`), `EnumMember.Description`
(`grammar.go:120-126`), `Field.Description` (`grammar.go:159-166`). Captured
from `dst` comments (`comments.go:1-68`); `StructField.Comments()` honours a
`` `description:"…"` `` struct tag (`node_wrappers.go:625-633`).

Surfaced by `writeDoc` (`typescript/printer.go:130-148`) emitting `/** … */`
blocks per type alias (`printer.go:24`) and per object property
(`printer.go:116`). `sanitizeComment` (`printer.go:151-175`) escapes `*/` and
control characters.

**The structural gap.** The typegrammar has no function type constructor
(`generate.go:100-171` handles Scalar, Time, Enum, Object, Pointer, Slice,
Array, Ref only). No function-level `Description` field exists, so `writeDoc`
has no function call site.

**Polytype issue #157** requests grammar-faithful JSDoc projection for named
wire-type typedefs with checked-JavaScript consumer tests. This concerns
`types.js` output only. skgo owns function-level JSDoc for remote/load/action/
endpoint signatures; no Polytype grammar extension is needed for those.

**No public API for doc attachment.** Documentation comes only from Go source
comments or a `description` struct tag. `declare_test.go` is compile-only with
zero assertions.

**Inference.** For JS-form output, function-level JSDoc type annotations
are skgo's responsibility. Doc prose could come from skgo's own scan of Go
declaration comments or from polytype's `Description` plumbing, but whether
skgo consumes `typegrammar.Definition.Description` is not established here.
Polytype's `writeDoc`/`sanitizeComment` is a working formatter for wire types
but lives in the `typescript` backend.

---

## Group 4 — Adapter manifest hash and check integration

**Source facts.** `fingerprintFS` (`internal/adapter/adapter.go:131-164`):
SHA-256 over `entryFile` = `"skgo-adapter.js"` bytes, then every file under
`filesDir` = `"skgo-adapter"` (sorted, `name+"\x00"+bytes`), truncated to 12
hex chars. Covers **only adapter package files** — `package.json` is read for
version only, never hashed.

`Identity(version, fingerprint)` (`adapter.go:203-213`) returns a display
string. The manifest carries the halves separately: `skgo: SKGO.version,
skgoAdapter: SKGO.adapter` (`skgo-adapter.js:126-127`). Go models them as
`static.go:22-32`.

Two gates compare the fingerprint:
- Generate-time `checkInstalledAdapter` (`internal/gen/adapter.go:31-70`):
  `FingerprintOf(installed)` vs `Fingerprint()` (`:41`). Error: `"skgo: the %s installed in this app is not the one this skgo publishes.…"` (`:58-69`).
- Startup `skgo.ReadManifest` (`remote.go:484-527`): `m.SkgoAdapter != adapter.Fingerprint()` (`:507`). Error: `"skgo: the frontend build and this program come from different skgo versions.…"` (`:512-518`). Only fingerprint compared; `m.Skgo` not compared.

Manifest written during `adapt` (`skgo-adapter.js:118-140`). Runtime reads
`skgo.manifest.json` at build root (`remote.go:500`, `static.go:330`).

Generated app files checked by separate comparisons: `readGenerated` validates
`skgo.remotes.json` shape (`skgo-adapter.js:198-262`), `checkRemoteHashes`
compares hash prefixes (`:653`), `checkRemoteIds` walks client bundles for id
literals (`:701`), `checkServerLoads` (`:756`).

**Inference.** The fingerprint gate is extension-blind. Extension-sensitive
gates: `readGenerated`'s `.server.ts` regexes (`skgo-adapter.js:235,245`) and
`checkRemoteHashes`' path-hash comparison.

---

## Group 5 — Probe results and required future proof

**Source facts (read-only researcher).**

*Probe app* (`/tmp/skgo-jsdoc-probe-20260926`): no remotes (`remotes: []`);
only `+layout.svelte` and `+page.svelte`.

*Demo app* (`/tmp/skgo-jsdoc-demo-probe-20260926`): `example.remote.ts` (TS).
Client node has `` ge(`1ptltty/record`) `` / `` ye(`1ptltty/status`) ``; URL
`` `${D}/${M}/remote/${e}` ``. SSR bundle:
`init_remote_functions(example_remote_exports, "src/routes/example.remote.ts", "1ptltty")`.

*JS-proof app* (`/tmp/skgo-jsdoc-jsproof-20260926`): `example.remote.js` (JS)
with JSDoc types. SSR bundle:
`init_remote_functions(example_remote_exports, "src/routes/example.remote.js", "2b61k")`.
Client node has `` ge(`2b61k/record`) `` / `` ye(`2b61k/status`) ``.
`skgo_remotes_gen.go` still comments `.remote.ts#record` while
`skgo_bindings_gen.go` spells `Module: "src/routes/example.remote.js"` —
path mismatch on disk.

No logs, screenshots, or captured output survive from any prior run.

**Operator rerun (2026-09-26, after read-only research).** On the jsproof app:
`pnpm run check` exit 0, 0 errors. `record(123)` negative check exit 1 with
number/string diagnostic. Type-only `Status` alias import passed; wrong
`writes: string` rejected. `pnpm run build` exit 0 with `remote-2b61k.js`.
Earlier, with stale `.ts` ID `1ptltty`, the adapter failed its
generated-versus-compiled hash comparison. After aligning the disposable remote
manifest and Go binding path, the Go binary served HTTP 200 SSR markup and
`/_app/remote/2b61k/status` returned JSON from Go.

**Inference.** The failure mechanism: `checkRemoteHashes`
(`skgo-adapter.js:653`) compares hash prefixes in `skgo.remotes.json` against
kit's `manifest.remotes` keys. A `.ts`→`.js` switch changes the hash on both
sides; if one side is regenerated before the other, the gate fires.

**Unresolved.** What `record(123)` returned in the earlier uncaptured run.
The operator rerun's `pnpm run build` exit 0 proves `checkRemoteIds` passed on
this `.js` build — it is called unconditionally in `adapt`
(`skgo-adapter.js:81`) after `builder.writeClient`, in the same throwing
sequence as `checkRemoteHashes` (`:72`) and `checkServerLoads` (`:76`).
Uncertainty remains for: an auto-generated JavaScript-form mode, loads/actions
against the `.server.ts` regexes (`skgo-adapter.js:235,245`), and
`checkServerLoads` with a non-empty load set.

---

## Implementation map: declarations requiring JSDoc

For JavaScript-form output, every generated **function** needs JSDoc because
JavaScript has no type annotations:

1. **Remotes** (`.remote.js`): `export const <name> = <factory>(…)`. JSDoc
   carries the verified `$app/server` return type for each factory:
   `RemoteCommand<Input, Output>` (`types/index.d.ts:3367`),
   `RemoteQueryFunction<Input, Output>` (`:3468`, also `query.batch()` at
   `:3718`), `RemoteLiveQueryFunction<Input, Output>` (`:3479`, for
   `query.live()`), or `RemoteForm<Input, Output>` (`:3297`). Plus `@param`,
   `@returns`, and the doc comment. Must not add runtime type exports — kit's
   `init_remote_functions` throws on any non-function export
   (`src/exports/internal/server/remote-functions.js:11-27`).

2. **Loads** (`+page.server.js` / `+layout.server.js`): `export const load =
   …`. JSDoc with the generated `./$types` identifiers `PageServerLoad` /
   `LayoutServerLoad` (emitted by kit at
   `src/core/sync/write_types/index.js:426`). The universal `Load` type
   (`types/index.d.ts:420`) is not exported from `$app/server` and kit's own
   doc comment says to import generated types rather than use `Load` directly.
   `readGenerated` gate requires `/\/\+(page|layout)\.server\.ts$/`
   (`skgo-adapter.js:235`) — must change for `.js`.

3. **Actions** (`+page.server.js`): `export const actions = { … }`. JSDoc
   with the generated `./$types` identifiers `Action` / `Actions` (emitted at
   `write_types/index.js:282-283`). Same `.server.ts` regex gate
   (`skgo-adapter.js:245`).

4. **Endpoints** (`+server.js`): `export const GET = …`, `POST = `, etc.
   JSDoc with kit's route handler types (e.g. `RequestHandler`). The proposed
   `@skgo/sveltekit-adapter.EndpointHandler` type is **unsupported** — use
   verified kit route types or describe as to-be-proven. `endpointStubPath`
   hardcodes `+server.ts` (`endpoints.go:166-168`).

5. **Named wire types** (`types.js`): polytype's `typescript.Generate` emits
   `/** … */` JSDoc on type aliases and properties today. For JS output,
   `export type Status = {…}` becomes `/** @typedef {…} Status */`. Polytype
   issue #157 requests this projection; skgo should consume it rather than
   invent a second wire-type grammar.

**What polytype can and cannot do.** v1.1.0 has a working JSDoc formatter
(`writeDoc`/`sanitizeComment`) and `Description` plumbing for types and
properties. The grammar has no function node, so function-level JSDoc
projection does not exist. skgo owns function-level JSDoc for
remote/load/action/endpoint signatures — no Polytype grammar extension is
needed for those.

**Extension-sensitive surfaces.** A `.ts`→`.js` switch touches: `scan.go:338`,
`gen.go:309-312`, `endpoints.go:166-168`, `emit.go:107-110`, the three
`stubSignature` templates (`emit.go:160,169,177`), `fn.module` strings hashed
by `kithash.Kit` (`emit.go:745`), and `readGenerated`'s regexes
(`skgo-adapter.js:235,245`). The fingerprint gate (`adapter.go:131-164`) is
extension-blind.

---

## Gaps and unresolved

- No captured output from the read-only researcher's session; operator rerun
  results are labelled as such.
- The aligned `.js` sample **passed** `checkRemoteIds` in the operator rerun
  (`pnpm run build` exit 0); the gate is called unconditionally at
  `skgo-adapter.js:81`. Automatic JavaScript generation is not yet implemented.
- Whether skgo's generator consumes `typegrammar.Definition.Description` —
  not checked; skgo passes `typescript.Options{}` (`types.go:395`).
- JSDoc expression syntax for each remote kind (batch, live, form) should be
  established by a checked consumer fixture before choosing emitter templates.
- A `never`-typed throwing helper alone may not provide the desired public
  remote signature.
- `NewStaticHandler` (`static.go:330`) reads the manifest without comparing
  `SkgoAdapter`; only `ReadManifest` enforces it.
