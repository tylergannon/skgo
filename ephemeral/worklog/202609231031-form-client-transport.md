friction: macOS rejects Unix socket paths under long Go test temp directories -> use a short socket path under /tmp for UDS integration tests.
decision: Go http.Client follows 307/308 and replays a POST with a rewindable body -> Form transport must disable redirects on a per-call client copy so a mutation is never submitted twice and the caller's shared client stays unchanged.
