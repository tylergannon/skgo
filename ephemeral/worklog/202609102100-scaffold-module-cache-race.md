finding: Two concurrent runs of the old scaffold test almost always pass even though the second run deletes the first run's extracted skgo from the module cache. Deleting a module version between go commands heals itself: the next go command downloads it again from the run's file proxy. The run fails only when the deletion lands inside a go command that has already resolved the directory, which is why #112 looked like a flake.

trap: An exit-code repro of this race proves nothing. Watch the inode of the extracted `skgo@<version>` directory while the first run is alive. On main it vanished 0.6s after the second run started and 11.6s before the first run exited. With the fix, it vanishes only in the last ~0.1s, and that is the run's own t.Cleanup.

decision: publish mints a version per call and registers exact removal of that version (extracted tree plus .info/.mod/.zip/.ziphash/.lock/.partial). The go command's `list` file is left to the go command, which rebuilds it under its own lock. A run killed by -timeout or ^C leaks one copy of the checkout; `go clean -modcache` reclaims it. A sweep of stale versions would bring back sibling deletion.

open: `serveVite` still leaks node `vp dev` processes (ppid 1, hours old) after scaffold runs. Killing the `vp` shim does not kill its node child.
