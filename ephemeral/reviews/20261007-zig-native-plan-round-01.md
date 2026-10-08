# Adversarial review: Zig native client plan, round 01

Date: 2026-10-07. Reviewer worktree: `/Users/tyler/.codex/worktrees/5c57/skgo`
on `codex/zig-native-development-plan` (HEAD `c39de23`, main `89208d3`).

## Review target

`ephemeral/plans/zig-native-client.md` (338 lines, untracked), judged against:

- the authoritative user request: a phased development plan with 5–15
  milestones, opening with the value proposition and end state, using a Zig
  native client core and tracked XcodeGen formats with generated Apple
  projects as temporary artifacts, supporting the previously requested local
  and hosted macOS and iOS applications;
- `CLAUDE.md` (Go-library rule, mirroring-not-design, missions-not-methods,
  toolchain-missing-is-not-passing, never reimplement kit);
- the installed Kit 3.0.0 pin and its transitive devalue 5.9.4;
- the Devalue Zig codec at `/Users/tyler/src/devalue` (HEAD `e2d2dc6`, the
  revision the plan cites);
- skgo source on this branch and the two research documents the plan rests on.

Caller constraints honoured: read-only except this artifact; artifact path as
supplied. No caller instruction narrowed the defects, files or subject matter
under review, so nothing was ignored.

## Evidence inspected

- Plan text in full; `ephemeral/worklog/20261007-zig-native-plan.md`;
  `ephemeral/worklog/zig-native-client-research.md`;
  `ephemeral/research/swift-remote-client.md` (sections on the three-layer
  wire, shell questions, evidence limits);
  `ephemeral/research/zig-017-native-client.md` (cache, model duplication,
  executed probe, remaining proof).
- Kit pin: `example/web/node_modules/@sveltejs/kit/package.json` is 3.0.0;
  `src/runtime/shared.js` lines 94–360 (`__skrao`/`__skram`/`__skras`
  reducers, `stringify_remote_arg` with sorted reducers for queries,
  `stringify_command_arg` unsorted with `File` support, base64url without
  padding, `create_remote_key` = `id + '/' + payload`);
  `src/runtime/client/remote-functions/shared.svelte.js` (page-context headers
  `x-sveltekit-pathname`/`x-sveltekit-search`, refresh fulfilment and
  `fail_unhandled_refreshes`); `command.svelte.js` (POST JSON `{payload,
  refreshes}`); `query/index.js` (URL shape); `exports/vite/plugins/remote.js`
  (remote id is `hash(file) + '/' + name`); `runtime/server/csrf.js`
  (`get_self_origin(paths_origin, url_origin)`); `core/config/options.js`
  line 194 (`paths.origin` default undefined). The plan's Kit claims match.
- Devalue pin: `/Users/tyler/src/devalue` HEAD is `e2d2dc6` as the plan
  states; `zig/build.zig.zon` requires Zig 0.17.0, version 0.1.0; `zig/README.md`
  profile, ownership and "graph access is not thread-safe" text; `zig/src/root.zig`
  API; `zig/src/encode.zig` lines 56–76 (reducer replacements are themselves
  re-flattened through the reducer list, which is what Kit's `clones` map and
  marker guard against in JS). The plan's codec claims match.
- skgo: `remote.go` lines 472–535 and 733–740 (`RemoteConfig.Origin` is a
  runtime value; non-GET remote requests with a different `Origin` header get
  403 only when it is non-empty); `example/cmd/main.go` lines 26–32 (`--origin`
  defaults from `--listen`); `example/server.go` `NewHandler`; `example/web/
  vite.config.ts` (`paths.origin` fixed to `ORIGIN ?? http://127.0.0.1:8080`);
  `example/web/skgo.remotes.json` (ids already `<hash>/<name>`); no Go handler
  reads `x-sveltekit-pathname`, so the plan's "native calls can omit
  page-context headers" holds.
