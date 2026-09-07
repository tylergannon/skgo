# The transport hook inside the SSR engine

Branch `claude/ssr-transport-in-engine`, off `964ae17`. Traps and decisions.

## The seam is the wire format, not a second encoder

`202609062040` left the engine with `get_hooks = () => ({})`, so a component
that called a method on a transported value threw and the visitor got the shell.
The fix is not to teach the engine about Go types: it is to stop handing the
engine a *different* encoding from the one the browser gets.

A load result and a remote answer now cross into the bundle as
`devalue.StringifyWith(tree, transport.reducers())` — the same bytes, from the
same call, that `__data.json` and `/_app/remote/...` already send the client —
and the bundle reads them back with kit's own `#app/internal/transport`, whose
`parse` is `devalue.parse(data, decoders)` over the app's `src/hooks.ts`. So the
`Money` a component renders against and the `Money` the client hydrates are
built by one `decode`, and there is no third code path to keep in step. The
document's hydration output did not have to change and did not: it is still
`price:app.decode("Money", {cents:4500})`.

`ssr.Node.Data` and `remoteAnswer.V` are therefore **strings**, not JSON values.
A reviewer reading `{"v": "[[\"Money\",1],...]"}` on the host boundary should
read it as "the wire", not as double encoding.

## `#app/internal/transport` cannot be imported by its own specifier

The bundle's virtual modules are loaded with `resolveDir: cwd`, so a `#`-prefixed
package import inside one resolves against the *app's* `package.json`, not kit's,
and fails. It is aliased by absolute path instead
(`skgo:kit/transport` → `<kit>/src/runtime/app/internal/transport.js`), which is
the same file kit's own modules reach, so esbuild dedupes it and the bundle holds
one installed transport rather than two.

## The failure mode of a missing transport is loud, and that is new

With the hooks module stubbed back out to `{}`, `devalue.parse` throws
`Unknown type Money` from inside `build_props`, the render fails, and
`TestEveryRouteInTheManifestIsServed` reports the shell. It does not silently
hand the component a plain object. That is worth knowing before anyone
"simplifies" the entry back to plain JSON: the old shape failed at the first
method call, deep in a component; this one fails at the boundary.

## `init_transport` runs once per runtime, not once per request

Kit calls it per request (`runtime/server/index.js`) because it loads the hooks
with a dynamic import. The bundle has no top-level await — `dynamic-import` is
one of the four syntaxes turned off for the engine's parser — so the entry
imports `skgo:hooks` statically and calls `init_transport` at module scope. The
hooks are a build-time constant, so the two are the same thing.

`skgo:generated` stays `get_hooks = () => ({})`. The only other thing kit's
`get_hooks` carries is `reroute`, and Go does the routing.

## The app's hooks file has to be found the way kit finds it

`builder.config.files.hooks.universal` is an absolute path with **no
extension** (`core/config/index.js` resolves it and defaults it to
`<src>/hooks`). `resolveEntry` in the adapter is kit's `resolve_entry`
(`utils/filesystem.js`) — file, then `index.js`/`index.ts` under a directory,
then a sibling whose basename matches. An app with no hooks file, or one that
declares only `reroute`, still builds: the module is a namespace import with
`?? {}`, not a re-export, so a missing named export is not a bundle error.

## `{#if browser}` is gone from /pricing and should not come back

The guard existed only because the engine had no `Money`. The page now calls
`data.featured.price.format()` unconditionally and the price is in the raw
document. `example/ssr_test.go`'s
`TestATransportedValueReachesTheEngineWithItsType` is what holds that: it fails
on both halves — no `$45.00` in the markup, no `app.decode("Money", …)` in the
boot script — when the bundle's transport is removed. Verified by removing it.

The plans list below it still renders `plans-pending` on the server. That is
Svelte's rule about `pending` snippets, unchanged and unrelated.
