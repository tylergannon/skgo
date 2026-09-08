# Polytype programmatic generation

correction: The remote version list was stale relative to newly merged Polytype PR #113. The new programmatic configuration API is at commit d17dc67, after rc.11 and not yet tagged; refresh the upstream branch and release page before concluding the current pin is latest.

decision: skgo must request only the TypeScript and devalue projections it consumes. It must not restore build-tagged declarations, schema accessors, JSON Schema directories, or CLI execution merely to adopt the new high-level API.

discovery: `codegen.Gen` accepts executable `polytype.Configuration`, whose roots are created by generic `Declare[T]`; skgo discovers roots as `go/types.Type` values across multiple application packages. The new high-level API cannot construct that configuration dynamically and explicitly rejects a request spanning packages. Replacing skgo's existing `grammar.Load`/`Lower` plus backend calls would require generating and executing a temporary Go driver that spells every concrete type, recreating the synthetic-bindings mechanism the API was meant to remove. Keep the direct public grammar/backend path; it already emits only TypeScript and devalue artifacts and no JSON Schema.

decision: Until the merged commit receives its release tag, pin Polytype commit d17dc67 through Go's reproducible pseudo-version in the library, example, and new-app default. Replace it with the release tag when upstream publishes one.

proof: `just test` passes both modules against d17dc67. A real `just generate` is idempotent and produces no `skgo_polytype_gen.go`, `jsonschema_gen.go`, or `jsonschema/` artifact.

correction: Polytype v1.0.0-rc.12 was published at 2026-09-08T05:02:54Z after the pseudo-version checkpoint. Replace the temporary commit pin everywhere with the release tag; rc.12 also includes configurable sealed-union discriminator inflection from #114.

proof: Focused generator and scaffold tests and the full two-module `just test` suite pass against the published v1.0.0-rc.12 tag.