- Toolchain on this machine: `go1.27.1` with `ios/arm64` and `ios/amd64` in
  `go tool dist list`; `zig version` 0.17.0; `xcodebuild` present;
  `xcodegen` not found; `gomobile` not found; `example/mise.toml` pins only
  `node` and `npm:vite-plus`; `/Users/tyler/src/devalue/mise.toml` pins
  `zig = "0.17.0"`.
- Executed probe (scratchpad only, nothing written to the repo):

  ```
  GOOS=ios GOARCH=arm64 CGO_ENABLED=1 \
  CC="$(xcrun --sdk iphoneos -f clang) -arch arm64 -isysroot $(xcrun --sdk iphoneos --show-sdk-path) -miphoneos-version-min=16.0" \
  go build -buildmode=c-archive -o skgo-ios.a ./cmd      # run in example/
  ```

  succeeded on the first try and produced a 44.9 MB static archive of the
  example server, goja renderer and embedded web build. It also reported
  `-buildmode=c-archive requires exactly one main package` when pointed at
  `./cmd/...`, so the mobile host must be a `main` package. This proves
  linkability only, not loopback bind, WKWebView load, memory or lifecycle on
  a device.

Structural requirements of the request are met: twelve milestones, value
proposition and end state first, Zig core, XcodeGen YAML as source with
`.xcodeproj` as disposable output, all four local/hosted × macOS/iOS
combinations, and the Zig/Swift language exception recorded explicitly.

## Findings

### 1. Issue — the "early" iOS feasibility gate is sequenced behind two milestones it does not depend on

Plan lines 37–39 call local iOS "a required outcome with an early feasibility
gate" whose failure "requires revisiting that hosting design with the user".
Milestone 3 (lines 145–154) then "Depends on 2", and milestone 2 depends on 1,
and the probe is required to perform "a native remote command/query through the
Zig core". The risks the gate exists to retire — a Go HTTP server plus goja
runtime pool plus embedded assets living inside an iOS process, loopback bind,
WKWebView document load, suspend/resume, memory pressure, device signing — are
independent of the Zig core and of generated Swift. The c-archive build above
shows the Go side of that probe is reachable today with no milestone 1 or 2
work. Deferring it means the highest-uncertainty decision in the plan (whether
local iOS hosting is viable at all) is learned last among Phase A, after the
transport ownership, confinement and C-boundary designs have been built around
the assumption that an in-process host exists. Impact: if the gate fails, the
user is consulted after, not before, the client core's integration shape is
fixed. Fix: let milestone 3's host/WebView/lifecycle half start alongside
milestone 1 with a WKWebView-only probe, and keep only the "native call through
the Zig core" clause dependent on 2. The source anchor for this work also cites
gomobile (line 334) while the mechanism that works, and that gomobile wraps, is
`go build -buildmode=c-archive` with `GOOS=ios`; gomobile is not installed here
and is not needed.

### 2. Issue — how Zig and XcodeGen are obtained and pinned is unspecified, so the "one build gesture" and "fresh checkout" acceptances cannot be met as written

End state (lines 17–22) says Go commands "invoke XcodeGen and `xcodebuild`" and
milestone 6 (lines 196–205) requires "one Go-owned build gesture" that rebuilds
"from a fresh application checkout" with "pinned Zig/devalue inputs". Line 31
lists "selected dependency pins" as durable source but never says what pins
Zig 0.17.0 or XcodeGen or where they live. On this machine `xcodegen` is not
installed and the repository pins toolchains through `example/mise.toml`, which
knows only node and vite-plus. `CLAUDE.md` says a check that cannot run because
its toolchain is missing has not passed and must not be able to report that it
did; the plan repeats that rule (line 109) but gives no milestone the job of
making the toolchain present and pinned. Impact: milestone 6's acceptance is
unverifiable until someone decides, outside the plan, whether XcodeGen and Zig
arrive via mise, a vendored binary, or a documented manual install, and every
later milestone inherits that gap. Fix: name the pinning mechanism (the repo's
existing mise pattern is the obvious candidate) and make "fresh checkout with
only mise installed produces both targets" part of milestone 6's proof.

