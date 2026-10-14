# Native Go reload POC

A separate experimental development host for evaluating a capability eventually
shared by SKGO and Gimbal. It compiles one compatible Go source file, replaces
its live HTTP behavior without relinking/restarting the host, and retains a
host-owned counter. It is **not integrated into either application**.

Requires this experiment's private Go 1.27.2 GOROOT on darwin/arm64 and adjacent
pkujhd/goloader checkout at `2695cfc5350631cc3becc1409162b12a885471b5`.
See [experiment setup](../experiment-notes.md#exact-paths-and-environment).
Upstream needs cgo and private compiler/runtime internals. No installed
Go toolchain modifications; no change to SKGO's pure-Go production requirement.

From this directory, with the private GOROOT:

```sh
export GOROOT=/Users/tyler/.codex/worktrees/skgo-generation-costs/skgo/ephemeral/reload-spike/goroot
export GOWORK=off
go test -ldflags=-s=false -count=1 -v ./...
go build -o ../native-reload-poc .
../native-reload-poc -source handler.go.txt -addr 127.0.0.1:8097
```

The test linker flag retains symbols that upstream's runtime loader requires;
plain `go test` strips them on this toolchain and cannot exercise native loading.
The tests also modify and restore **private cloned** `strconv/number.go` to
prove dependency-body rejection; do not run multiple POC test suites against
this same private GOROOT concurrently.

Open `http://127.0.0.1:8097/proof?n=7`. Edit `"original"` or `n * 3` in
`handler.go.txt`; the next successful compilation changes the response while
`X-Host-PID` and the counter persist. A compiler error leaves the prior revision
serving. The command prints an explicit restart requirement for unsupported
changes. It does not perform a restart itself.

The admitted lane is deliberately closed: the fixed imports/signatures and
approved synchronous operations in `compatible`. There are no source globals,
initializers, new types, goroutines, deferred work, escaping function values,
or writes through host pointers. This is not arbitrary Go package hot reload.
The `pause` argument exists to exercise a request blocked inside loaded code.

Each accepted edit compiles fresh; there is no revision cache or expiry.
Dependency source contents/membership, archives, compiler binaries and build
environment are fenced. Dependency/configuration edits require restart.
Function-body source reverts are ordinary new compilations. Initial build and
launch must use the same private inputs: this POC does not prove that an
arbitrary prebuilt host matches newly discovered dependency archives.

Revision lifetime is independent: concurrent readers retain the revision until
its returned string has been copied, replacement waits for all active calls,
and old code/data mappings are unloaded. At most two native modules exist
during a replacement. A permanently blocked handler blocks its replacement;
this POC does not interrupt arbitrary native Go execution. Unexpected loader
failure terminates the experimental command because upstream does not promise
rollback of partially registered runtime state.

Tests exercise real HTTP, exact changed results, held requests, compile
failure/recovery, source reverts, dependency/configuration rejection, 60 reloads
under concurrent traffic/forced GC, and Mach VM observations that each retired
code/data mapping disappears. The watched-file test builds and starts the
actual command once and edits its source externally.

Observed tiny-fixture edit-to-HTTP timing was 19–24 ms before the conservative
input fence. Complete fenced watched-file runs observed 254–451 ms; hashing
inputs twice dominates. These are observations, not percentiles or full SKGO /
Gimbal latency claims. An immutable build-bound session snapshot with explicit
restart events could remove repeated hashing; it is not implemented here.
