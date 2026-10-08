# Adversarial review — native iOS client probe, round 02

Date: 2026-10-08 (local). Worktree `/private/tmp/skgo-native-ios-client`,
branch `codex/native-ios-client-probe`, HEAD `5fe0936` (PR #289, open),
merge-base with `main` `198b992`. Round 01 reviewed `c5ae38f`; this round
covers the whole branch as it stands, with emphasis on what changed since.

## Review target

The iOS client-probe work (`example/native/**`, `native/**`, the
`example/web` build configuration, the new `.github/workflows/native-ios.yml`,
and the worklogs) judged against `AGENTS.md` and milestone 3 of
`ephemeral/plans/zig-native-client.md` (Phase A, lines 215–236): an
XcodeGen-built probe linking the real Go app and goja renderer, loopback bind,
a known page in WKWebView, with simulator and device execution, foreground
startup, suspend/resume, teardown and memory-pressure behaviour proved, no Node
sidecar, fixed port or JS endpoint, and the probe extended with a native
Zig-core query/command.

Caller constraints honoured: read-only apart from this file; mutation checks
done through a `go test -overlay` of a scratch copy, never on tracked files.
The caller's prompt did not narrow scope.

## Evidence inspected

Source (current tree, plus `git diff c5ae38f` and `git diff main`):

- `example/native/ios/App.swift` (serialised lifecycle via `LocalHost` actor,
  `transition(active:)`, `generation` token, `callNative`).
- `example/native/iosTests/NativeProbeUITests.swift` (single XCUITest covering
  launch, WebView sign-in through Kit's client, native command+query, home
  press, reactivation, second native completion).
- `example/native/host/main.go` (`localhost` origin over a `127.0.0.1:0`
  listener; `SKGoHostStop` 2 s graceful shutdown) and `main_test.go`
  (`TestLocalHostStartStopAndRestart`, new `TestLocalHostSessionCookie`).
- `example/native/cmd/build/main.go` (now ends by running `xcodebuild build`
  or `xcodebuild test`; `SKGO_NATIVE_LOCAL` removed), `example/native/project.yml`
  (new `SKGoNativeProbeUITests` bundle.ui-testing target and scheme test
  action), `example/web/vite.config.ts` (fixed `paths.origin` restored).
- `.github/workflows/native-ios.yml` (macos-15, Xcode 16.4, destination
  `iPhone 16, OS=18.5`, `-test`, xcresult upload).
- `event.go:611-630` `secureCookieDefault` and `event_test.go:265`
  (Kit `runtime/server/cookie.js:78` localhost exemption mirrored);
  `native/swift/Sources/SKGoNative/RemoteClient.swift:40` (Origin header);
  pinned Kit client runtime has no use of `paths.origin`.
- `native/core/build.zig` (`library` step), `native/core/src/abi.zig` (iOS
  panic handler); `example/native/ios/Generated.swift`.
- `ephemeral/worklog/native-ios-client-probe.md`,
  `ephemeral/worklog/native-ios-simulator-tests.md`.

Proof run or fetched this round:

- `go vet ./example/native/... ./native/...` clean;
  `go test -count=1 ./example/native/host/ ./native/` → both `ok`
  (native 12.3 s, includes the darwin SwiftPM build+test).
- `go test -count=1 -run TestNothingGeneratedWasWrittenByHand ./example/` → ok
  (`Generated.swift` matches the generator after the #288 regeneration).
- Mutation (scratch overlay, host origin switched back to the numeric
  loopback): `TestLocalHostSessionCookie` fails with `Secure:true`, and
  `TestLocalHostStartStopAndRestart` fails on the prefix. Both are load-bearing
  for the cookie fix.
- GitHub Actions, workflow "iOS native probe", this branch:
  - run 37780650192 (`ea01223`, image release macos-15-arm64/20260907.0337):
    built everything, booted iPhone 16 / iOS 18.5, executed the UI test; it
    failed at `NativeProbeUITests.swift:21` after 93.8 s
    (`Executed 1 test, with 1 failure`). Lines 10–19 passed, so launch, host
    start, WebView render of Todos and Kit's own sign-in in the WebView were
    demonstrated on that commit. The native result assertion failed (the
    cookie bug the next commit fixes).
  - run 37782819944 (`5fe0936`, HEAD, same image release): finished during
    this review with `conclusion: failure`. `xcodebuild test` waited 60 s
    (13:18:29 → 13:19:29) then reported "Unable to find a device matching
    the provided destination specifier" with only the three placeholder
    destinations listed; the test never launched.
- Local Xcode remains unusable (round 01: `xcodebuild` plugin-load error,
  `simctl` hang); not re-run this round, nothing on the branch changes it.

## Round 01 findings, status

1. Critical "nothing demonstrated running" — partly addressed (CI simulator
   lane exists and has launched the app once); still open, see Finding 1.
2. `SKGO_NATIVE_LOCAL` dynamic-origin build — fixed. The flag is gone, the
   probe builds the ordinary embed, and the dynamic port is carried by the
   handler origin, which is the only server-side consumer in pinned Kit.
3. Host lifecycle on the UI actor — fixed. `LocalHost` actor, serialised
   transitions, client shutdown before host stop.
4. `resetSession()` result display — the displayed string now does depend on
   the reset (a stale cached `whoami` would print an empty query user). Closed.
5. Worklog narration — rewritten as friction/correction entries. Closed.

## Findings

### 1. Critical — the current head has never run in a simulator, and the lifecycle and native-extension halves of milestone 3 have never passed anywhere

Evidence: run 37782819944 for HEAD found no simulator device and exited 70
before launching the app. The only execution of the UI test (run 37780650192,
commit `ea01223`) stopped at `NativeProbeUITests.swift:21`, before
`XCUIDevice.shared.press(.home)` at line 23. Therefore no commit has
demonstrated: the native query/command completing (line 21), the app entering
background and the host stopping (lines 23–28), reactivation restarting the
host and re-rendering the page (lines 30–32), or a second native completion
(lines 33–35). The cookie fix in `5fe0936` is proved only by the Go host test,
not in the simulator where it failed.

Why it matters: `AGENTS.md` — "never claim anything that hasn't been
demonstrated running", and "a passing command is not evidence on its own".
Commit subjects `test: exercise the native iOS probe in an Xcode simulator`,
`test: require a new native completion after foreground resume` and
`test: exercise Kit remote functions in the actual WebView` describe tests that
exist but have not passed. Milestone 3's "prove simulator … execution,
foreground startup, suspend/resume, teardown" is still unproven; only
foreground startup plus WebView render plus WebView sign-in has been seen, and
on a superseded commit. The PR must not merge, and the milestone must not be
called done, until a run of HEAD shows `Executed 1 test, with 0 failures`.

### 2. Issue — the simulator proof is not reliable enough to be a gate, and it does not run for most changes that can break it

(a) `example/native/cmd/build/main.go:85-104` goes straight from
`xcodegen generate` to `xcodebuild test` with a hard-coded device name and OS
from the workflow. On the same image release, one run enumerated the device
and the next listed only placeholders after a 60 s wait. Nothing boots or even
lists a simulator first, and nothing retries. A gate that fails on a cold
CoreSimulator one time in two will be ignored, which is the `AGENTS.md` "skips
are failures" trap in a different costume: red that nobody reads is
indistinguishable from green. The destination should be discovered or the
runtime warmed before `xcodebuild`, and a device-enumeration failure should be
distinguishable from a test failure in the job output.

(b) `.github/workflows/native-ios.yml:5-9` triggers only on `native/**`,
`example/native/**` and `example/internal/skgo/config.go`. The probe links
the whole `skgo` module and the example app; the bug that broke run 1 lived in
`event.go` (`secureCookieDefault`). A change to `event.go`, `internal/**`,
`example/web/**` or `go.mod` ships without the simulator ever running, so the
proof silently does not execute for exactly the changes most likely to break
it. Run it on every push to the branch, or at least on the module paths the
host links.

### 3. Issue — milestone 3's memory-pressure requirement is unaddressed

`ephemeral/plans/zig-native-client.md:224-225` lists "behavior under memory
pressure" with suspend/resume and teardown. `grep -i memory` over
`example/native/ios/App.swift` and `example/native/host/main.go` finds
nothing: no `UIApplication.didReceiveMemoryWarningNotification` observer, no
pool shrink, no measurement, and the UI test does not exercise it. The worklog
(`native-ios-simulator-tests.md` decision entry) defers it to "actual
platforms", but the plan does not grant that exemption (it grants one only for
device access/signing), and a simulator can deliver memory warnings. At
minimum the probe needs a defined response to the warning and a test or
recorded observation of what two goja runtimes plus the Go heap cost under it;
otherwise the milestone text has to say the outcome is pending, as it does for
devices.

### 4. Nitpick — every cold launch starts the host twice

`App.swift:34-38`: `.task { transition(active: true) }` and
`.onChange(of: scenePhase)` for `.active` both fire at launch. The guard
`if active && origin != nil { return }` (line 42) is false while the first
start is still awaiting `host.start()`, so a second transition is queued, the
first one's `generation` check discards its result after the pool has been
built, and the second calls `host.start()` again. It works only because Go's
`start()` returns the existing origin (`main.go`, covered by the restart
test). Track an in-flight start in the guard, or drop one of the two triggers.

### 5. Nitpick — the UI test's post-resume proof leans on an absence check

`NativeProbeUITests.swift:33` asserts the old result text is gone before
tapping again; the "new completion" at line 35 is the same literal as line 21.
That is acceptable because line 33 is sandwiched between positive assertions,
but a result string that carried something unique to the completion (the
origin's port, say, which changes across a host restart) would prove the
restart without relying on the clear in `transition(active: false)`.

## Outcome

material findings remain
