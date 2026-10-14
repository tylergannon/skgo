# Native Go reload experiment — 2026-10-14

## Result and evidence boundary

Native compiled Go code was loaded and swapped into a running HTTP server on
this Mac without relinking or restarting the host. Three source revisions
produced three independently expected HTTP responses from the same PID.

This is an isolated experiment, **not a SKGO or Gimbal integration**, and not
proof that arbitrary application or type changes can be hot-swapped safely.
The initial spike is now extended by the repeatable POC described below; the original three-revision observations are retained as historical measurements.

The user explicitly requested aggressive reload techniques, then requested that
these notes be saved and an Astra subagent own the proof of concept. Parent
continues with the larger generation-latency problems.

## Exact paths and environment

- Research worktree: `/Users/tyler/.codex/worktrees/skgo-generation-costs/skgo`
- Experiment directory: its `ephemeral/reload-spike/`
- Upstream checkout: `ephemeral/reload-spike/goloader/`
- Upstream: <https://github.com/pkujhd/goloader>
- Upstream revision: `2695cfc5350631cc3becc1409162b12a885471b5`
- Host code: `goloader/examples/hotproof/main.go` (new, untracked in that nested checkout)
- Host binary: `hotproof`
- Build log: `build.log`
- Initial simple-function run: `run.log`; generated source/objects in `generated/`
- Stronger imported-types/generic-function/state run: `run-http.log`; inputs in `generated-http/`
- Toolchain: Go 1.27.2, darwin/arm64.
- Installed GOROOT: `/opt/homebrew/Cellar/go/1.27.2/libexec`
- **Private** experimental GOROOT: `ephemeral/reload-spike/goroot`

The installed toolchain was not modified. The private GOROOT contains an APFS
clone (`cp -cR`) of the installed `src/`, with `src/cmd/internal` additionally
copied to `src/cmd/objfile`, as required by this library. Its `bin`, `pkg`, `lib`,
and `VERSION` are symlinks to the installed toolchain. Do not run upstream's
copy command against the installed GOROOT. Do not commit the copied toolchain,
binary, or nested `.git` directory.

## Commands that ran

From `ephemeral/reload-spike/goloader/`:

```sh
GOROOT=/Users/tyler/.codex/worktrees/skgo-generation-costs/skgo/ephemeral/reload-spike/goroot GOWORK=off go build -o ../hotproof ./examples/hotproof > ../build.log 2>&1
```

From `ephemeral/reload-spike/`:

```sh
GOROOT=/Users/tyler/.codex/worktrees/skgo-generation-costs/skgo/ephemeral/reload-spike/goroot GOWORK=off ./hotproof ./generated-http > run-http.log 2>&1
```

Both completed successfully. The initial build used `map[string]any`, which
failed because upstream's go.mod declares Go 1.11. Host code now uses
`map[string]interface{}`. Do not mistake that repaired harness compilation
error for a runtime-loader incompatibility.

## How the stronger experiment works

The host starts one HTTP server on a loopback ephemeral port. An `atomic.Value`
holds a function of type `func(*http.Request, *atomic.Int64) string`. The counter
lives in the host and persists across replacements.

At startup, the host discovers standard-library export archives **once**, using
`go list -export -deps` for net/http, fmt, and sync/atomic, and writes importcfg.
It registers host runtime symbols and relevant types with goloader.

For each of three revisions it:

1. Writes a changed Go source file.
2. Runs only `go tool compile -p hotcode -lang=go1.27 -importcfg ... -o ... source.go`.
3. Reads that object with `goloader.ReadObjs`, then loads it with `goloader.Load`.
4. Resolves `hotcode.Value`, converts its address to the known function signature
   using upstream's unsafe function-pointer technique, and atomically swaps it.
5. Makes a real HTTP request and asserts the exact literal response.

The loaded code imports net/http, fmt, sync/atomic and calls a generic identity
function marked `//go:noinline`, so this is more than changing a constant in
host code. The successive responses must be:

```text
version-one:1:/proof
version-two:2:/proof
version-three:3:/proof
```

The counter sequence proves host state survives. The output logs record one PID
for all three responses. Timing starts before writing the changed source and
ends after receiving and checking the HTTP response. Startup/export discovery
and the original host build are outside the hot-edit interval.

The main goroutine is locked to an OS thread during loading because Apple's
JIT write protection is per native thread. Old code modules are retained until
the experiment finishes; server closes before unloading. **Safe draining and
bounded retention during continuous reload are not yet proved.**

## Measurements

Initial tiny `func Value() string` experiment, PID 58155:

| Revision | Compile including source write | Load/swap | Edit to HTTP response |
|---|---:|---:|---:|
| one | 13.764 ms | 0.249 ms | 14.589 ms |
| two | 12.321 ms | 0.232 ms | 12.701 ms |
| three | 12.313 ms | 0.395 ms | 12.890 ms |

