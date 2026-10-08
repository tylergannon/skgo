# Adversarial review: native iOS client probe, round 01

Date: 2026-10-08. Branch `codex/native-ios-client-probe` at `c5ae38f`, seven
commits ahead of `main`, including the pending query-cache work from PR #288.

## Review target

Milestone 3 of `ephemeral/plans/zig-native-client.md` (local iOS feasibility:
XcodeGen-built probe linking the real Go application and goja renderer through
a mobile-host boundary, loopback bind, a known page in WKWebView, simulator and
device execution, foreground startup, suspend/resume, teardown, memory
pressure, and the milestone-2 extension that runs a native Zig-core query and
command), judged against `AGENTS.md`. The surrounding native core
(`native/core`), Swift package (`native/swift`), generator changes
(`internal/swiftgen`, `internal/gen`) and example wiring were read as the
context the probe depends on.

The launch prompt stated operating facts (Xcode's `DVTDownloads` breakage,
no simulator or device run yet, milestone unfinished). Those were treated as
claims to verify, not as scope limits; no caller narrowing was applied.

## Evidence inspected

- `AGENTS.md`; the plan's authority, boundary and milestone sections; the
  branch worklog `ephemeral/worklog/native-ios-client-probe.md`.
- Full diff `main..HEAD` (30 files). Read in full: `example/native/project.yml`,
  `example/native/ios/{App.swift,Generated.swift,Info.plist,Bridge.h}`,
  `example/native/host/{main.go,main_test.go}`, `example/native/cmd/build/main.go`,
  `example/internal/skgo/config.go`, `example/web/vite.config.ts`,
  `native/swift/Sources/**`, `native/swift/Package.swift`, `native/swift/README.md`,
  `native/core/src/abi.zig`, `native/core/build.zig`, the epoch/reset paths of
  `native/core/src/cache.zig` and `query_response.zig`, `native/swift_test.go`,
  `native/remote_test.go`, the generator diff and its Swift tests, and
  `native/mise.{toml,lock}`.
- Kit pin 3.0.0: `runtime/server/csrf.js`, `runtime/server/respond.js`,
  `exports/vite/index.js`, `core/adapt/builder.js`, `core/postbuild/prerender.js`
  for every use of `paths.origin`.
- Ran, from this checkout:
  - `go vet ./native/... ./example/native/...`: clean.
  - `go test -count=1 ./example/native/host/`: pass (0.4 s; two start/stop
    cycles, literal `Todos` title asserted).
  - `go test -count=1 ./native/`: pass (Zig build + `swift test` + Swift CLI
    against the production registry).
  - `go test -count=1 -run TestGeneratedSwift ./internal/gen/`: pass.
  - `zig build library -Dtarget=aarch64-ios.17.0-simulator --sysroot <sdk>`:
    produces `libskgo_native_core.a`.
  - `GOOS=ios GOARCH=arm64 go build -buildmode=c-archive ./native/host` with the
    simulator clang: produces `skgo-host.a` and `skgo-host.h` (46 MB archive).
  - `xcodegen generate --spec example/native/project.yml`: writes
    `SKGoNativeProbe.xcodeproj` with no warnings.
  - Direct `swiftc` compile of `SKGoNative` as a static module, then
    `App.swift` + `Generated.swift` with the bridging header, linked against
    the three archives for `arm64-apple-ios17.0-simulator`: links to a Mach-O
    arm64 executable. This confirms the caller's "compilation and linkage"
    statement independently of Xcode's project machinery.
  - `xcodebuild -project ... -list`: fails loading `IDESimulatorFoundation`
    (`DVTDownloads` symbol missing), as reported. `xcrun simctl list devices`
    hangs past 60 s, so CoreSimulator is unusable on this machine too; the
    blocker is the Xcode installation, not the project.
