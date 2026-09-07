# polytype rc.11: the TypeScript backend as a library (#68)

## A foreign root needs no local redeclaration

The `wiretypes/` relocation existed because polytype's CLI had to be pointed
at a package that *declares* the root, and a dependency's directory in the
module cache is read-only. The library path has no such rule: `grammar.Load`
on any app package that imports the dependency, then `Lower` with the
dependency's type looked up through `Types().Imports()`, loads the foreign
package on demand from the importer's dependency graph. polytype's own
`TestLowerAnonymousAndCrossPackageRoots` is the proof it was built for. So
issue #111's "follow-on" (`LoadPattern`) never needed to land: the importer is
always an app package, because a foreign type reaches the wire through a
marked signature, and that signature's package imports it.

## polytype renames a root when its name is claimed twice

`typescript.Generate` is collision-safe: a package whose `Thing` has a field of
`wire.Thing` gets one of them emitted as `Thing$<hex>`. The stubs import by Go
name, so under the CLI path that case produced a stub importing a name the
`types.ts` did not export, silently. The library path returns `Result.Names`,
which is what makes the refusal possible: `projectTypes` compares every root's
emitted identifier with its Go name and refuses the package if they differ.
Nothing in the example exercises it; `TestATypeThatReachesAnotherOfItsOwnNameIsRefused`
does.

## The route root's link now holds only links

`syncPerFile` kept any real file it found in the link directory, because
polytype's `jsonschema_gen.go` had to live where `go:embed` could see it. With
nothing generated into a link directory any more, the exception was a standing
rule for a thing that no longer exists, and it would have preserved the stale
schema output of an earlier run in the compiled package. It now removes
whatever is not a link.

## Upgrading an app generated before this change

`skgo generate` does not delete the files the CLI path wrote. An app that ran
an earlier skgo deletes `skgo_polytype_gen.go`, `jsonschema_gen.go` and
`jsonschema/` from every package with a type on the wire, and
`generated/wiretypes/`, and drops `tool github.com/tylergannon/polytype/polytype`
from go.mod. The example was cleaned with `git rm`; no other app exists yet,
which is why a sweep was not built.