Stronger HTTP-request/imports/generic/state experiment, PID 61034:

| Revision | Compile including source write | Load/swap | Edit to HTTP response |
|---|---:|---:|---:|
| one | 17.435 ms | 0.386 ms | 18.441 ms |
| two | 25.880 ms | 0.703 ms | 27.111 ms |
| three | 18.817 ms | 0.397 ms | 19.427 ms |

These are three observations on a deliberately small fixture, not percentiles,
not a benchmark of the whole application, and not a general latency promise.

## Material limits to exercise in the proof of concept

- A compile/type error must leave the last working implementation active; a
  later repair must load successfully.
- Exercise requests during replacement. Do not unload code or data while a
  request, goroutine, closure, or returned object can still refer to it.
- Retain only a bounded set of drained revisions, and verify repeated reloads
  plus forced GC. A three-revision process that exits cannot prove this.
- Detect/reject incompatible function/type/dependency changes and fall back to
  the ordinary rebuild/restart path. Matching symbol names are not proof of
  compatible layouts or runtime type identity.
- Host globals/state were deliberately kept outside the loaded module. Changed
  package initialization and long-lived module goroutines need an explicit
  boundary; they are not covered by this experiment.
- The loader is tightly coupled to Go compiler/runtime internals and needs the
  private toolchain source adaptation. This is not a supported standard Go
  compiler/linker embedding API.
- On darwin/arm64, upstream `mmap/mmap_darwin_arm64.go` uses **cgo** for
  `pthread_jit_write_protect_np` and instruction-cache invalidation. This is an
  isolated development-host experiment, not permission to add cgo to SKGO's
  production runtime or replace its pure-Go goja engine.
- Preserve a normal build/restart path. Do not silently apply unsafe swaps to
  arbitrary application changes just because this narrow fixture passes.

## Why this experiment is relevant

Gimbal owner measured a real workflow function-body edit after warming the Go
compiler cache, with no global cache purge:

- Ordinary backend build: 3.178 s total, **2.870 s linking**.
- Changed service-check package compilation: 26 ms.
- Changed invocation package compilation: 34 ms.
- `-ldflags=-compressdwarf=false`: 2.884 s total, 2.631 s link; preserves debug
  info but only saves about 0.29 s. Debug binary was 121,933,394 bytes.
- These are build-only measurements, not browser-ready timing.

Thus avoiding full relinking addresses a measured bottleneck. The runtime
experiment does not remove code generation costs; that is a parallel task.

## Other latency findings and active ownership

Parent `/root` continues coordinating original shared invocation-session work,
persistent caches, code-generation latency, and eventual landing. Do not merge
or release the runtime experiment as part of this POC assignment.

### SKGO generation and reusable development supervisor

Owner `/root/polytype_reuse` (name is historical), worktree
`/Users/tyler/.codex/worktrees/skgo-analysis-cache`, draft PR310.

- A real changed-body example generation took 6.852 s, zero session fallbacks.
- Preflight/Kit/capture/private view: 0.465 s.
- Locals/matcher/hooks/batched discovery: 1.111 s.
- Serial schema projections/codecs: about 4.86 s.
- Frontend formatting: 0.230 s; publish/cache cleanup about 0.18 s.
- Profile: `/tmp/skgo-session-profile-changed.jsonl` and
  `/tmp/skgo-session-changed.cpu`.
- Main-process CPU: 2.90 s, including temporary overlay writes 1.06 s and
  cleanup 0.69 s. Every metadata request rewrites the same captured files.
- 96 Go child invocations: 30 package/dependency/export discovery (2.990 s
  aggregate wall); 30 release-tag probes (0.194 s); 29 architecture/compiler
  probes (0.208 s); 4 environment probes (0.020 s); 2 directory lookups
  (0.048 s); 1 module discovery (0.007 s).
- Existing `skgo dev` already owns Vite and Go generate/build/restart, but its
  generation callback launches a fresh process on every edit. It uses a 150 ms
  poll and 100 ms quiet period. A public reusable supervisor seam is in progress.
- SKGO generation reads declarations/signatures/marker initializers rather than
  authored function bodies, but generated route-link source includes bodies.
  Body-only reuse therefore still needs fresh link bytes and normal Go build
  validation; blindly ignoring body changes in output hashes would be wrong.

### Polytype shared and warm analysis

Owner `/root/consumer_proof`, worktree
`/Users/tyler/.codex/worktrees/polytype-shared-load-session`, draft PR167:
<https://github.com/tylergannon/polytype/pull/167>.

- Shared `goload.Session` is independent of grammar and generators; zero value,
  lazy, invocation-scoped. Explicit fixture groups can share it in unit tests.
