finding: `node_modules/.bin/vp` is a /bin/sh shim that `exec`s node on vite-plus's CLI, and that CLI's native `run` binding spawns Vite (`vite-plus-core/dist/vite/node/cli.js dev`) as a second node process in the same process group. SIGKILL on the started pid kills the CLI and reparents Vite to pid 1, still listening. Closes the `open:` in 202609102100-scaffold-module-cache-race.md.

decision: `startProcess` sets `Setpgid` and stops with SIGKILL to the negative pid. Checked live: the Vite child's pgid equals the `vp` pid, so the group kill reaches it.

trap: `setpgid(0,0)` from a Bash-tool command fails with EPERM because the exec'd shell is a session leader. Experiment on process groups from a Go test or a forked child, not an exec wrapper.

open: t.Cleanup does not run when `go test -timeout` panics, so a timed-out scaffold run still leaks its whole group. Darwin has no Pdeathsig.
