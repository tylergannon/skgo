# Adversarial review: native foundation, round 02

Date: 2026-10-08. Worktree `codex/zig-native-foundation` at `d362ced` plus the
uncommitted tree: `native/`, `example/native/`, `.gitignore`,
`.github/workflows/{ci,qualification}.yml`,
`internal/adapter/prerender_predicate_test.go`, and the worklog.

## Target

The entire current work, judged against `ephemeral/plans/zig-native-client.md`
(milestones 1 and the host/WebView half of 3, which the plan starts together),
`CLAUDE.md`, the agent protocol, and Kit 3.0.0 at
`example/web/node_modules/@sveltejs/kit`. Round 01 findings were re-checked.

Caller constraints honoured: read-only except this file; mutations ran on
scratch copies and Go overlays, never on the tree. No narrowing was requested,
so none was ignored.

## Evidence inspected

- Native core sources are byte-identical to round 01 (`root.zig`, `cli.zig`
  diffed against the round-01 copies); `build.zig.zon` now pins
  `archive/20a8d45bf73561af0cac227eb22a94084155c5b7.tar.gz`, the squash commit
  on devalue `main`. `zig fetch` of that URL reproduces
  `devalue-0.1.0-QJ2gKoMeBAC-jGq_M30rVJvF7jT-GzcB0MwMxEWxoIVk`.
- `example/native/`: `project.yml`, `host/main.go`, `host/main_test.go`,
  `ios/App.swift`, `ios/Bridge.h`, `ios/Info.plist`, `cmd/build/main.go`, and
  the ignored `build/` output (109 MB: c-archives and XcodeGen projects for
  `iphoneos` and `iphonesimulator`, no built app).
- `example/generate_test.go` and `example/typedrift_test.go` (sandbox copy and
  snapshot rules), `example/server.go` (`NewHandlerSized`),
  `example/web/dist.go` (embedded build).
