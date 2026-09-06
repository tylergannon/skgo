# Research: inputs for a `.remote.ts` bindings generator

Read by an Opus researcher on 2026-09-06. Sections 2, 4, 5 were sourced from the
semantic-index leaves (`ephemeral/semantic-index/sources/`) because the token cache was
broken at the time; it is restored now — re-verify against `ephemeral/inspiration/` before
building on a line number.

## 1. Polytype (latest: v1.0.0-rc.9)

- Module path is now `github.com/tylergannon/polytype` (the old `go-gen-jsonschema` path is
  gone). Nothing in skgo references it yet.
- Highest published is **v1.0.0-rc.9**; `@latest` resolves to **v0.11.3** (prerelease rule),
  which predates TypeScript emit entirely. Pin `@v1.0.0-rc.9` explicitly, including in
  `go get -tool`. Module cache holds rc.5, rc.9, v0.11.3 — not stale.
- rc.9 public API (`declare.go:24-66`): `Declare[T](T.Schema)` with chained `.Accessor`,
  `.Method`, `.Function`, `.StringerEnum`, `.Ref`, `.RenderProviders`; `SealedUnion[I]`
  (`sealed_union.go:25`); `Optional[T]`/`Nullable[T]` (`optionality.go:19,51`). Enums implicit
  via `func (T) enum()`; unexported-method interfaces are unions. `NewJSONSchemaMethod` etc.
  still exist but are deprecated. Markers are AST-read, no-ops at runtime.
- **TS emission is CLI-only** (`internal/typescript/`, not importable). Flags
  (`main.go:51-58`): `--typescript DIR`, `--typescript-barrel`, `--validate`, `-target`,
  `-no-changes`, `-force`. Output `DIR/types.ts` (`internal/typescript/generate.go:51,58`).
- Projection (`generate.go:67-166`): bool/string/ints/floats → `boolean`/`string`/`number`;
  `time.Time` → `string`; struct → object keyed by json tag; empty struct → `object`;
  **`*T` → `T` non-optional non-nullable (pointer silently unwrapped, `:119-120`)**;
  slice/array → `Array<T>`; named type → exported alias; `Optional[T]` → `"x"?: T`;
  `Nullable[T]` → `T | null`; enums/sealed unions → literal/discriminated unions;
  **`map[K]V` and `chan` unsupported** (`:134-135`).
- The junkyard's `-no-changes` defect is fixed in rc.9 (`internal/builder/builder.go:127-150`).

## 2. Junkyard prior art (`ephemeral/semantic-index/sources/junkyard/remote-codegen/`)

- Go-first: author writes `*.remote.go`; generator emits `.remote.ts` plus a Go registrar
  (`design-plan.md:9-13`).
- CLI (`skgo remotes generate [--check]`), explicit roots, never loads `./...` so the first
  clean run works before the registrar exists (`design-plan.md:69-76`).
- Module path came from a `//skgo:remote output=...` directive; POSIX, app-relative, under
  `src/`, ends `.remote.ts` (`design-plan.md:126-169`).
- The hash was *captured* from a Vite plugin during `vp build`, not computed
  (`design-plan.md:37,116-134`). Superseded: skgo computes it (`internal/kithash`,
  `remote.go:86`), so the Stage-B circular dependency (`design-plan.md:200`) no longer exists.
- Type-glue traps: bare `*T` becomes non-null; `Optional[T]` needs `json:",omitzero"`; skip
  `--typescript-barrel` (`typeglue.md`).
- Kit rejects `*.server.remote.ts` as server-only; use `data.remote.ts`
  (`lessons-and-traps.md:20-24`).

## 3. Current Go surface (`remote.go`, `remote_live.go`, `example/remotes.go`)

`Remote` (`remote.go:65-74`) retains `module`, `name`, `hash`, `id`, `kind`, two closures.
Accessors: `ID()`, `Module()`, `Name()` (`:77-83`). Missing:
- `kind` is unexported with no accessor (`remoteKind`, `:55-61`).
- No `reflect.Type` for `In`/`Out`: `Query`/`Command` (`:98-109`) erase them into
  `callAdapter` (`:132-144`); `LiveQuery` (`:114-130`) into the `live` closure.
- `None` is `struct{}` (`:28`) with no marker; the wire distinguishes presence per request
  (`decodeArg`, `:149-162`), not per registration.
- The module path is a Go string constant (`example/remotes.go:15`, used `:47-50`) that must
  equal the emitted file path — dual ownership.

Consequence: a runtime-reflection generator needs `Remote` to retain reflect types and expose
kind; the alternative is `go/types` over the registration source.

## 4. What kit needs from the stub (`example/web/src/lib/todos.remote.ts`)

- Client transform discards the module body and emits
  `export const <name> = __remote.<type>('<hash>/<name>')` (`exports/vite/index.js:L703-L757`).
- `'unchecked'` is required, not preferred: without a schema kit's validator rejects any
  argument with 400 (`shared.js:L12-L40`).
- `init_remote_functions` iterates `Object.entries(namespace)` and throws unless every runtime
  export has `__.type ∈ {command, form, prerender, query, query_batch, query_live}`
  (`remote-functions.js:L11-L27`): no `export default`, no exported helpers; type-only exports
  are erased and fine.
- `query.live` is a property on `query` (import `{ query, command }` only).

## 5. Build ordering / discovery

- Discovery is a Vite `transform` on the module id pattern `/[/.]remote\.[^/]+$/`
  (`exports/vite/utils.js:L144,L158-L164`): the file must be **imported by app code** to reach
  the client bundle.
- Prod: server bundle builds first, `analyse()` loads each remote chunk and records
  `{type, dynamic}` per export (`exports/vite/index.js:L1201-L1225`,
  `core/postbuild/analyse.js:L148-L166`); the client build reads it (`L722-L731`). Generating
  the file before `vp build` suffices. Requires `experimental.remoteFunctions: true`.
- Dev: add/unlink → manifest rebuild + `full-reload` (`exports/vite/dev/index.js:L153-L358,
  L402-L449`); edit → HMR accept re-runs `query(id)` (`index.js:L750-L752`). Kinds are
  re-learned per transform.

## Traps

1. `go get polytype@latest` gives v0.11.3. Pin rc.9.
2. Index leaf `sources/libs/polytype.md` is stale (old module path, rc.5, `-no-changes`
   defect). Fix or ignore.
3. `map[K]V` unsupported by polytype's projector; bare `*T` flattened to non-null while
   `encoding/json` marshals nil as `null`.
4. A generator cannot read kind or In/Out from a `*Remote` today.
5. Exported non-remote helpers in the stub break the build.
6. Module path string vs emitted path: renaming the file changes every URL and cache key.
