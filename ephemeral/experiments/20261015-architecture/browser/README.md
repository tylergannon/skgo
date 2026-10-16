# Browser observations, 2026-10-15

The parent independently operated the cooperation fixture through the Codex in-app browser, inspected screenshots, and saved DOM/error observations. This is the real Go SSR plus Vite browser experiment, not a native-patch timing run.

| Action | Observed positive result | State outcome |
|---|---|---|
| Initial Go-edge visit | Go SSR heading and literal 3000; hydration false | Browser dependencies returned 504 |
| Isolated Vite cache, fresh visit | Heading, 3000, Hydrated true; no browser errors | Usable client |
| Seed ARCH_DRAFT_KEEP_73 and count 3; click Go value 2400 | Visible 2400, reported direct-control + refreshAll + tick 15.80 ms | Draft and count preserved |
| Click Edit Svelte | FRONTEND_ONE became FRONTEND_TWO, Go 2400 remained | Draft cleared and count reset to 0 under ordinary Svelte HMR |
| Seed VITE_EVENT_KEEP_91 and count 2; change Go fixture value to 1800; notify Vite | Visible 1800 through Vite custom event and Kit refresh | Draft and count preserved |
| Break Go load | Visible GO_LOAD_ERROR 500 | Component replaced by error boundary |
| Repair Go fixture value to 900; notify Vite | FRONTEND_TWO and 900, Hydrated true | Recovered with empty draft and count 0 |
| Visit Vite edge; seed VITE_EDGE_KEEP_52 and count 1; set Go value 1200 and notify Vite | Visible 1200, Hydrated true, no errors | Draft and count preserved |

Go-edge origin was http://127.0.0.1:56313; Vite-edge origin was http://127.0.0.1:56311. The independent backend controls were GET /__control/value?value=N on the Go edge followed by GET /__cooperation/notify on Vite. Those calls changed the supplied fixture value and sent the real Vite hot-channel event; they did not compile native code. The 15.80 ms number times the button's direct control/refresh operation, not the separate event route or save-to-visible performance.

The first dependency failure is preserved in initial-hydration-failure.json and reload-network.json. Vite's optimized devalue/esm-env URLs returned empty HTTP 504 responses. A symlinked node_modules directory shared its default optimizer cache across experimental configurations. The corrected fixture isolated cacheDir. A server restart also left the first browser tab on a connection-refused error page; a fresh tab on the same browser reached the live fixture. No browser security protections were bypassed.

Each named JSON observation contains actual DOM and, where relevant, console output. The screenshots were inspected. browser-tool-records.jsonl preserves the structured browser tool call/result records selected from this chat's rollout file, including failed interactions; it is a JSON reserialization, not reconstructed prose or a byte-identical source-line extraction. Source: /Users/tyler/.codex/sessions/2026/10/15/rollout-2026-10-15T12-52-44-01a140e8-d934-7be3-9b32-c790cd31675d.jsonl. The selection is all response_item records paired by call_id with function_call name js through this experiment's final browser observation.

See ../cooperation/README.md for exact source, routing/metadata limitations, HTTP samples and process cleanup. The browser evidence does not prove automatic Go watching, native patch publication, route additions, reconnect, or total save-to-visible timing. Component/document identity was not instrumented in this fixture; observed draft/counter continuity must not be promoted into such a claim.
