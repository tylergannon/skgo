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

**Corrected twice, 2026-09-06 (Tyler).** First pass led with "no codec for plain
structs" — wrong: codecs earn their place only where the default encoder cannot
produce the target format (a union discriminator), and nil `*string` → `null` is
`encoding/json` behaving correctly. Second pass then asked to *widen* the
projection to `T | null` — also wrong: `Nullable[T]`/`Optional[T]` exist so
nullability and presence are stated, not inferred. The right ask is that the
un-stated forms be **diagnosed**, not silently accepted.

**The admitted subset is the design, not a limitation.** Scalars, `time.Time`,
structs, slices, named types, embedding, enums, sealed unions, `Optional`,
`Nullable` cover the remote-function boundary. Remote In/Out types are wire
contract types; every typed-RPC system narrows the host language at that
boundary. Cost to state in skgo's docs: an existing domain struct containing a
map or a bare `*T` cannot be handed to a remote function directly — the author
writes a boundary type.

1. Bare `*T` and bare `omitempty`/`omitzero` should be rejected with a
   diagnostic naming `Nullable[T]` / `Optional[T]`, rather than silently emitting
   a declaration that is false of the bytes. → tylergannon/polytype#100
2. Nil slices marshal to `null` and no wrapper covers it — the zero value hits
   the empty-list case immediately. Genuinely unresolved; three options weighed
   in the issue. → tylergannon/polytype#100
3. A run without `--validate` silently deletes `ValidateJSON` and breaks the
   build. → tylergannon/polytype#102
4. `types.ts` overwritten per run; no machine-readable Go-type → TS-name record.
   → tylergannon/polytype#103
5. `map[string]V` rejected. **Not blocking** — filed from a probe finding, not a
   use case. → tylergannon/polytype#101

Not filed: an enum-typed Go value outside its declared set (`Priority("bogus")`)
marshals happily against a TS literal union. Real, but a Go type-system
limitation rather than a projection defect.

## Traps

- Flags are not sticky: a run without `--validate` deletes `ValidateJSON` from the generated
  file and breaks the build. Every invocation must carry the full flag set.
- `time.Time` is the only external type accepted.
- Union discriminators are concrete Go type names — renaming a variant is a wire break.
- Recursive types are rejected (max depth 100): a tree/comment-thread payload cannot be
  declared.
- `Optional[T]` needs `json:",omitzero"`; the generator does not always catch a missing one.