- Persistent generator caches remain first; a hit leaves Session.Stats empty.
- Captured source bytes reach go/parser directly, no duplicate in-process read.
- Compatible roots/dependencies share coherent go/types objects. Different
  source/build-tag/overlay views remain isolated. Imported-export to source
  promotion isolates a new universe instead of mutating existing types.
- Current session still reruns metadata and retains universes; simply keeping
  it forever is not the warm-worker solution. Investigating reusable native
  overlay lifetime, metadata reuse, dependency-aware invalidation, bounded
  coherent-graph retention. LRU eviction is separate from correctness.
- `gopls` is useful precedent, not a drop-in public AST service. Its cache is
  internal and its TypeCheck API does not promise global cross-package pointer
  identity. It carefully manages FileSets/token.Pos ranges for parse reuse.
- Target: ordinary generation edits should not rerun the 96-command discovery
  sequence. Initial loading/import/membership/tag/module/toolchain changes may
  still require Go discovery. No daemon-wide global mutable analysis state.

### Gimbal integration and preview

Owner `/root/skgo_reuse` (historical name), worktree
`/Users/tyler/src/gimbal-worktrees/gimbal-ir-cache`, draft PR528.

- Existing `just dev` uses `web/dev/run.mjs`, does generate/build once and keeps
  Vite/backend alive; it does not watch/rebuild the backend.
- Separate Vite workflow preview used a 250 ms debounce and regenerated all
  eight selected workflows after one edit, sequentially, then rediscovered
  watch membership. Actual changed batch 1.377 s, of which seven unaffected
  workflows cost about 1.08 s; rediscovery 0.123 s, plus debounce.
- One warm workflow process 149 ms: input/export capture 125 ms, IR read 0.57 ms.
  Changed-body workflow process 269 ms: capture125 ms + typedload118 ms + rest.
- Affected-workflow targeting and Go batch analysis are now locally implemented.
  Actual Vite regression proves edited/broken/recovered workflow updates while
  independent sibling stays ready at revision1. Go batch proof uses one typed
  load for two roots and preserves independent broken-root diagnostics.
- Gimbal is to consume SKGO's reusable supervisor with generation/build/launch
  hooks, rather than maintain a separate backend lifecycle implementation.

### Regressions caught; do not repeat the misleading measurement

Two full Gimbal shared-loader runs became pathological (about43–44 s; first
reached14 GB RSS) and were stopped. Cause: unsupported session load fell back to
ordinary packages.Load with a large Overlay; it source-parsed dependencies.
One stack was deep Polytype decoration, but that was a consequence, not the
root cause.

Fixed with per-request caller fallback (`Session.LoadWithFallback`, propagated
through lazy grammar dependency loads) plus removing unchanged captured bytes
already present in SKGO's owned immutable private source tree from the fallback
overlay. Generated phase overrides remain. Small regression went from1,093
parsed files to exactly2 roots, retaining new virtual source/dependency meaning.

Latest full Gimbal correctness run completed with391 observed files exactly
matching nil-session output. Stats:27metadata/31checks/144parses/8hits/6fallbacks;
the8hits were within SKGO. Its10.65s timing overlapped other work and is **not**
a controlled performance comparison. Matched final timings remain pending.

## Primary sources considered

- Go's incremental analysis/cache explanation:
  <https://go.dev/blog/gopls-scalability>
- Gopls implementation:
  <https://go.dev/gopls/design/implementation>
- Actual native loader used:
  <https://github.com/pkujhd/goloader/tree/2695cfc5350631cc3becc1409162b12a885471b5>
- `got-reload` uses Yaegi and documents no generic-function support; not tested:
  <https://github.com/got-reload/got-reload>
- `hotswap` uses Go plugins, still builds plugins, and cannot unload old code;
  not tested: <https://github.com/edwingeng/hotswap>
- Standard plugin limitations: <https://pkg.go.dev/plugin>
- Linker options: <https://go.dev/cmd/link/>

## Handoff scope for Astra

Own only the native-reload POC within this experiment directory (or an isolated
task worktree if needed). Read these notes and the actual code, retain the
proof limits, and choose the implementation. Demonstrate useful live handler
replacement with retained state, failed-edit recovery, concurrency/GC/lifetime
handling, and a safe boundary for incompatible changes. Report exact observed
latencies separately from unproved SKGO/Gimbal integration. Coordinate any
heavy timing window with the other agents. The parent owns production codegen
and supervisor integration; do not modify their worktrees or revert their edits.


## Completed POC extension — 2026-10-14

Source: `ephemeral/reload-spike/poc/`, with its own module referencing the pinned
adjacent loader. Run instructions and the exact accepted lane are in
`poc/README.md`. The actual command watches a source file and serves HTTP;
Go tests exercise both the handler and that independently launched command.
No SKGO/Gimbal production packages were modified.