- Kit sources as in round 01; `remote.go:835` for the query redirect envelope.
- Checks run:
  - `zig build test` uncached in `native/core`: 6/6 pass; `zig fmt --check`
    clean.
  - `go test ./native/`: pass (4.5 s). `go vet ./...` at the root and
    `go vet ./native/...` in `example`: clean. `gofmt -l`: empty.
  - `go test ./native/host/` in `example`: pass.
  - `go test ./internal/adapter/ -run Predicate -count=3`: pass.
  - `CI=true just test`: 26 packages ok, no FAIL or skip; slowest package
    75 s (`cmd/skgo`), `example` 63 s.
  - Linux runner reproduction in `ubuntu:24.04` with `CI=true`: fresh mise,
    `mise install zig` from `native/mise.toml` + `mise.lock`, then
    `mise exec -- zig version` prints `0.17.0` and exits 0 even though the
    darwin-only `aqua:yonaskolb/XcodeGen` pin is in the same config. Locally,
    `mise exec` does fail when a pinned tool exists for the platform but cannot
    be installed, so the Linux result is the one that matters for CI.
  - Mutations (overlay or scratch copy, tree untouched, `git status` clean):
    - host `SKGoHostStop` made a no-op: host test fails ("stopped host still
      responds");
    - host `start` opens a new listener on every call: fails ("repeated start");
    - host serves a blank 200 instead of the real handler: fails on the title;
    - `guard` admits regexp: Zig "unsupported argument nodes" test fails;
    - `receive` reports a redirect as a `.result`: **all 6 Zig tests and the
      Go CLI test still pass** (see finding 2).

## Round 01 status

- Finding 1 (unreachable pre-squash pin): resolved, pin is `20a8d45` on `main`,
  hash verified.
- Finding 2 (codec copy in `native/core/zig-pkg/`): resolved, `**/zig-pkg/`
  ignored; the worklog records that Zig 0.17 populates it by default.
- Finding 3 (CI has no Zig): resolved by `mise -C native install zig` in both
  workflows; the container run above shows the test's `mise exec` path works
  on Linux with the macOS-only XcodeGen pin present.
- Findings 4 and 5 (nitpicks): unchanged, restated below.

## Findings

### 1. Issue: the milestone 3 feasibility gate has not been run

The plan makes local iOS "a required outcome with an early feasibility gate":
the probe must bind loopback and "render a known SKGo page in WKWebView", and
"prove simulator and physical device execution, foreground startup,
suspend/resume, teardown and behavior under memory pressure", with only the
device outcome allowed to stay pending. What exists: a c-archive host
(`example/native/host/main.go`) with a Go test proving start/stop/restart over
HTTP on the Mac, a SwiftUI shell (`ios/App.swift`) that has never been
compiled, an XcodeGen spec, and generated projects for both SDKs. The worklog
records that `xcodebuild` exits 70 on this machine and `simctl` hangs on a
first-launch step, and that "neither proves app linking or device/WebView
execution". So no WebView render, no lifecycle, no memory-pressure evidence,
and the Swift file is unverified source (for example, `Page(url:)` reloads on
every `origin` change by `.id`, and `SKGoHostStop()` blocks the main thread
for up to two seconds on `.background`). This is the right honesty, but the
gate is the plan's stated precondition for shell/scaffold work and for
milestone 6, and it remains open. Resolving the Xcode installation (or using
another Mac) is the unblocker; the review cannot verify it here.

### 2. Issue: the query redirect path in `receive` is untested

`root.zig:191-195` returns `.kind = .redirect` when the devalue data carries
a `redirect` key, mirroring Kit's `query/index.js:30-33`, and skgo's dispatcher
really emits that envelope for a redirecting query (`remote.go:835`). A scratch
mutation that returns `.kind = .result` instead passes every Zig test and the
Go CLI test. With that bug a redirecting native query would surface as a
successful empty result. Neither `tests.zig` nor `remote_test.go` has a
redirect case; the CLI test can add one with a remote that returns a redirect,
and `tests.zig` can assert the kind and location from literal bytes.

### 3. Nitpick: disposable iOS output is copied into every test sandbox

`example/typedrift_test.go:301-306` skips `web/node_modules`,
`web/.svelte-kit/output`, `e2e` and `tmp` when copying the module into a
sandbox; `native/build` (109 MB after a probe build) is not skipped, so four
sandboxes copy it and `TestNothingGeneratedWasWrittenByHand` compares the
c-archives byte for byte. Git ignores the directory; the tests do not. Cheap
now, but it grows with every SDK slice and is exactly the "disposable build
output" the plan says lives outside the inputs.

### 4. Nitpick: `receive` is stricter than Kit's client in two harmless spots

Unchanged from round 01: `root.zig:186-187` rejects a 2xx `result` without
`data` where Kit's `remote_request` treats it as `{}`; `root.zig:159-164`
reads `error.message` from any non-2xx JSON regardless of `type`; the
`!success` branch at `root.zig:170-171` is dead.

### 5. Nitpick: HTTP-failure and alias coverage stop at `receive`

Unchanged from round 01: real non-2xx responses from the dispatcher (403
origin refusal, 405 GET on a command) are not exercised through the CLI, and
no fixture carries a shared object reference through `__skrao`. The round-01
Origin mutation shows the 403 path is load-bearing indirectly.

## Also checked

- The `prerender_predicate_test.go` change is a timing fix in an existing test
  (the late reply is now released after the real timeout failure); it passes
  three times in isolation and in the full suite, and no adapter logic changed.
  Protocol rule 7 covers fixing it on this branch.
- `project.yml` paths resolve correctly from the generated project directory
  (`$(SRCROOT)/../../../ios/Bridge.h` and `$(SRCROOT)/../host`), and
  `cmd/build/main.go` builds the c-archive with the SDK's clang before running
  XcodeGen under the `native/` mise pins.
- The host composes the production handler (`NewHandlerSized`, two runtimes,
  embedded `web.Build`) with the actual loopback origin, so Kit's origin check
  stays on.
- No new runner, manifest or JavaScript harness was added.

## Outcome

material findings remain
