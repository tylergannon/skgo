# SKGO is a lightweight server

Proposed local promise for `tylergannon/skgo-project`; not applied there or published.

Lightweight must describe the server a developer actually runs, including its renderer and fetch path. A Go-native endpoint being cheap does not establish that a page is cheap. No idle CPU loop, no retained request trees, reusable isolated runtimes, and measured CPU/allocations per request belong to the promise.

The example on this 10-core darwin/arm64 machine, Go 1.27.1, Kit 3.0.0-next.28, Svelte 5.57.1, pinned Goja fabc3b8078ad, exposed an adapter bug: a comment falsely triggered ES2017 lowering, replacing Svelte's native private fields with WeakMaps. Correcting that lowers measured warm home CPU from 4.176 to 2.027 ms/request and retained heap after 2,000 requests and three collections from 270.11 to 6.65 MB. Nested universal fetch drops from 5.033 to 2.281 ms/request and retained heap after 1,000 requests from 194.64 to 7.96 MB. These are handler measurements including recorder/request/fixture costs. They are not maximum network throughput or a universal application guarantee. FileProvider contention contaminates wall/p95 comparisons; process CPU is measured with OS getrusage and corroborated by time user+system.

Next measurement should use a quiet machine and fixed offered load against the actual binary over HTTP. Separate cold startup, idle, warmed pages, endpoint/static/data requests, nested page fetch, and cold growth of another worker. Record OS process CPU, bytes allocated, live heap after collection, runtime creation count, p95, errors, and literal visitor fixtures. Compare identical compiled component/renderer work under pinned Goja and Node/V8 to identify intrinsic engine cost, without changing app I/O or using Node in the delivered server. Keep build/toolchain CPU separate, and measure child processes when ranking generator tests.

The current result supports retaining fetch and repairing demonstrated waste. It does not yet establish Goja's throughput ceiling or an acceptable product budget. Agree on deployment hardware and required request rate before assigning a numeric performance promise.
