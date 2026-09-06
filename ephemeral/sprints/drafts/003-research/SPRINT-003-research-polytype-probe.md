# Probe: what polytype v1.0.0-rc.9 actually generates

Empirical, 2026-09-06. Scratch module with a plain `Todo` struct, `Optional`/`Nullable`
fields, an enum, an embedded struct, and a sealed union; ran `go tool polytype --typescript`.

## Generated Go

- Plain struct (`Todo`): only `func (Todo) Schema() json.RawMessage` and (with `--validate`)
  `func (Todo) ValidateJSON([]byte) error`. **No `MarshalJSON`/`UnmarshalJSON`.** Same for
  enums. `Optional[T]`/`Nullable[T]` codecs are hand-written in the runtime package.
- Codecs are generated only for a struct that owns a sealed-union field (`MarshalJSON` +
  `UnmarshalJSON` injecting the discriminator). That is the entire codec surface.
- The README says so: "schema generation does not create a general-purpose Go codec or
  guarantee a typed encode/decode round trip for every Go type"; TS declarations "do not
  provide … definitive Go/TypeScript transport semantics; issue #71 tracks that proof."

## Field outcomes

| Field | Outcome |
|---|---|
| `Meta map[string]string` | Rejected: `mapType/chanType not allowed`, exit 1, nothing written |
| `Note *string` | **Silently widened**: schema `{"type":"string"}`, required; TS `"note": string` |
| `Due time.Time` | Accepted → `string` (RFC3339 description, no `format`) |
| `Tags []string` | Accepted → `Array<string>`, required; nil-vs-`[]` not modeled |

## Conformance — does not hold

`encoding/json` on `Todo{Note: nil}` → `"note":null`; emitted TS says `"note": string`.
Zero `Todo` → `"tags":null` against `Array<string>`. `wire.Item{Prio:"bogus"}` marshals
against TS `"low" | "high"` with no error. Only where a generated codec exists (the union
owner) does the round-trip match the TS exactly. `ValidateJSON` is opt-in and post-hoc.

## `types.ts`

- TS name == bare Go identifier, no package qualifier. Predictable; importable without
  parsing. Embedding flattens into the owner and also emits the embedded type.
- **One `types.ts` per run, overwritten, not merged.** Two packages → two output dirs;
  same-named types in different packages collide unless namespaced by dir.

## Registration

- `Declare` marker and the stub `Schema()` method must live in the type's own package
  (`undeclared local type found` otherwise, `internal/syntax/scan_result.go:703`).
- The *file* is free: a generated `skgo_polytype_gen.go` with a `Code generated` header
  works. **skgo can write the markers for the app author**, one file per package that owns
  an In/Out type, and run polytype once per such package.

## Gaps for the skgo flow, ranked

1. No codec for plain structs — the "marshaled Go ⇒ TS shape" guarantee is false for the
   common case. Needs generated `MarshalJSON` on every declared struct (normalize nil slice
   to `[]`, validate enums on encode), or rejection of every field whose Go encoding can
   deviate.
2. Pointer widening is silent. `*T` must become `T | null` in schema + TS, or be rejected.
3. Nil slice ⇒ `null` (fixed by 1).
4. Cross-package `Declare` (workaround exists: per-package generated marker files).
5. No maps (`Record<string, T>` is expressible in both targets).
6. `types.ts` overwritten per run (workaround: fan out one dir per package; skgo owns the
   barrel).
7. No machine-readable "Go type ⇒ TS name/file" manifest; name equality is an unstated
   invariant.

## Traps

- Flags are not sticky: a run without `--validate` deletes `ValidateJSON` from the generated
  file and breaks the build. Every invocation must carry the full flag set.
- `time.Time` is the only external type accepted.
- Union discriminators are concrete Go type names — renaming a variant is a wire break.
- Recursive types are rejected (max depth 100): a tree/comment-thread payload cannot be
  declared.
- `Optional[T]` needs `json:",omitzero"`; the generator does not always catch a missing one.
