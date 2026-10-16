# Structural Kit metadata lifecycle

This bounded experiment compares Kit's live dev metadata lifecycle with the adapter's full-build publication and Go app restart. It does not add implementation to skgo.

## Kit 3.0.0 lifecycle and publication boundary

Verified the installed pin with `node -p "require('./example/web/node_modules/@sveltejs/kit/package.json').version"` → `3.0.0`.

In the pinned source, `src/exports/vite/dev/index.js` owns route add/unlink handling. Its `update_manifest` recreates `manifest_data` with `create_manifest_data`, calls `sync.create`, publishes the object only through the `set_manifest_data` closure supplied by `src/exports/vite/index.js`, validates parameter matchers, and replaces the internal SSR `manifest` with `generate_manifest`. The route watcher debounces add/unlink by 100ms. The refreshed `sync.create` writes the client manifest, `$app/manifest`, server modules and generated types.

The client `app.js` publishes node import loaders, server-load flags and the route dictionary with exact client node positions. `$app/manifest` publishes only route `{id,page,endpoint}`. Kit's internal SSR manifest additionally has route regex `pattern`, `params`, page layouts/errors/leaf, endpoint loaders, and matcher loaders, but the Kit dev plugin does not provide an adapter callback for that object. Go's `skgo.manifest.json` needs this richer route/node identity. Thus the Kit-owned *dev* update is not itself a Go metadata publication seam; polling the public app module cannot reconstruct the full Go manifest without parsing/reimplementing Kit internals.

A supported full-build boundary does publish complete metadata: the adapter's `adapt(builder)` reads `builder.manifest`, builds the Go manifest and Goja bundle, and writes `build/skgo.manifest.json`. A structural fallback can therefore build the frontend and regenerate Go registration before restarting the application. This demonstrates the build boundary, not live dev graph synchronization or a selected product architecture.

## Executed structural rebuild fallback

All changes were in the isolated copied project under this experiment directory; the shared example tree was not changed. The copied app added:

```svelte
<h1>STRUCTURAL_LITERAL_7719</h1>
<a href="/">Home</a>
```

at `/aaa-structural`, plus a link from the home page. A separate `/account/aaa-structural` route was added before `/account/orders` to force a later node-number shift. The observations recorded after `vp build` were Go manifest `/account/orders.page.leaf: 11` and client dictionary `"/account/orders": [~12,...]`, then leaf `12` and dictionary `"/account/orders": [~13,...]`. In Kit's `write_client_manifest.js`, `~` marks server data and is emitted before the route leaf index; the client parser detects a negative value and complements it back to its original client node index (`runtime/client/parse.js`). Thus `~13` evaluates to `-14`, then parses to client index `13`. Go's `page.leaf` is a position in its manifest node array, while Kit's client entry is an original client node index; these numbers are not directly comparable. The paired generated files and raw build output were removed with the isolated fixture, so this experiment does not preserve evidence to audit their mapping or claim coherence for `/account/orders`. The actual direct SSR and client navigation proof below covers the added root route, not `/account/orders`.

The first Go start against the new build correctly refused with:

```text
skgo: caller manifest drift: route "/account/aaa-structural" has no generated Params constructor; regenerate and rebuild Go
```

After running `go generate ./...` in the copied app, restarting its actual Go server returned `/aaa-structural` as HTTP 200 with the literal `STRUCTURAL_LITERAL_7719` in the document and `node_ids: [0,10]`. `/about` remained HTTP 200. In Chrome, the rebuilt home page showed the added link; clicking it changed the visible page to `STRUCTURAL_LITERAL_7719`. CDP recorded only script requests for `/_app/immutable/nodes/10.1s576_ef.js` and `/_app/immutable/chunks/Ddb2yGAN.js`, with script initiators and the same document loader ID; there was no document request. This is Kit client navigation against the rebuilt client graph and Go's SSR.

For removal, the added root route was deleted, then the same `vp build`, `go generate ./...`, and Go restart sequence was repeated. `/aaa-structural` returned HTTP 404 and Kit's error boot document had `node_ids: [0,1]`; `/about` remained HTTP 200. The `/account/aaa-structural` route remained for this shift check. The removed fixture/build trees mean the noted account leaf/dictionary values are observations only; they cannot support an independently auditable identity claim.

The copy's example account routes redirect signed-out requests to `/`, so direct identity assertions for `/account/orders` are from the paired generated manifest/client metadata. The actual direct SSR and client navigation check used the unprotected root route.

## Remote membership boundary

This experiment did not add or remove a remote export. The current adapter performs two explicit membership guards during full build: `checkRemoteHashes` compares Kit's compiled remote module hashes with `skgo.remotes.json`; `checkRemoteIds` checks every remote ID the client bundle calls against IDs generated by `go generate`. A mismatch stops the build with actionable errors; the adapter documents that a new `.remote.ts` only reaches the build after the app imports it and says to run `go generate ./...`. These guards make the rebuild fail closed, but do not provide live remote table publication. A remote membership change needs Go generation and a new application build/restart before Kit's changed client is served as a coherent app.

## Reproduction

From an isolated copy of `example/` with `web/node_modules` linked to this worktree's already-installed dependencies and the Go module replacement pointed to the worktree root:

1. Add `/account/aaa-structural/+page.svelte`, then run `GOWORK=off ORIGIN=http://127.0.0.1:56521 vp build` from the copied `web/` directory.
2. Compare `web/build/skgo.manifest.json`'s `/account/orders.page.leaf` with `web/.svelte-kit/generated/build/client/app.js`'s `/account/orders` dictionary entry.
3. Add `/aaa-structural/+page.svelte` with the literal above and a home-page link, then run the same `vp build` command.
4. From copied `internal/skgo`, run `GOWORK=off go generate ./...`; start `GOWORK=off go run ./cmd -listen 127.0.0.1:56521` from the copied app root.
5. Fetch `/aaa-structural` from Go (HTTP 200, literal and node IDs); use a browser to click the home-page link and verify the URL and heading. The captured CDP record distinguishes script import/client navigation from a document request.
6. Remove only the root route, repeat build/generation/restart, then fetch the removed path and `/about` to check 404 and existing-route survival.

The earlier live-dev failure is preserved in `../20261015-vite-listener/structural.log`, `structural-client-metadata.js`, and `browser/structural-route-probe.json`: Kit adds `/listener-added` and shifts `/orders`, but the Go fixture still returns GO_ERROR 404 with stale node metadata; its browser reports failure loading generated `nodes/2.js`. It is exact evidence that component-level refresh is insufficient for structural membership.

## Timing and limits

The observed production build took about 3 seconds per run; `go generate ./...` took about 12 seconds in the full example copy; Go process startup was about 0.5 seconds. These are rough tool-wall intervals from this run, not a repeated benchmark. The hot body-edit path is separately measured in the retained qualification leaf and must not be conflated with structural fallback.

No `skgo dev` / Vite-dev structural update was tested with refreshed Go metadata. No live coordinator, route addition in the production source tree, or remote export mutation was tested. No browser navigation of a remote membership change was claimed.
