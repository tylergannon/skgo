# Structural rebuild recovery

correction: A parse-error recovery test never reached the complete-build path. Structural type errors must retain a live, mutated application state and actual serving PID; eventual healthy responses alone allow a crash/restart to pass.

decision: Give each application candidate its own private Unix socket and publish the stable Vite socket alias only after readiness and revision admission. Closing or rejecting a candidate must remove only its owned socket, never the healthy application's listener.

friction: The real-session fixture watches its replacement SKGO root. Editing runtime Go source during a live test triggers unrelated rebuilds and invalidates the comparison. Keep source fixed during the test and use a Go overlay for an independent pre-fix negative run.

friction: Adding deferred sync.Mutex.Unlock to the integration fixture's hot-replaced handler caused `symName:sync.(*Mutex).Unlock relocateType:R_ADDROFF, symbol is not exist!` before the structural-edit check. The fixture sends requests sequentially and has no background state access, so it does not need that mutex. This is an observed native-replacement limitation, not repaired or qualified by the socket lifecycle change.

limitation: The mutex observation came from changing only the literal response of an already-running handler containing `mu.Lock(); defer mu.Unlock()` with package-global `sync.Mutex`. The clean pre-fix run (`ephemeral/structural-recovery-red-clean.log`) emitted raw relocation stderr, kept returning HTTP 200, and neither published the edited literal nor produced a normal revision/Vite error notification or rebuild/recovery before the 75-second assertion expired. Automatic rebuild fallback was not observed. Subsequent source-edit or restart recovery was not exercised; a stalled loader call or event loop remains a hypothesis. Issue #330 tracks this boundary. Preservation of arbitrary in-flight state is not established by that log.

correction: The review's suggested type-invalid helper in a route package is caught by generated-link type checking before host rebuild, so it does not demonstrate the lifecycle defect. The distinguishing fixture must change the application constructor package, which generation does not import, while a real route keeps serving its already-mutated state.
