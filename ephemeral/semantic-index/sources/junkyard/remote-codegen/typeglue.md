# Junkyard remote-codegen: Go type -> JSON Schema -> TypeScript ("type glue")

Purpose: how endpoint types became `types.ts` and Go JSON codecs using go-gen-jsonschema
(now `polytype`; module path was still `github.com/tylergannon/go-gen-jsonschema`, pinned
`v1.0.0-rc.5`, source ref `90d5798`). Records the verified provider behaviour, the
registration contract, the staged-invocation workaround, admission rules, and how `.remote.ts`
imports the emitted types. Every claim here was executed against the pinned checkout.

## Key concepts and decisions

### Integration surface

- All projection code is under the provider's `internal/`; the root package exports only
  markers (`NewJSONSchemaMethod`, `WithEnum`, `WithInterface`, `AsRef`, ...), `Optional[T]`
  / `Nullable[T]`, and manual helpers. So integration was **by CLI**, once per Go package that
  declares endpoint types. `junkyard/ephemeral/remote-codegen/typeglue-source.md:L23-L39`
- Tool pin: `go get -tool github.com/tylergannon/go-gen-jsonschema/gen-jsonschema@v1.0.0-rc.5`.
  CLI: `gen-jsonschema [gen] -target DIR --typescript TSDIR [--typescript-barrel] [--validate]
  [--formats json|both] [-no-changes] [-pretty] [-force]`; `gen-jsonschema new -out FILE
  -methods 'T=Schema,U=Schema' [--validate] [--generate]`. `typeglue-source.md:L41-L52`
- Exact per-package invocation: `gen-jsonschema -target <pkg> --validate --typescript
  <app>/src/lib/skgo/types/<import-path-slug>` (no barrel). Slug is a reversible encoding of the
  import path, e.g. `example.com~skgofixture~remotes~accounts`.
  `junkyard/ephemeral/remote-codegen/plan.md:L333-L343`,
  `junkyard/ephemeral/remote-codegen/implementation/fixture-build-progress.md:L71-L72`
- `--typescript DIR` was briefly (wrongly) believed not to exist; confirmed real from
  `gen-jsonschema/main.go:78-79` and `internal/builder/builder.go`.
  `implementation/runtime-worklog.md:L168-L179`

### Registration contract (who writes the `//go:build jsonschema` file)

- Final decision: **developer-owned**. The author writes the build-tagged file with panic-stub
  `Schema()`/`ValidateJSON()` methods and `var _ = jsonschema.NewJSONSchemaMethod(T.Schema, opts...)`
  for each argument/result **root** type, options (`WithEnum`, `WithStringerEnum`,
  `WithInterface`) included. skgo reads it with a shallow `go/types` match on the exact marker,
  never writes/rewrites/inventories/deletes it, infers no options, keeps no support catalog.
  `plan.md:L315-L331`, `junkyard/ephemeral/remote-codegen/worklog.md:L50`
- History: the plan first had skgo *emit* the stub (`typeglue-source.md:L54-L83`, `:266-270`),
  Sol's typeglue review showed a bare marker cannot express enums/unions
  (`junkyard/ephemeral/remote-codegen/reviews/sol-typeglue-01.md:L23-L45`), an adjudication
  narrowed scope to plain structs (`worklog.md:L44`), then the user reversed to
  developer-owned registration with enums/unions in scope (`worklog.md:L50`).
- Nested named types need no registration — registering `Outer` emitted `Inner` too.
  Registration selects roots; the DAG below comes along. `typeglue-source.md:L112-L115`
- Missing root registration is one of the only two type-glue errors: fails naming binding,
  Go type, package, and the file to add it to. `plan.md:L328-L331`

### Stub shape (what the author file looks like)

```go
//go:build jsonschema
package accounts
import ("encoding/json"; jsonschema "github.com/tylergannon/go-gen-jsonschema")
func (Profile) Schema() json.RawMessage   { panic("not implemented") }
func (Profile) ValidateJSON(_ []byte) error { panic("not implemented") }
var _ = jsonschema.NewJSONSchemaMethod(Profile.Schema, jsonschema.WithEnum(...))
```
`typeglue-source.md:L60-L78`, `implementation/sonnet-generator-task.md:L92-L101`

- Consequence accepted: endpoint types gain `Schema()`/`ValidateJSON()` and their package gains a
  committed `jsonschema/` dir (`*.json` + `.sum`) and `jsonschema_gen.go` (`//go:build !jsonschema`,
  `embed.FS`). `typeglue-source.md:L80-L83`, `plan.md:L406-L408`

### Projection rules (verified by execution)

- One exported TS alias per named Go type, named after the Go type; property names are the
  `json` tag names, JSON-quoted; Go doc comments become JSDoc. `typeglue-source.md:L112-L120`