- The example's `TestNothingGeneratedWasWrittenByHand` sandbox copies the
  whole example tree except `native/build`, so the tracked
  `example/native/ios/Generated.swift` is covered by the existing drift test.

## Findings

### 1. Critical — incomplete requirement: no part of the probe has been demonstrated running

Milestone 3 lists its proof explicitly: "Prove simulator and physical device
execution, foreground startup, suspend/resume, teardown and behavior under
memory pressure", with a known SKGo page rendered in WKWebView. None of that
has happened. `AGENTS.md` forbids claiming "anything that hasn't been
demonstrated running", and its "open the box" rule applies directly: a page
in a WebView is exactly the kind of thing only a picture can verify.

What exists is build evidence only: the Zig archive, the Go c-archive, an
XcodeGen project, and (verified here) a linkable simulator binary. The branch
worklog records the pending outcome honestly, and the final commit message
("link the Zig native client into the iOS hosting probe") does not overclaim.
But the milestone's gate is execution, and the gate "precedes extensive
shell/scaffold work". Until a simulator or device run has been looked at, the
lifecycle code in `App.swift` (scene-phase stop/start, origin replacement,
client shutdown) is unexercised in any environment, and finding 3 below is
the kind of thing that run would surface immediately.

Evidence: `example/native/ios/App.swift:1-74` has no test of any kind;
`example/native/host/main_test.go` is HTTP-only and says so. `xcodebuild`
and `simctl` are both broken on this host (see above), so the unblock is
repairing or reinstalling Xcode 26.5 (`xcodebuild -runFirstLaunch` is the
first thing to try), not changing the project. Because direct `swiftc` linking
works, a hand-assembled `.app` bundle installed with `simctl` would be a
viable route once CoreSimulator loads, without waiting on `xcodebuild`.

Impact: milestone 3 cannot be marked met; milestone 6 depends on it.

### 2. Issue — the dynamic-origin Kit build is an untested configuration that also silently reconfigures the shared embed

`example/web/vite.config.ts:9-16` adds an `SKGO_NATIVE_LOCAL=1` branch that
leaves `paths.origin` undefined, and `example/native/cmd/build/main.go:32-33`
sets that variable when it rebuilds the frontend. The plan says the opposite
of what was done: "Dynamic local ports need an actual built-application test,
not an assumption from configuration alone."

Three concrete problems:

- Nothing exercises the flag. `example/native/host/main_test.go` serves
  whatever `example/web/build` currently holds. The built output carries no
  marker of which origin configuration produced it (grep of
  `example/web/build` finds neither `127.0.0.1:8080` nor
  `sveltekit-prerender`), so the host test passes identically against either
  build and cannot tell them apart.
