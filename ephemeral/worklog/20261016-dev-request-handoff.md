# Development request handoff

decision: Keep full replacement compilation before retirement and wait for old process exit before the new constructor. Pause first public dispatch during this interval; never replay a request whose application effects may already have happened.

correction: A drain at the public proxy must keep private Kit/Go continuation traffic available. Go internal fetches enter Kit over its private socket, so gating every private begin/callback can deadlock a draining request behind its own descendant.

correction: HEAD and 204/205/304 bridge responses previously launched logical request cleanup without awaiting it. Public response completion could therefore precede Go middleware completion; these responses now await cleanup like ordinary response bodies.

friction: Fresh vp install and Go generation do not create node_modules/$app/types/index.d.ts. Run the pinned Kit sync before development adapter/session tests that verify shared-type isolation.

correction: Vite prepends the configured development base to its WebSocket path (installed Vite node.js, hmrBase). HMR bypass must recognize the base-prefixed tooling path as well as the root path, and still route that tooling connection to Vite.

correction: Independent review reproduced another completion boundary: a body with explicit Content-Length can finish at the public Go transport before its final Node stream read awaits Go cleanup. The final bridge response now lets Kit frame body responses by actual completion; HEAD metadata and internal-fetch readable headers remain intact. A test using the installed Kit setResponse failed before this repair and passes after it.

friction: A fresh worktree also needs the ordinary example build before the full just test recipe, because its example/native and embedded-asset tests require real compiled frontend outputs. Freeze adapter bytes during real-session tests: their compiled identity must match the linked adapter throughout subsequent fixture startups.

correction: Real consumer startup exposed the same unavailable-backend invariant before the first publication. Vite readiness alone cannot open public admission. Run now starts paused and keeps that pause across obsolete initial-build retries until an initial Go host and its routing are published.

friction: A cold-start test source edit can be observed just after first publication, scheduling a follow-up rebuild. Keep its external readiness latch released until the expected edited endpoint is serving; clearing it earlier accidentally holds an unrelated constructor and tests the fixture's timeout instead of request admission.
