# Mapped boundaries before the run

Authoritative Kit package: example/web/node_modules/@sveltejs/kit/package.json version3.0.0 (read directly).

- Kit dev `src/exports/vite/dev/index.js:102-119` recreates manifest data and syncs generated dev output; client graph changes originate in Kit/Vite.
- Kit client `src/runtime/client/client.js:2700-2720` implements refreshAll by re-running active remote functions/loads while preserving page.state. This alone cannot turn separately delivered frontend HMR and Go readiness into one transaction.
- Current adapter `internal/adapter/skgo-adapter/dev.js:311-318` lets normal HMR messages pass and intercepts full-reload; published handler sends custom skgo:go-ready and, for structural publication, full-reload. Snapshot uses current Vite/Kit-owned source and config, rather than a captured Go filesystem lease.
- Current Go `internal/gen/retained.go:567-594` holds Project mutex across current-snapshot check, optional native admission and generated-output publication. Observe cannot become admitted while that callback holds the mutex.
- Current coordinator `internal/devserver/session.go:397-419` completes Commit, then updates goodInputs/status and calls front.notify outside the Project mutex. Experiment barrier before that call can distinguish installation currentness from later notification currentness.

Owned overlay preserves those algorithms. It introduces named blocking hooks at captured, prepared, and before-notify, plus a control endpoint that reports current observed Project revision. Assertions will compare independent supplied literals and frame revisions, not infer expected values from a current response.