- Identifier safety: invalid runes hex-encoded, TS reserved words get `$type` suffix,
  cross-package base-name collisions get an injective hex suffix. `typeglue-source.md:L116-L118`
- `Optional[T]` (+ `json:",omitzero"`) -> `"x"?: T`; `Nullable[T]` -> `"x": T | null`;
  **bare `*T` -> non-null `T` in both schema and TS while nil marshals to `null`** -> skgo
  rejects bare pointer fields by binding name (its only own type rule).
  `typeglue-source.md:L121-L129`, `plan.md:L399-L403`
- Scalars: all int/float kinds -> `number`; `time.Time` -> `string` (RFC3339 in schema);
  slices and fixed arrays -> `Array<T>`; empty struct -> `object`; `WithEnum` -> union of exact
  literals; `WithStringerEnum` -> union of constant names; registered interfaces ->
  `Omit<Impl,"tag"> & { tag: "value" }` unions. `typeglue-source.md:L130-L135`
- `WithEnum` is for a plain named string type; `WithStringerEnum` is for iota ints with
  `String()`. `implementation/runtime-worklog.md:L163-L166`
- Verified real output for the enum/union fixture:
  `"tier": "free" | "pro" | "enterprise"` and
  `"payment": Omit<CreditCard,"kind"> & {"kind":"credit_card"} | Omit<BankTransfer,"kind"> & {"kind":"bank_transfer"}`;
  the generated Go `MarshalJSON`/`UnmarshalJSON` injects/strips the `kind` discriminator.
  `implementation/fixture-build-progress.md:L69-L94`, JSON schema at `implementation/runtime-worklog.md:L192-L231`
- Provider rejections (fail fast with `file:line`): `map[string]string` field ->
  `mapType/chanType not allowed`; self-referential `Node{Children []Node}` -> `cyclic dependency found`;
  also external package types other than `time.Time`, several interface container forms.
  `typeglue-source.md:L136-L140`, `:222-229`
- Admitted scope: document-shaped structs of bool/string/numeric/named scalars, slices/arrays,
  nested named structs, `Optional`, `Nullable`, `time.Time`, plus author-registered enums/unions.
  `plan.md:L375-L384`

### Codecs and validation

- `--validate` adds `ValidateJSON([]byte) error` (santhosh-tekuri/jsonschema v6, compiled once
  in `init()`), returning `*jsonschema.ValidationError` with `InstanceLocation`/`ErrorKind`/`Causes`.
  Order for untrusted bytes: validate, then unmarshal. `typeglue-source.md:L144-L152`
- Enum/union fields cause generated value-receiver `MarshalJSON` / pointer-receiver
  `UnmarshalJSON` on the owner struct; plain structs need no codec. `typeglue-source.md:L144-L148`
- TS side is declarations only — no runtime decoder or validator emitted. `typeglue-source.md:L153-L155`
- The provider describes the **ordinary JSON** wire form only; Kit's transport is devalue,
  owned by `go/remote/codec.go`; see `runtime-interface.md` for the bridge.
  `typeglue-source.md:L162-L191`

### Staged generation (the `-no-changes` trap)

- Source-verified defect at `internal/builder/builder.go:96-124`: `-no-changes` returns early
  only when schemas or TS would change; on the no-drift path it still calls `RenderGoCode()`
  and rewrites `jsonschema_gen.go` unconditionally; ownership protection covers the TS plan
  only. So `-no-changes` is NOT a read-only check. `plan.md:L349-L354`, `worklog.md:L52`
- skgo therefore always runs the provider in an isolated staging copy of the module (temp dir,
  same relative package as `-target`, TS to a temp dir) and compares: `--check` compares staged
  vs working tree and writes nothing; `generate` compares, refuses to overwrite an existing
  output that is neither inventoried nor ownership-headed, then copies staged artifacts in.
  `plan.md:L356-L369`
- Proof: `--check` catches a stale `types.ts` AND a stale `jsonschema_gen.go` and rewrites
  neither; read-only proved by full-tree hash before/after. `implementation/generator-progress.md:L74-L77`

### Inventory ownership of provider artifacts

- Stage A inventory owns `jsonschema/`, `jsonschema_gen.go`, `types.ts`, `.remote.ts`; all
  committed, `--check`ed, removed when a package stops declaring remotes. The author's
  registration file is never inventoried. `plan.md:L340-L345`

### How `.remote.ts` imports the types

- `import type { Document, ... } from '../../skgo/types/example.com~app~documents/types'`
  (extensionless, relative, type-only so Vite never sees it) followed by
  `export type { ... }` re-export. `plan.md:L177-L182`, `typeglue-source.md:L271-L275`
- Not verified in the source note but covered by the gate: `svelte-check` under
  `moduleResolution: bundler` accepts the declarations. `typeglue-source.md:L231-L233`,
  `plan.md:L460-L468`

### Dependency side effect

