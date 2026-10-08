# Adversarial review: Zig native client plan, round 02

Date: 2026-10-07. Reviewer worktree: `/Users/tyler/.codex/worktrees/5c57/skgo`
on `codex/zig-native-development-plan` (HEAD `c39de23`, main `89208d3`).
Round 01: `ephemeral/reviews/20261007-zig-native-plan-round-01.md`.

## Review target

`ephemeral/plans/zig-native-client.md`, now 386 lines (untracked, modified
19:49), re-read in full against the same authoritative sources as round 01:
the user request (phased plan, 5–15 milestones, value proposition and end state
first, Zig native client core, tracked XcodeGen formats with generated Apple
projects as disposable artifacts, local and hosted macOS and iOS), `CLAUDE.md`,
the installed Kit 3.0.0 pin and devalue 5.9.4, the Devalue Zig codec at
`/Users/tyler/src/devalue` HEAD `e2d2dc6`, skgo source on this branch, the two
research documents and the updated worklog.

Caller constraints honoured: read-only except this artifact; artifact path as
supplied. No caller instruction narrowed the subject matter; nothing was
ignored.

## Evidence inspected

- Plan diff versus round 01, by re-reading: the Authority section gained the
  user's query-cache authorization (lines 61–66), the Devalue snapshot
  mechanism (lines 75–84) and the mise/XcodeGen/Xcode pinning paragraph (lines
  86–97); milestone 3 was re-sequenced alongside milestone 1 with a Kit-client
  WebView probe first and the Zig-core extension gated on 2 (lines 214–229);
  milestone 1 now owns the Zig pin and codec dependency (lines 192–194);
  milestone 6 now requires a fresh checkout with only mise and Xcode (lines
  265–269); the diagram is labelled a starting architecture to validate (lines
  144–153); gomobile is demoted to optional (line 226). Worklog
  `ephemeral/worklog/20261007-zig-native-plan.md` records each of these as a
  correction or decision.
- Round 01 findings: 1 (gate sequencing) resolved; 2 (toolchain pinning)
  resolved as far as a plan can resolve it; 3 (codec dependency mechanism)
  answered, but see finding 1 below; 4 (cache authority) answered by quoting
  the user; 5 (prescriptiveness) softened.
- Verified facts the revised plan now asserts:
  - XcodeGen `2.46.0` is the latest release, published 2026-07-16
    (`gh release list -R yonaskolb/XcodeGen`).
  - mise `2026.10.4` is installed; `mise registry` lists `xcodegen` as
    `aqua:yonaskolb/XcodeGen`; `mise ls-remote aqua:yonaskolb/XcodeGen` lists
    2.46.0; `mise lock` exists. The `lockfile` setting is not enabled in this
    environment, so `mise.lock` is a thing to turn on, not a default.
  - `xcodebuild -version` reports Xcode 26.5 (17F42), matching line 94.
  - `zig version` is 0.17.0, matching the codec's `minimum_zig_version`.
  - `xcodegen` and `gomobile` are absent, as line 95 says.
- Devalue codec: `zig/build.zig` line 8 defaults the shared fixture path to
  `../v5/testdata/golden.json`, outside the `zig/` directory the plan proposes
  to snapshot; `zig/testdata/` holds only the two flat goldens. The README's
  only import instruction is a `.path` dependency.
- skgo: no root `mise.toml` (only `example/mise.toml`, pinning node and
  vite-plus); `internal/newapp/newapp.go` writes no mise configuration for a
  generated application; `internal/adapter/skgo-adapter.js` and `adapter.go`
  do not read or record `paths.origin`, so skgo's origin remains the runtime
  value `RemoteConfig.Origin` while Kit bakes `__SVELTEKIT_PATHS_ORIGIN__` into
  its server bundle. The plan's lines 125–127 already name that as needing a
  real built-application test; milestone 3 forbids the fixed-port shortcut, so
  the probe will meet it.
- Project memory `skgo-one-devalue-polytypes` (2026-09-06 decision): skgo
  keeps exactly one devalue implementation, polytype's; "do not reintroduce a
  devalue implementation inside skgo".
- Structural requirements of the request remain met: twelve milestones, value
  proposition then end state, Zig core, XcodeGen YAML as source, generated
  projects gitignored build output, all four combinations.

## Findings

### 1. Issue — vendoring the Devalue Zig package into skgo and into every application contradicts the project's one-devalue decision and costs more than the upstream fix the plan defers

Lines 75–84 make a checked-in copy of Devalue's `zig/` tree under skgo
`native/core/deps/devalue/` the "first dependency mechanism"; line 173 repeats
it; lines 268–269 extend it so that "the application's pinned client source
includes its Devalue snapshot", meaning the scaffolder copies the codec into
each generated application as well. That is three maintained copies of the same
codec source (upstream, skgo, every app), each advanced by hand "at a
deliberately selected revision". The project recorded on 2026-09-06 that skgo
holds no devalue implementation of its own and depends on the one its author
maintains elsewhere, precisely because a local copy is the thing a later session
drifts or patches; a Zig copy is the same hazard in a different language, and
`CLAUDE.md`'s "prefer removing hazards" applies. The snapshot is also not a
clean unit: `zig/build.zig` resolves its conformance corpus to
`../v5/testdata/golden.json`, which does not exist inside a copied `zig/`
directory, so the vendored package's own `zig build test` fails by default and
the snapshot can only be built, never checked, where it lives. The plan's own
line 83 says an upstream release archive rooted at `zig/` "can replace this
mechanism later; none is required to begin", but the user owns
`tylergannon/devalue`, and producing one tagged archive of `zig/` (or moving
the package to the repository root) is less work than writing and maintaining
a snapshot policy in skgo plus a copy step in the application scaffolder. Zig
0.17's `build.zig.zon` can then pin that archive by URL and hash in both skgo
and generated applications with no copied source. Impact: milestones 1 and 6
build durable copy-and-sync machinery that the plan already expects to throw
away, and generated applications inherit a vendored tree they cannot test.
Fix: make "publish a Zig package archive of `zig/` at `e2d2dc6` (or move the
package to the repository root) upstream" the milestone 1 prerequisite, pin it
by URL and hash, and drop the snapshot and the per-application copy.

### 2. Nitpick — the query-cache authorization cites a conversation that no durable record holds

Lines 61–66 ground milestone 5's exception to "never reimplement kit" in a
user message quoted "earlier in this conversation". The worklog's last
correction records that the quote was added, but neither the worklog nor the
research documents record the message itself, its date, or where it was said
(`grep` across `ephemeral/research` and `ephemeral/worklog` finds no "wire
client" or "query cache" request from the user). A reader of the plan without
this session's transcript cannot check the one sentence that authorizes the
largest non-codec Zig surface. Fix: add the date and a pointer to the record
the quote comes from, or paste the full message into the worklog decision line.

### 3. Nitpick — `mise.lock` integrity (line 88) assumes a setting the environment does not enable

mise writes and honours `mise.lock` only when its `lockfile` setting is on;
`mise settings get lockfile` reports it unset here. The plan's "fresh checkout
starting with mise" proof (lines 265–267) should therefore include the setting
in the checkout's `mise.toml`, or the integrity information it promises will
not be consulted. No milestone names this.

## Outcome

material findings remain
