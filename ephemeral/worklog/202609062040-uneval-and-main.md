# The real `uneval`, and `origin/main` merged in

Branch `claude/ssr-no-node-sidecar-9e3be4`. Traps and decisions only.

## #35 moved the load's encoding downstream, and SSR was built against the old shape

Before #35 a `dataNode.data` was already a tree — `map[string]any` with a
`*deferred` wherever a load had promised something — because `encodeValue` ran
in the load path. #35 keeps the raw Go value instead and encodes it at
serialization time, which is the only place the app's transport hook is known.

Merging that into the SSR branch broke the document in two ways at once, and
only the first was loud:

- `uneval` refused the struct (`cannot write account.LayoutData into a
  document`), so `/account` and `/account/orders` fell back to kit's shell.
- `settle()` walks maps and slices; it does not descend into a Go struct, so a
  `skgo.Deferred` field was never settled and `Deferred.MarshalJSON` wrote
  `null` into the engine's request. `/account/orders` would have rendered a page
  with no orders and no error.

The fix is one encoding at the document boundary: `render()` calls
`Transport.encodeLoadValue`, then settles the tree, and stores the result back
on the node. The engine's JSON and the document's hydration array are then the
same tree by construction, which is what they have to be. `settle` also encodes
each settled value, because a `*deferred` holds the raw Go value for the same
reason a load does.

## A remote answer has to be kept as a Go value, not as JSON

`SSR.answer` used to record `json.Marshal(value)` for the boot script. That
throws away the type before any transporter can see it, so a `Money` reached the
browser as `{cents:2000}` and `price.format()` was not a function. The registry
now records the Go value (`answered{value, err}`) and the document encodes it;
the engine still gets JSON, because a component in it has no class to be handed.

## The engine has no transport hook, and that is why the scenario is a client one

`skgo:generated` in the adapter is `export const get_hooks = () => ({})`, so the
SSR bundle has no `transport` and no `Money` class. A component that calls
`price.format()` **while rendering** throws, the render fails, and the visitor
gets the shell — `TestEveryRouteInTheManifestIsServed` catches it.

So `/pricing` writes its featured price under `{#if browser}`: the value travels
in the document, the method is the browser's. Giving the engine the app's
`transport` so a domain type can be server-rendered is a slice of its own, not
done here.

Related and not a bug: `/pricing`'s plans list renders `plans-pending` in the
document. A `<svelte:boundary>` with a `pending` snippet renders that snippet on
the server and never runs the `await`, so `getPlans` is not called during the
render and is not in `<global>.data`. That is Svelte's rule.

## The load-stub generator did not import the transported class

`writeLoadStubs` dropped `projected.transported`, so a load returning a
transported type emitted `{ price: Money }` with no import and `pnpm check`
failed with "Cannot find name 'Money'" — from a generated file, during
`go test ./example/...`. The remote-stub emitter had always done this; the load
emitter never had a load that needed it.

## `git checkout --theirs go.sum` loses whatever only your side had

`00fa90d` predates `internal/ssr`, so taking its `go.sum` wholesale dropped
goja's hashes. The symptom is not a build failure: `go build` still works from
the module cache, and it is `skgo generate` shelling out to `go` that fails with
"missing go.sum entry for module providing package github.com/dop251/goja".
`go mod tidy` in both modules is the fix; `quickjs-go` fell out at the same time,
because nothing imports it any more.

## `pkill -f "example/cmd"` does not stop `just serve`

`go run ./example/cmd` compiles to a binary called `cmd` in the build cache and
execs it; the child's command line is the cache path, so no pattern containing
`example/cmd` matches it. `pkill` reports success, the port stays bound, and the
next `just serve` exits with "address already in use" — into a log nobody reads,
because it was backgrounded. The suite then runs against the *previous* build.

`lsof -nP -iTCP:8080 -sTCP:LISTEN` and `kill <pid>` is the reliable pair, and
`just ports` exists for the first half of it. Confirm the port is free *before*
starting the replacement, not after.
