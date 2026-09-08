# Dev routing comes from the dev server (#40, mission 3)

## `just dev` did not start in a tree that had never run it

Two modules the engine needs are written by kit's own dev server, lazily, on the
first request its middleware answers (`exports/vite/dev/index.js`,
`init_manifest`): `<outDir>/generated/dev/client/nodes/*.js`, which the node
table used to be read out of, and `<outDir>/generated/dev/server.js`, which
kit's runtime resolves `<sveltekit:generated>/server.js` into. Go is not a
browser — it boots the engine as soon as vite is listening — so on a checkout
where `.svelte-kit/generated` does not exist, the Go process exits before
anything has made kit sync. Both are gone: the node table is now built from
`create_manifest_data` directly, and the plugin calls kit's own `write_server`
in `configureServer`.

It reproduces as `rm -rf example/web/.svelte-kit/generated && just dev`, and it
was already true before this change.

## Kit's dev numbering is not the manifest's, and the failure is silent

Adding `src/routes/aaa-scratch/+page.svelte` while both servers ran, before this
change: `/aaa-scratch` answered 404, and `/about` answered **200 carrying the new
page's markup**. Kit numbers nodes by walking `src/routes` — layouts and error
pages first, then leaves, in traversal order — so a page added ahead of the
existing leaves shifts every one of them by one. The engine's node table had
followed (mission 1 invalidates it), Go's route table had not, and every leaf
index Go supplied then named the component next door. Nothing logged.

## The page options dev can see are not the ones the build records

The build reads a node's `ssr`/`csr` off the compiled module, so `ssr = !dev`
folds to `true` and `csr = dev` to `false`. Dev has kit's static analyser, which
returns null for any option that is not a literal — so under `vp dev` the root
layout and `/plain` now state no opinion, and `/plain` gets a boot script it did
not get while dev was reading the last build's answer. That is the correct
answer for dev (the build's is a fact about a build), and it is the shape
mission 4 leaves behind once those two lines are deleted. `/spa`'s `ssr = false`
is a literal and survives.

Kit's own `create_node_analyser` inherits a parent's options into a child, and
the parent here is unanalysable, which would have poisoned `/spa` too. The
plugin calls `get_page_options` per node instead, which is what the build's
`option()` does.

## Remote ids cannot diverge between dev and the build

`exports/vite/index.js`'s remote plugin assigns `hash(posixify(relative(root,
id)))` in both, from kit's `utils/hash.js`; `skgo generate` writes the same id
from `internal/kithash.Kit` over the same path (`internal/gen/emit.go`). The id
is a pure function of where the file is, so the only way to move it is to move
the file — which needs `go generate` and a Go rebuild before Go can answer it at
all. The dev manifest therefore carries no remote list; `RemoteConfig.Dev`
already turns the build's check off. The dev suite exercises the agreement
end to end: a route the scenario writes at runtime calls `getSite` and gets Go's
answer in the document.

## A per-request sync has to know which requests are routing's

`DevManifest.Intercept` sits outside every registry, and a page load under
`vp dev` is several hundred module requests. Asking the dev server about the
route tree once per request put a round trip and a global mutex in front of
every module the browser fetched. It now asks only for requests no vite prefix
and no app-directory prefix claims — documents, `__data.json`, server routes,
files out of `static/` — which is the set whose answer a moved route tree
changes.

## A mutation that does not bite is a scenario that is not about what it says

Skipping `ssr.setNodes` on a route change — the renderer keeping the build's node
table — left the whole dev suite green. In kit's dev numbering a node's `index`
is its position, and every node in the example app has a component, so the two
tables differ only in the page options each position carries. The scenario that
catches it is `/spa` still arriving as the shell after a route has been added
ahead of it.