### 3. Issue — "reused as a pinned dependency" hides an unsolved packaging step for the Devalue Zig codec

The ownership table (line 132) says `tylergannon/devalue/zig` is "reused as a
pinned dependency", and milestone 1 says "Reuse the pinned codec". The codec
is a Zig package rooted at the `zig/` subdirectory of the devalue repository:
its `build.zig.zon` lists `paths` relative to that directory and its README's
only import instruction is a *path* dependency. Zig's package fetcher expects
`build.zig.zon` at the root of whatever archive or git URL it fetches, and a
GitHub archive of the devalue repository has it one level down, so a
`build.zig.zon` URL+hash pin cannot point at this package as it exists today.
The viable options — a release tarball of `zig/` alone, a git submodule or
vendored copy, or moving the package to the repository root upstream — are
different amounts of work and the second one conflicts with "pinned". Impact:
milestone 1 cannot start as described without first making a cross-repository
decision the plan does not record, and the plan's claim that no upstream change
is needed (line 311 only covers Polytype) is silent on devalue. Fix: state the
dependency mechanism and, if it requires an upstream release or layout change,
list it as a milestone 1 prerequisite.

### 4. Issue — milestone 5 reimplements Kit's client query cache without recording it as an exception or citing authority for it

`CLAUDE.md` opens with "We never reimplement kit", and the plan's own
"Authority" section (lines 46–58) records exactly one exception, the
Zig/Swift language exception for client modules. Milestone 5 (lines 183–194)
then requires a native reimplementation of Kit's `query/instance.svelte.js`,
`cache.svelte.js` and refresh-fulfilment rules: single-flight updates, Kit's
"key and fulfillment rules", "explicitly ignored keys and unhandled requested
refreshes", bounded cache retention. That is a port of Kit's client runtime
into Zig, which the research document itself calls "a Kit semantic question"
and notes "is not a literal replacement for JS finalizers, Svelte effect roots
and ticks". The authoritative request as restated to this review asks for a
Zig native client core; it does not mention caching, and the plan's value
proposition (line 10) introduces "native query caching" as a given. Milestones
8 and 11 then depend on it. Impact: the plan's largest Zig surface after the
codec is justified only by inference, and if the user did not ask for it, it is
over-engineering that also crosses a project rule the plan otherwise handles
carefully. Fix: either record the user's explicit request for native query
caching and add "reimplementing Kit's client cache subset" to the Authority
section as a second named exception, or demote milestone 5 to "stateless typed
calls plus explicit refresh of named queries" and let caching be separate work.

### 5. Nitpick — the plan prescribes designs where `CLAUDE.md` says plans carry mission, acceptance and ownership only

`CLAUDE.md` ("Delegate missions, not methods") says sprint documents contain
"No file lists, no prescribed designs, no step-by-step". The plan hedges its
table as "ownership, not a prescribed implementation recipe" (line 127) but
the component section fixes that URLSession executes HTTP on Apple, that a C
ABI sits between Swift and Zig, that one serialized owner confines each core,
that handles are opaque, and milestone 1 fixes a Zig CLI with its own loopback
HTTP transport that milestone 2 immediately makes injectable and replaces on
Apple. Some of these are constraints the codec forces (exclusive graph access)
and belong; others (two transports, URLSession-not-Zig) are choices the
implementer is told to map from pinned sources and then not allowed to make.
Impact is low because the choices are defensible, but an implementer following
the plan literally cannot discover a better boundary, which is the failure mode
the rule exists to prevent. Fix: move the codec-forced constraints into the
non-negotiables, and phrase the rest as questions the first milestone must
answer.

## Outcome

material findings remain
