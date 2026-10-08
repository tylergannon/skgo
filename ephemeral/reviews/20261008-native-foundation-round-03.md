# Adversarial review: native foundation, round 03

Date: 2026-10-08. Worktree `codex/zig-native-foundation` at `1cf4e50`
("feat: add Zig remote client and local iOS host probe"), clean tree apart
from the untracked `ephemeral/reviewer-logs/`.

## Target

The entire committed work, judged against `ephemeral/plans/zig-native-client.md`
(milestone 1, the host/WebView half of milestone 3, and the gate the plan puts
before milestone 6), `CLAUDE.md`, the agent protocol and Kit 3.0.0 at
`example/web/node_modules/@sveltejs/kit`.

Caller context taken as authoritative: Xcode is mismatched and cannot be
updated here, so available work continues; milestone 3 is pending, not
claimed. The caller also disputed round 02's statement that `App.swift` was
never compiled and named `example/native/build/iphonesimulator/SKGoNativeProbe`
as the executable. That is a factual claim, not a verdict request, and it was
verified independently below rather than accepted. No narrowing was requested.

Operating constraints honoured: read-only except this file; every mutation ran
on a scratch copy of the Go module and Zig package, never on the tree.

## Correction to round 02

Round 02 said `App.swift` "has never been compiled". That was wrong.
`example/native/build/iphonesimulator/SKGoNativeProbe` is a Mach-O arm64
executable with `LC_BUILD_VERSION` platform 7 (iOS simulator), minos 17.0,
SDK 26.5, linking SwiftUI, WebKit and the Swift runtime, and exporting
`_SKGoHostStart`, `_SKGoHostStop` and `_main`. It is 22 MB and the Go host
archive's symbols are inside it. I reproduced the link from a scratch copy of
`example/native/ios` and the simulator `skgo-host.a` with

```
xcrun --sdk iphonesimulator swiftc -parse-as-library \
  -target arm64-apple-ios17.0-simulator -sdk "$SDK" \
  -import-objc-header ios/Bridge.h -I host ios/App.swift host/skgo-host.a \
  -framework WebKit -framework SwiftUI -o probe
```

which typechecks and links in under a second. Without `-parse-as-library` the
compile fails on `@main` (top-level code), which is a swiftc invocation detail,
not a source defect; xcodebuild supplies that flag for SwiftUI targets.

## Changes since round 02

- `native/core/src/tests.zig:91-97` adds the query redirect case; round-02
  finding 2 closed.
- `native/remote_test.go:69-78,113-114` adds a redirecting query and a GET on
  a command (`http_error (405)`) through the real dispatcher; round-02 finding
  2 and part of 5 closed.
- `example/typedrift_test.go:302` skips `native/build` when copying the
  sandbox; round-02 finding 3 closed.
- Worklog records the swiftc link and the user's direction that milestone 3
  stays pending with its gate before milestone 6.
- `root.zig`, `cli.zig`, `build.zig`, `build.zig.zon`, the host, the Swift
  shell, `project.yml` and the CI steps are unchanged from round 02.

## Evidence inspected and checks run

- Full read of the files above plus `example/native/cmd/build/main.go`,
  `example/native/host/main.go`, `ios/App.swift`, the plan's milestones 1, 3,
  6 and execution order, and the worklog.
- `zig build test` from a cleared `.zig-cache`/`zig-out`: 7/7 pass.
  `zig fmt --check`: clean.
- Fresh-checkout dependency fetch: on a scratch copy with `zig-pkg/` removed
  and `ZIG_GLOBAL_CACHE_DIR` pointed at an empty directory, `zig build test`
  fetched `archive/20a8d45…tar.gz`, matched
  `devalue-0.1.0-QJ2gKoMeBAC-jGq_M30rVJvF7jT-GzcB0MwMxEWxoIVk` and ran the
  tests. This is the path the Linux CI runner takes, which round 02 had only
  proven as far as `zig version`.
- `go test ./native/`: ok. `go vet ./...` (root) and `go vet ./native/...`
  (example): clean. `gofmt -l` reports only files unchanged by this branch.
- `go test ./native/host/` in `example`: ok.
- `CI=true just test`: 26 packages ok, exit 0, no FAIL or skip in the log
  (`native` 17 s, `example` 50 s, `devrender` 41 s).