`poc-final.log` records the complete passing suite (22.360 s). Its actual
watched command stayed at PID8310 across source edit, compiler failure, repair,
source revert, and incompatible signature rejection. Exact HTTP responses were
`original:1:21:/proof`, `watched:2:35:/proof`, `watched:3:35:/proof`,
`repaired:4:49:/proof`, `original:5:21:/proof`, `original:6:21:/proof`.

The lifecycle test waits until a candidate has loaded and reached the drain,
while an existing HTTP request remains blocked inside the old native function.
That old call returns the old result; the next request returns the new result.
Host-owned state survives both successful and rejected edits. Returned strings
are cloned before releasing the revision; a retained literal is checked after
unload and forced GC.

The repeated-reload test compiles 60 changed implementations, checks 60 exact
HTTP results, runs concurrent HTTP requests and forced GC, and bounds native
retention to one active plus one candidate. It queries Mach VM directly for
the old code AND data addresses: each mapping exists before replacement and
is absent after unload. These are OS observations, not just unload counters.
All 61 loaded modules are unloaded at closure. This does not establish absence
of every possible heap/runtime cache leak for arbitrary code or long sessions.

Compatibility tests reject changed imports, signatures, named/anonymous types,
initializers, globals, goroutines, retained callbacks, unapproved calls and
host-pointer writes. A real body edit to `strconv.Atoi` in the PRIVATE cloned
GOROOT, with preserved modification time, produces an explicit restart
requirement. Added dependency source membership and GOFLAGS/GOEXPERIMENT/
GOARCH/GOROOT changes also reject. The serving HTTP behavior remains old.
Restoring dependency bytes/source permits a fresh compilation and new response.

### Cache and input-identity boundary

There is no revision cache, TTL, mtime expiry or generic incremental linker.
Every compatible edit compiles fresh against frozen dependencies. The current
conservative fence hashes dependency source names/bytes, export archives,
compiler binaries and relevant environment before/after compilation. Any
changed frozen input requires restart. The standalone watcher observes only
the handler source; dependency/config changes are detected on the next reload
attempt, not by a background dependency watcher. Signature/import/global/type changes
are rejected before compiler/load. Module lifetime reclamation is separate.

The admitted application is one source file with four fixed standard-library
imports and a closed synchronous call set. No application dependency packages
are accepted. Initial host build/launch coherence is supplied by the proof's
build-and-launch procedure; runtime version/config checks alone do not prove
that an arbitrary prebuilt host used today's archives. This POC does not solve
that broader integration problem.

A possible next boundary is a supervisor-owned immutable snapshot, created
with the host build, of compiler and export archives. Body-only candidates
would compile against those same private bytes. An accepted dependency,
package-membership or build-config edit would cause an explicit normal restart
and new snapshot. That removes per-edit rehashing without TTL or ambiguous
expiry. It is an unimplemented integration possibility, not a demonstrated
claim or permission to build a generic invalidation framework.

### Timing with the complete fence

- Before the fence, `poc-mappings.log` observed 19.677, 20.315, 22.816 and
  20.287 ms for revisions1,2,3,60 with concurrent traffic and OS mapping checks.
- An early full watched-file run (`poc-watcher.log`) observed 441.736,
  444.447 and 451.133 ms; fence work alone was407–417ms.
- The final full run (`poc-final.log`) observed 256.202,257.059,254.381ms;
  fence work alone was226–232ms, compiler17–18ms, load0.14–0.15ms,
  drain/swap/unload0.34–0.42ms.
- These are separate observations on a tiny fixture. Startup/export discovery
  and initial host link remain outside edit timing. No whole-app speed claim.

### Friction found by extending the spike

- The original unsafe function-pointer trick used a stack address cell. A
  reloader returning from its load method must heap-retain that cell; otherwise
  HTTP calls jump through a stale address. The POC retains it per revision.
- Upstream runtime-symbol registration is not reentrant: its fixed slot array
  overflows on the second registration. The POC registers once per process.
- Go1.27's default test build strips symbols this loader needs; use the explicit
  linker flag in README. Missing tooling never turns these tests into skips.
- Darwin `mincore` does not provide Linux's ENOMEM unmapped-address contract.
  The mapping proof uses Mach VM region lookup, not an assumed portability rule.
- Unthrottled concurrent clients exhausted ephemeral ports after hundreds of
  thousands of requests. The final stress uses a shared connection pool and
  a 1ms request interval, preserving concurrent activity across replacements.
- Upstream loader failure does not promise allocation/runtime registration
  rollback. The command terminates on that exceptional failure rather than
  pretending its old module is necessarily safe or retaining unlimited retries.

Independent validation is pending; this section records local proof only.
