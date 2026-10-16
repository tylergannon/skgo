# Actual skgo dev structural browser check

Command was `/tmp/skgo-dev-first dev --root example --listen 127.0.0.1:56811` built from this working tree. Coordinator owner controlled its process. Parent inspected Chrome with the supported browser tooling; observations below are browser observations, not assertions inferred from source.

Before structural changes, initial real Item42 hydration was clean after matching Svelte HMR compiler settings. A malformed item Go query displayed authored file/line and kept Widget42 plus seeded `healthy draft 7719`. Exact repair cleared the Vite overlay automatically, preserving browser timeOrigin1792099991223.4 and draft. Editing the page title to `HMR_8426 Item 42` changed the open page and fresh Go SSR while GoPID49562, document and draft remained. Original page restored.

Added `/aaa-dev-probe` with actual Go `readProbe` query returning literal7719 and a Svelte boundary rendering it. Added a temporary link on the existing item page. Direct Go response was200 with the new route and remote payload. Actual Kit client navigation showed heading STRUCTURAL_7719 and paragraph REMOTE_7719. Following its link back to `/items/42` showed Item42 and Widget42 twice. Both sides of that navigation had timeOrigin1792100115266.6. The structural transition had naturally reloaded earlier; no draft-preservation claim is made across route membership changes.

After removing the route and remote together and restoring the item page, direct `/aaa-dev-probe` returned404. The open existing item page rendered Item42 and Widget42 twice, with no temporary link. All three touched existing authored files (item.remote.go, item +page.svelte, about +page.svelte) were byte-compared to saved originals and restored. The newly owned route directory was removed.

Exact generated artifacts are retained here, avoiding the earlier mistake of deleting paired manifests. Go generations1/2 exclude route and remote;3/4 include both;5 excludes route but still contains remote;6 excludes both. `added-client-app.js` contains the route and `removed-client-app.js` does not. Added/removed generated Go Params are retained. No equality between Go manifest positions and Kit client node indexes is inferred.

Two defects were observed and assigned rather than omitted:
- During initial add, revisions8/9 refused host initialization because fresh frontend metadata had no matching generated Params constructor; revision10 repaired automatically. The manifest guard prevented mixed host publication. Coordinator owner subsequently changed admission to consume fresh snapshot metadata before replacement and must recheck that fix.
- The existing About page returned404 on navigation and500 in direct SSR because its prerender remote was treated as requiring production recorded output. This is separate from new-route success; Go and adapter owners are implementing Kit's actual development semantics.

`product.log` retains the raw session output, including those failures. `removed-response.html` is the observed404 document. `authored-route/` contains the temporary fixture and generated throwing remote stub. These are bounded product observations, not a completed all-feature qualification or save-to-visible benchmark.
