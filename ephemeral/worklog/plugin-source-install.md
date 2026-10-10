# Source plugin installation

request: Source install for the executing standalone or project tool host, tracked by skgo298 and the central plugin-source-install contract. Archive/prebuilt installation remains outside this increment.
discovery: Recorder preparation needs its nested Go module, which a root module zip omits. Use canonical direct Git resolution plus hash verification, then the existing stage builder with pre/post-generation tidy and final host pin checks.
decision: Per-host publication lock covers peer qualification and replacement. Private child discovery prevents the prior managed binary contaminating candidate qualification. Manual directories remain additive to current managed storage.
friction: Isolating HOME for real private-repository qualification also hides macOS gh keyring access. Keep plugin storage isolated but let the test's Git credential helper use the existing caller HOME and Go policy; never copy credentials into receipts or source cache.
friction: Full tests in a fresh worktree need `just install` and `just build` first. Staticcheck0.8.1 cannot parse Go1.27.2 export data here; the same targeted check succeeds with the repository's declared Go1.27.1 toolchain. Use that qualified toolchain for this run instead of altering unrelated analyzers.
correction: A private qualification set must canonicalize paths too: macOS candidate paths under /var resolve to /private/var. Matching a canonical candidate against raw Entry paths incorrectly rejected a successfully loaded recorder.

- Parse subprocess stdout independently from stderr: a first-use Go toolchain download announces itself on stderr and corrupted the GOROOT path when CombinedOutput was parsed. Preserve resolver JSON on nonzero exits for its actual module diagnostic.
- Materialize the pinned Git commit with archive rather than checkout filters; skip its global PAX commit metadata while rejecting source symlinks. A real local Git fixture exposed that archive header.
- Failed/removed installations can leave lock-only host directories. Discovery hints must count actual retained .so files, not those empty directories.
- correction: A digest-named managed file is not enough to recover from a conflict. Show its recorded module/version and concrete remove command during discovery and qualification failures, with exact-file fallback for missing metadata.
- correction: Other-host installation hints are advisory. An unreadable retained sibling must warn with its path while current-host help/add discovery remains usable; metadata diagnostics must not become a second discovery gate.

correction: The prior native UI failure did not establish an unlock requirement. Configured CUA launched the retained TemplateFieldNotes app and actual controls produced Recording, Paused, resumed Recording and Stopped with retained synthetic text. Quit/reopen used a new backend and final quit removed the process/listener. Its retained binary still belongs to the earlier v0.28.0-generated consumer; current and released observations remain separately attributed in Mission Control.

decision: Merge current main 2edb21c (v0.28.3) before installer delivery, retaining the scaffold bootstrap and updater/watcher fixes. Integration a6b69f5 is conflict-free; installer/builder/store code and its independently mutated assertions remain unchanged. Independent Go1.27.1 build/test/vet qualifies the integrated candidate.