- What the flag changes for skgo is unmapped. In the pinned Kit, the server
  reads `__SVELTEKIT_PATHS_ORIGIN__` in exactly one place,
  `runtime/server/csrf.js:19` via `respond.js:103`, and skgo's Go middleware
  owns that check with `HandleConfig.Origin` supplied by the host
  (`example/native/host/main.go:52`, `example/server.go:95`). The other users
  are build-time prerender origins (`core/adapt/builder.js:178`,
  `core/postbuild/prerender.js:154`). So the comment in `vite.config.ts`
  ("a bundled local host can use its actual dynamically allocated loopback
  port") describes a Node-server behaviour, not a skgo one; whether the
  change is needed at all, and what it does to prerendered output, has not
  been established from source.
- The probe build overwrites `example/web/build`, which is the embed used by
  `just test`, `just e2e` and the desktop example binary. After one probe
  build, every other suite runs on a differently configured Kit build with
  no record of it. `AGENTS.md`'s hazard rule applies: two configurations
  sharing one output directory is a trap that needs a standing rule to avoid,
  so it should not exist.

Impact: a configuration difference nobody can observe or test, and a shared
build directory whose contents depend on which command ran last.

### 3. Issue — the lifecycle handler blocks the main thread on every background transition, and tears down more than it proves it releases

`App.swift:34-43` reacts to `.background` by calling `SKGoHostStop()`
synchronously on the main actor. `SKGoHostStop` (`host/main.go:69-82`) runs
`http.Server.Shutdown` with a two-second context and then `Close`. The todos
page the probe loads holds a `query.live` stream (`TodoCount.svelte:5`,
`watchCount`), which `remote_live.go:81-95` keeps open with a keep-alive
timer; `Shutdown` only closes idle connections and waits for active ones, so
with the WebView showing that page the call waits the full two seconds before
falling back to `Close`. Two seconds of main-thread blocking during the
background transition is inside iOS's budget but is a visible hang on every
app switch, and it is the first thing a simulator run would show.

The same handler then sets `origin = nil`, which destroys the `WKWebView`
(`.id(origin)`), and `.active` calls `start()` which binds a new port and
constructs a new handler with two goja runtimes (`NewHandlerSized(..., 2)`)
on the main thread. Each cycle therefore compiles the SSR bundle twice on the
UI thread and abandons the previous pool. `main_test.go` proves the old port
stops answering; it does not prove the old renderer, manifest and pool are
released, which is the "repeated start/stop" and "memory pressure" clause of
the milestone. Separately, `client.shutdown()` is launched as a detached task
while the host is stopped synchronously, so an in-flight native call sees a
connection failure rather than `RemoteError.closed`.

Impact: a user-visible stall on each background, and unmeasured growth across
cycles on a target where memory pressure is a stated acceptance criterion.
This is design under the milestone's lifecycle requirement rather than a
proven crash; it needs the run in finding 1 to be measured.

### 4. Nitpick — `resetSession()` in the probe does not reset a session

`App.swift:56-59` signs in, calls `client.resetSession()`, then queries
`whoami`. The client's reset clears the Zig cache and bumps the epoch
(`RemoteClient.swift:75-81`, `Query.swift:65`); it cannot touch the
`.ephemeral` URLSession's cookie store, which still holds `skgo_session`. So
the query after reset returns the signed-in user and the displayed line
cannot distinguish "reset worked" from "reset did nothing". The README
("Call `resetSession()` after changing authentication") describes the real
contract; the probe's use is decorative. Either drop the call or make the
probe show something that only a cleared cache produces.

### 5. Nitpick — worklog entry narrates status

`ephemeral/worklog/native-ios-client-probe.md` second entry ("constraint:
... remain pending until the installation and device access are available")
is a status report. `AGENTS.md` reserves the worklog for corrections, traps
and decision-changing facts. The first entry (Zig 0.17's default panic
handler cannot compile for iOS; use the `library` step) is exactly that and
should stay. The actionable form of the second is one line: `simctl` and
`xcodebuild` both fail on this host because of the system `DVTDownloads`
framework; direct `swiftc` linking against the Apple SDKs works.

## Not findings, recorded because they were checked

- Epoch accounting is aligned on both sides of the ABI: Swift and Zig both
  start at 1 and increment only after a successful `sk_cache_reset`
  (`Query.swift:62-65`, `cache.zig:11,63-67`); stale command completions are
  dropped by call ID and stale query completions by epoch.
- `project.yml` path arithmetic resolves correctly for the layout the build
  command creates (`$(SRCROOT)` = `example/native/build/<sdk>/project`);
  XcodeGen generates it cleanly and the equivalent flags link by hand.
- Zig, Go and XcodeGen pins are tracked with lockfile checksums
  (`native/mise.lock`); XcodeGen 2.46.0 is installed through mise, as the
  milestone requires.
- No fixed-port shortcut, no Node sidecar, no Go endpoint in JavaScript;
  the host binds `127.0.0.1:0` and serves the production handler composition.
- `Generated.swift` is the generator's output and is covered by the example's
  existing hand-write drift test; the generator refuses to overwrite an
  authored Swift file.

## Outcome

material findings remain
