# Source plugin installation

request: Source install for the executing standalone or project tool host, tracked by skgo298 and the central plugin-source-install contract. Archive/prebuilt installation remains outside this increment.
discovery: Recorder preparation needs its nested Go module, which a root module zip omits. Use canonical direct Git resolution plus hash verification, then the existing stage builder with pre/post-generation tidy and final host pin checks.
decision: Per-host publication lock covers peer qualification and replacement. Private child discovery prevents the prior managed binary contaminating candidate qualification. Manual directories remain additive to current managed storage.
friction: Isolating HOME for real private-repository qualification also hides macOS gh keyring access. Keep plugin storage isolated but let the test's Git credential helper use the existing caller HOME and Go policy; never copy credentials into receipts or source cache.
friction: Full tests in a fresh worktree need `just install` and `just build` first. Staticcheck0.8.1 cannot parse Go1.27.2 export data here; the same targeted check succeeds with the repository's declared Go1.27.1 toolchain. Use that qualified toolchain for this run instead of altering unrelated analyzers.
correction: A private qualification set must canonicalize paths too: macOS candidate paths under /var resolve to /private/var. Matching a canonical candidate against raw Entry paths incorrectly rejected a successfully loaded recorder.
