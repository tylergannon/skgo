# Dev child exit status

decision: an owned Vite process ending before caller cancellation is a failed dev session even when it exits with status 0. Preserve the child result before cancelling `server.Run`'s context; only exits caused by the later cleanup path remain clean.

friction: an empty `select {}` in a Go fake child triggers the runtime deadlock detector instead of waiting for supervisor cleanup. Use a long sleep loop for a fixture that models a live child.

friction: direct `go test` bypasses the Justfile's pinned Staticcheck PATH. The canonical `just test` ran `cmd/skgo` successfully but its generator tests rewrote checked-in example bindings before failing on stale generated aliases and missing frontend build assets; restore incidental `example/` output when testing this scoped CLI change.
