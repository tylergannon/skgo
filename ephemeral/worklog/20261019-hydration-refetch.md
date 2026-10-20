# Hydration refetch flake (/items/[id])

trap: Kit 3.1.0's client hands the boot payload's `q` entry to the first Query
constructed (the universal load at hydration) and keeps the cache entry only
while a QueryProxy is reachable (FinalizationRegistry in
`remote-functions/cache.svelte.js`). A load that awaits a query and returns
only the value leaves a later component call to GC timing: an extra request at
hydration is Kit's behaviour, not a skgo payload defect. Assert "no request at
hydration" only where page data retains the query (`?retain=1`).

discovery: the flake reproduces on demand with Chromium `--js-flags=--expose-gc`
and an init script calling `gc()` every task (20/20), and about 1 in 5 runs
under allocation pressure; CPU pressure alone never triggered it (0/150).
