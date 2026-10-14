# Targeted generation: upstream gopls boundary experiment

The useful target is less generation, not merely a faster full generation run.
This experiment calls upstream gopls's actual `typerefs` implementation, pinned
to tools commit `b8f586f8588e28f0c9fc12bcafd850db114ba049`. The root tests exercise
the algorithm; `snapshotprobe` runs the complete in-process language server.
Neither implements a new cache. The artificial nested module path
permits importing gopls's private packages for research; it is not a production
integration or a proposed public module name.

`golang.org/x/tools` is pinned to the same commit. Gopls's own `replace
golang.org/x/tools => ..` is not inherited by a consuming module; its declared
older tools requirement alone did not compile the complete snapshot test.

Run from this directory:

```sh
GOWORK=off go test -count=1 -v
GOWORK=off go test -run '^$' -bench . -benchtime=500ms
```

Five literal fixtures demonstrate:

- Type declarations, aliases, array lengths, inferred values, and signatures
  retain their external symbol references through the upstream index.
- Changing a workflow's called function leaves that type-reference index
  unchanged. Gimbal's body-sensitive workflow generation must track more.
- Changing a JSON field tag leaves the index unchanged. The graph is not an
  output fingerprint and cannot authorize cached TypeScript/JSON by itself.
- Changing a signature changes its dependency edge as expected.
- An explicitly typed constant's initializer is not in this type graph. The
  ordinary Go type checker proves that changing the imported constant changes
  the exported value and an array type's length anyway. Enum/value projection
  needs additional tracking. This algorithm-only test does not establish
  complete snapshot behavior; the integration experiment below tests that.

### Full snapshot follow-up

`snapshotprobe` goes beyond that isolated algorithm test. It uses upstream's
own in-process LSP integration runner with four small packages:
`storage.Code -> model.Explicit/Block -> api.New -> client.Value`.
The client assigns the returned array to `[1]byte`. Editing the constant from
1 to 2 must produce an assignment error; reverting must clear it.

```sh
GOWORK=off go test -count=1 -v ./snapshotprobe -timeout=45s
```

At the pinned upstream commit, the inferred-constant control passes; the
explicitly typed constant case fails after completed change-diagnosis work,
with no client diagnostic. This is an intentionally failing reproducer, not a
passing integration claim.

Independent validation confirmed the diagnostic is present on fresh startup
with `Code = 2`; the stale explicit-constant case remains after completed
diagnosis, an additional 250 ms, and saving the buffer. The ordinary Go compiler
rejects those saved files at the expected assignment. Opening the importer does
not repair the stale diagnostic; a comment-only edit to the importer does.
These controls support an invalidation failure at this exact pin, rather than
a fixture that never reports errors or an unfinished change notification.
Their preserved source is in `controls`:

```sh
GOWORK=off go test -count=1 -v ./controls -timeout=45s
```

This command characterizes the upstream defect and passes at the pinned version;
its stale-diagnostic assertion should fail after a repair. The correctness
regression is `snapshotprobe`, which should pass after a repair.

`typed-initializers.patch` conservatively visits initializer expressions even
when a value has an explicit type. In a separate private copy of the pinned
gopls module, with this single patch and a local `-modfile` replacement, both
cases pass (including reversal). The shared module cache was not modified.
The broad patch also retains typed-variable initializer edges, so it can cause
extra invalidation; it is a causal experiment, not a proposed upstream patch
ready for publication. An independent rerun confirmed both patched cases pass.
No gopls issue or PR has been sent.

On this Apple M4, the initial tiny-fixture parse+encode+decode benchmark was
7,088 ns/op. This is only the local algorithm cost for the small fixture,
excluding package discovery, type checking, full-project graph maintenance,
code generation, compilation, native reload, and browser latency. It is not a
developer-server performance claim.

## What to borrow and what to add

Upstream already separates metadata validity from local parsed information and
type-check keys. File membership, package/import changes, build constraints,
and embed directives trigger rediscovery; ordinary edits need not do so.
Unchanged package handles and imports can survive in a new immutable snapshot.
Its symbol-reference graph prunes transitive packages that cannot affect types.

The published Polytype v1.8.0 session still asks `go/packages` for metadata on
every typed load, before deciding whether a package can be reused. It solves
invocation sharing, not this incremental invalidation problem. Gimbal's final
selective-overlay comparison still performed 25 metadata loads, 34 source
checks, 5 package hits, and 6 ordinary fallbacks. Its changed-input run was
6.766 seconds versus 7.448 without sharing. The 0.599-second warm replay had
zero session work. None of those measurements establish targeted generation.

For the next experiment, distinguish three decisions:

1. Does package selection need rediscovery? Borrow the snapshot/metadata split.
2. Which generated outputs may change? Combine reachability with producer-owned
   inputs: projected declarations and tags, enum/constant values, route and
   configuration membership, docs where emitted, and workflow bodies where read.
   Track changes inside a package as well as its external edges; upstream's
   serialized graph collapses internal edges into external reachability.
   Gimbal also embeds source line numbers in generated graphs
   (`internal/generate/source.go`, `position`): inserting unrelated lines can
   change an output's source mapping without changing its semantic graph. Keep
   those mappings current even when graph analysis can be reused.
3. Which native code must be replaced? Include compiler dependencies such as
   inlined callers and generic instantiations. A type graph or unchanged API is
   insufficient; the existing restricted native POC remains a separate proof.

The smallest useful integration should record actual producer reads and show
that an unrelated edit invokes zero generators, a relevant body edit invokes
only its body-sensitive producer, and a projected field/tag/value edit changes
only its dependent outputs. Compare those output bytes with ordinary full
generation. Keep diagnostics/error recovery independent: skipping a projection
does not mean accepting invalid application code. Full invalidation remains
available for changes not yet understood; do not add TTL correctness.

## Pinned primary sources

- [Symbol reference algorithm and scope](https://github.com/golang/tools/blob/b8f586f8588e28f0c9fc12bcafd850db114ba049/gopls/internal/cache/typerefs/doc.go)
- [Declaration traversal; function bodies and explicitly typed initializers](https://github.com/golang/tools/blob/b8f586f8588e28f0c9fc12bcafd850db114ba049/gopls/internal/cache/typerefs/refs.go)
- [Package handle states, reachability, and source-based keys](https://github.com/golang/tools/blob/b8f586f8588e28f0c9fc12bcafd850db114ba049/gopls/internal/cache/check.go)
- [Metadata change classification](https://github.com/golang/tools/blob/b8f586f8588e28f0c9fc12bcafd850db114ba049/gopls/internal/cache/snapshot.go)
- [Go team's scalability explanation](https://go.dev/blog/gopls-scalability)

Production delivery remains PRs Polytype #167, SKGO #310, Gimbal #528. This
experiment belongs to deferred SKGO #311 and does not enlarge those PRs.
