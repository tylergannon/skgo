# SSR-in-process spike

Not production code. It answers one question: can a JS engine embedded in the
Go binary server-render the example app, with every remote function answered by
Go in-process and no Node at request time?

    just install
    just build                                   # kit's own SSR build; needs ssr = true
    cd ephemeral/spike-ssr
    node build.mjs                               # AsyncLocalStorage shim
    node build.mjs --als=webcontainer --out=dist/ssr.webcontainer.js
    node build.mjs --als=none        --out=dist/ssr.none.js
    cd ../.. && go test ./internal/ssrspike/ -v

`node oracle.mjs` runs the identical bundle under Node and prints the same
scenarios, which is what the Go test diffs against.

`build.mjs` reads kit's own build output for the node table and the route table,
compiles the app's real `.svelte` files with the app's own Svelte compiler, and
bundles kit's real `Root`, `Props`, `RenderNode` and remote-function wrappers.
The only substitution is `src/app-server.js`: the `$app/server` the bundle sees
keeps kit's `query()` and replaces the user function body with a call to Go.

Findings live in ephemeral/worklog/202609061810-ssr-in-process-spike.md.
