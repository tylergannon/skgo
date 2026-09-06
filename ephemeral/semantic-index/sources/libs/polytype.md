# polytype (go-gen-jsonschema) — external dependency pointer

**Materialized in the token cache at `reference/polytype/`** (mirror of the pinned
module plus the hosted llms.txt; see `reference/polytype/README-LOCAL.md` for refresh
commands). Cited here because the brief names it as the type pipeline and the
junkyard used its predecessor API.

- Module path (still): `github.com/tylergannon/go-gen-jsonschema`; published as
  `github.com/tylergannon/polytype`. Latest at index time: **v1.0.0-rc.5**
  (`go list -m -versions github.com/tylergannon/polytype`).
- Local mirror: `reference/polytype/` — `README.md` (authoritative for rc.5),
  `llms.txt` (module copy), `hosted-llms.txt` (website copy, AHEAD of the release),
  `cli-source/main.go` (flags), `typescript-source/` (the TS emitter),
  `skills/go-gen-jsonschema/SKILL.md`, `examples/`, `website-docs/`.
- Module cache origin: `~/go/pkg/mod/github.com/tylergannon/polytype@v1.0.0-rc.5/`
  (older v0.11.3 also cached — it predates TypeScript emit; do not read it).

## TRAP: hosted docs are ahead of the released module
`reference/polytype/hosted-llms.txt` documents a fluent API
(`polytype.Declare(T.Schema).Enum(...).Interface(...)`, `.Ref()`,
`.RenderProviders()`), a `go tool polytype` binary, and
`import "github.com/tylergannon/polytype"`. **None of that exists in rc.5**: no
`func Declare`, the binary is `gen-jsonschema/`, the import path is
`github.com/tylergannon/go-gen-jsonschema`. Code against `reference/polytype/README.md`
and `llms.txt` (module copies) unless the project deliberately pins an unreleased
commit. The hosted file's legacy→fluent table is the migration map for later.

## Purpose
Generate JSON Schema, Go `ValidateJSON` methods, enum/union JSON codecs, and
**structural TypeScript declarations** from Go types, driven by no-op marker calls
in a build-tagged `schema.go` that the generator reads from the AST.

## Facts that shape skgo's codegen
- `--typescript DIR` writes `DIR/types.ts`; `--typescript-barrel` adds `index.ts`
  with type-only re-exports. `reference/polytype/README.md` `## TypeScript declarations` (~L370-L400);
  CLI flags in `reference/polytype/cli-source/main.go:L78-L79`; output planning in
  `reference/polytype/typescript-source/typescript_output.go`; projection in `reference/polytype/typescript-source/generate.go`
  (Time → string ~L78, Optional/Nullable/OptionalUnion ~L145-L160).
- TypeScript output is **declarations only**: no runtime decoders/validators in TS
  (README ~L394-L400 and ~L440-L448; upstream issue #71 tracks a cross-language
  transport proof). skgo's client stubs therefore do plain `JSON.parse` /
  devalue on the wire and trust the types; Go side validates with `ValidateJSON`.
- Combined TS + generated owner codecs requires **>= v1.0.0-rc.5**; rc.4 has TS
  but no codecs (README `### Adopt TypeScript declarations with Go JSON codecs`).
- Generator only overwrites files carrying its generated-file header; an
  application-owned `types.ts`/`index.ts` is never clobbered.
- `-no-changes` / `JSONSCHEMA_NO_CHANGES=1` makes a CI drift check for both
  schemas and requested TS artifacts.
- Registration API (README `## 📖 Registration API`): `NewJSONSchemaMethod(T.Schema, WithEnum(field), WithStringerEnum(field), WithInterface(field, Discriminator(..), Impl(..)))`,
  `NewJSONSchemaFunc`, `NewJSONSchemaBuilder[T]`. The junkyard's generator
  discovered these same registrations (`WithEnum`, `WithStringerEnum`,
  `WithInterface`) to drive its own TS emit — that path is now redundant with
  `--typescript`. See `sources/junkyard/remote-codegen/typeglue.md`.
- Limits: only direct one-dimensional slices for interface unions; `Nullable[I]`
  unsupported; time values are strings; numbers are `number`.
- Generation has no Node dependency. Its own TS conformance lane
  (`tests/typescript/README.md`) is dev-only.
- Agent skill shipped in-module: `skills/go-gen-jsonschema/SKILL.md` (also
  available in this session as the `go-gen-jsonschema` skill).

## Recipes
- Add types for a remote function's input/result: write `schema.go` with
  `//go:build jsonschema` registrations, then
  `//go:generate go tool gen-jsonschema --validate --typescript <dir> --typescript-barrel`.
- Decide the one `types.ts` location per Go package: polytype emits one
  `types.ts` per `-target` package; skgo's generated `.remote.ts` must import
  from wherever that lands (junkyard placed it under `src/lib/skgo/types/<encoded-pkg>/types.ts`).

## Open question for skgo
Whether skgo invokes `gen-jsonschema` per package as a subprocess / `go tool`,
or links `internal/builder` (internal → not importable). Plan for the CLI route.

## Status note (2026-09-05)
Tyler is preparing **polytype v1.0.0-rc.6**, expected to ship the fluent
`polytype.Declare(...)` surface that `hosted-llms.txt` already documents. Plan:
target rc.6 when it lands (refresh the mirror per `reference/polytype/README-LOCAL.md`),
fall back to the rc.5 `NewJSONSchemaMethod` surface only if rc.6 is not out when
codegen work starts. Re-check the module import path at that time: it may
become `github.com/tylergannon/polytype`.