- Mutations on the scratch copy, each run through both the Zig suite and the
  Go CLI test:
  - M1 `root.zig:194` reports a redirect as `.result`: Zig fails
    ("expected .redirect, found .result"), Go `TestZigRemoteCalls` fails.
  - M2 non-2xx responses reported as `.remote_error`: Zig fails at
    `tests.zig:51`, Go fails on the 405 step.
- Simulator state: `xcrun simctl list devices available` and `list runtimes`
  print nothing; `xcodebuild -version` reports Xcode 26.5 (17F42);
  `/Library/Developer/PrivateFrameworks/CoreSimulator.framework` is version
  1048. This matches the worklog's account of the mismatch.

## Findings

### 1. Issue: the milestone 3 feasibility gate is still open

Status is now accurately reported as pending by both the worklog and the
caller, so this finding records the remaining gap rather than a misstatement.
What exists: a Go c-archive host with a start/stop/restart HTTP test on the
Mac, a linked simulator executable, an XcodeGen spec and generated projects.
What the plan requires and nothing yet shows (`zig-native-client.md:220-235`):
the probe installed and launched on a simulator (there is no `.app` bundle,
only a bare executable, and `Info.plist` is not packaged), a known SKGo page
rendered in WKWebView, foreground startup, suspend/resume, teardown and
memory-pressure behaviour, and a device outcome. The plan says this gate
"precedes extensive shell/scaffold work" and "the complete milestone requires
both parts before 6", and the Xcode/CoreSimulator mismatch on this machine is
the blocker. Nothing in the current work should be read as feasibility proof,
and milestone 6 and shell work must not start on the strength of it. Unblocking
is a toolchain fix or another Mac; the review cannot perform it.

### 2. Nitpick: the simulator executable is produced by an unrecorded command

`example/native/cmd/build/main.go` stops after the c-archive and XcodeGen
generation; nothing tracked produces `SKGoNativeProbe`, and the worklog
records the swiftc link without its invocation. The plan names an
"XcodeGen-built probe" and wants Apple output regenerable after deletion
(`zig-native-client.md:40-45`). The executable is honest evidence of linking
(reproduced above) but a fresh checkout cannot recreate it until xcodebuild
works or the command is captured. Not material while the artifact is only
evidence and not a claimed deliverable.

### 3. Nitpick: lifecycle behaviour of the Swift shell is designed, not observed

`ios/App.swift:23-29` stops the Go server on `.background` and starts a new
listener on `.active`, so every resume gets a fresh port and `.id(origin)`
rebuilds the WKWebView, reloading the page and discarding client state.
`SKGoHostStop()` runs on the main actor and can wait up to the two-second
`Shutdown` budget (`host/main.go:84-88`) during the background transition.
Both are plausible for a probe and may be exactly what the plan's
suspend/resume proof should surface, but they are untested choices that the
gate run must observe rather than assume.

### 4. Nitpick: `receive` is stricter than Kit's client and carries a dead branch

Unchanged since round 01: `root.zig:186-187` rejects a 2xx `result` without
`data` where Kit's `remote_request` yields `{}`; `root.zig:159-164` reads
`error.message` on any non-2xx JSON regardless of `type`; the `!success` test
at `root.zig:170-171` is unreachable.

### 5. Nitpick: an empty untracked `native/core/tests/` directory

Created 01:46 and never used; git does not record it, so it is only local
clutter, but it suggests a path that was abandoned in favour of `src/tests.zig`.

## Also checked and found sound

- The redirect expectation `redirect (200): /sign-in` matches the dispatcher's
  envelope (`remote.go:835`, `{redirect: location}` inside devalue data) and
  Kit's client reading (`query/index.js:30-33`).
- The 405 step exercises the dispatcher's method check through a real socket;
  together with the 403 Origin mutation from round 01, both of Kit's
  non-envelope failure shapes now have a load-bearing path.
- `native/build` is ignored by git and now skipped by the sandbox copy;
  `TestNothingGeneratedWasWrittenByHand` still snapshots `project.yml` and the
  Swift sources, which are inputs and should be compared.
- No new runner, manifest, JavaScript harness or "done" command was added.

## Operational note

Running a scratch `zig build` at the same moment the background full suite was
building the tree's package produced a one-off `hash mismatch` naming an
`N-V-…` package: the two processes raced in the shared global Zig cache. A
clean rerun with an isolated cache fetched and verified normally. CI runs one
fetch per job, so this is a reviewer-concurrency artefact, not a project defect.

## Outcome

material findings remain