- First generation for a package adds the validator runtime import
  (`github.com/santhosh-tekuri/jsonschema/v6`, `golang.org/x/text`); skgo deliberately never
  edits `go.mod`, so the app needs `go mod tidy` (or pre-declared deps).
  `implementation/generator-progress.md:L92-L96`, `:199-207`

## Citations (bookmarks)

- Provider surface + CLI: `junkyard/ephemeral/remote-codegen/typeglue-source.md:L23-L52`
- Stub + consequences: `typeglue-source.md:L54-L83`
- Projection rules: `typeglue-source.md:L110-L140`
- Codec contract: `typeglue-source.md:L142-L160`
- Devalue vs JSON layering: `typeglue-source.md:L162-L191`
- AST patterns to copy from the provider: `typeglue-source.md:L193-L215`
- Executed probe table: `typeglue-source.md:L217-L229`
- What skgo still owes vs provider supplies: `typeglue-source.md:L257-L286`
- Plan's type-glue section (final contract): `plan.md:L305-L408`
- `-no-changes` defect: `plan.md:L347-L369`
- Real enum/union `types.ts` and schema: `implementation/fixture-build-progress.md:L69-L94`,
  `implementation/runtime-worklog.md:L192-L231`

## Reusable verdict against the new brief (polytype)

| Element | Verdict | Notes |
|---|---|---|
| Developer-owned `//go:build jsonschema` registration with `WithEnum`/`WithStringerEnum`/`WithInterface`; skgo reads, never writes | KEEP | Three rounds of review converged here. Do not reintroduce inference or a support catalog. |
| Only two type-glue errors (missing root registration; provider rejection re-reported with binding) + skgo's own bare-pointer rejection | KEEP | |
| CLI shell-out per package | KEEP WITH CHANGES | Re-check polytype: if projection is now importable (same author), an in-process call avoids the staging copy dance and temp dirs. Verify before assuming; at rc.5 it was `internal/`-only. |
| Staged copy + compare instead of trusting `-no-changes` | KEEP unless polytype fixed `builder.go:96-124` | Verify the defect against the current polytype source; if fixed, staged compare can shrink to a plain drift diff. |
| `types.ts` under `<app>/src/lib/skgo/types/<slug>/` with type-only relative import | KEEP WITH CHANGES | With Go colocated in route dirs, emitting `types.ts` beside the `.remote.ts` (or beside `+page.server.ts`) may be simpler than a slug directory; the slug scheme is only needed when several packages could collide. Keep the type-only import and the re-export. |
| Nested types auto-emitted; only roots registered | KEEP | |
| `Optional`/`Nullable` wrappers; reject bare pointers | KEEP | |
| `time.Time` -> string | KEEP WITH CHANGES | See runtime-interface.md; a devalue `Date` mapping is a possible upgrade. |
| Inventory owns provider artifacts | KEEP | |
| `go mod tidy` requirement after first generation | KEEP WITH CHANGES | Either pre-declare the validator dep in the app template, or have skgo run `go get` explicitly; do not leave it as a documented incantation. |
| Loads (`PageServerLoad` return types) through the same pipeline | NOT COVERED | Old design typed remotes only. The same registration -> `types.ts` path should serve `+page.server.ts` `PageData` typing; nothing in this segment proves it. |

## Gotchas and traps

- `-no-changes` rewrites `jsonschema_gen.go` even with no drift. `plan.md:L349-L354`
- Bare `*T` fields silently become non-null in TS and schema. `typeglue-source.md:L125-L129`
- `Optional[T]` needs `json:",omitzero"` to project as optional. `typeglue-source.md:L123`
- `WithEnum` vs `WithStringerEnum` are not interchangeable. `implementation/runtime-worklog.md:L163-L166`
- Skip `--typescript-barrel`; its `./types.js` specifier buys nothing. `typeglue-source.md:L271-L275`
- Provider diagnostics are per-type and lack the binding name; re-report with it. `typeglue-source.md:L276-L279`
- Generation needs no Node, npm or JS. `typeglue-source.md:L160`
- Registration file compiles only under `-tags=jsonschema`; verify with `go build -tags=jsonschema ./...`.
  `implementation/sonnet-generator-task.md:L133-L137`

## Recipes

- To author a registration file with an enum and a discriminated union: `implementation/sonnet-generator-task.md:L80-L101`, real output at `implementation/fixture-build-progress.md:L69-L94`.
- To run the provider into a throwaway dir for inspection: `implementation/sonnet-generator-task.md:L138-L142`.
- To implement staged compare-then-apply: `plan.md:L356-L369`.
- To copy loader/marker/diagnostic discipline for the Go scanner: `typeglue-source.md:L193-L215`.
- To see the exact set of provider rejections to expect: `typeglue-source.md:L217-L229`.
- To see the projection table for writing tests: `typeglue-source.md:L121-L135`.
