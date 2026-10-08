# Adversarial review: Zig native client plan, round 03

Date: 2026-10-07. Reviewer worktree: `/Users/tyler/.codex/worktrees/5c57/skgo`
on `codex/zig-native-development-plan` (HEAD `c39de23`, main `89208d3`).
Earlier rounds: `ephemeral/reviews/20261007-zig-native-plan-round-01.md`,
`ephemeral/reviews/20261007-zig-native-plan-round-02.md`.

## Review target

`ephemeral/plans/zig-native-client.md`, now 393 lines (untracked, modified
19:54), re-read in full against the same authoritative sources as rounds 01
and 02: the user request (phased plan, 5–15 milestones, value proposition and
end state first, Zig native client core, tracked XcodeGen formats with
generated Apple projects as disposable artifacts, local and hosted macOS and
iOS), `CLAUDE.md`, the installed Kit 3.0.0 pin and devalue 5.9.4, the Devalue
Zig codec at `/Users/tyler/src/devalue` HEAD `e2d2dc6` (unchanged since round
01), skgo source on this branch, the research documents and the worklog.

Caller constraints honoured: read-only except this artifact; artifact path as
supplied. No caller instruction narrowed the subject matter; nothing was
ignored.

## Evidence inspected

- Changes since round 02, by re-reading: the vendored snapshot is gone.
  Lines 76–88 make publishing a consumer-ready Devalue Zig source archive
  (package-root `build.zig`/`build.zig.zon`, codec, licenses, the shared
  conformance fixtures, tests runnable after unpacking) an explicit
  cross-repository prerequisite of milestone 1, pinned by real URL and package
  hash with no placeholder; line 178, lines 197–199 and lines 274–276 follow
  through for the ownership table, milestone 1 and milestone 6 ("no codec
  source copies"). Lines 62–63 point the query-cache authorization at the
  worklog, whose new decision line holds the full user message. Lines 92–94
  require the `lockfile` setting in the native `mise.toml` and a fresh-checkout
  proof that the lock is consulted.
- Worklog `ephemeral/worklog/20261007-zig-native-plan.md`: three new lines
  record the archive correction, the quoted user message (timestamps not
  available), and the mise lockfile friction.
- Round 02 findings: 1 (vendoring) resolved by the archive prerequisite; 2
  (quote provenance) resolved; 3 (lockfile setting) resolved.
- Verified against sources:
  - `zig fetch --help` accepts git+https URLs and tarballs; either form needs
    `build.zig.zon` at the package root, which is what the prerequisite
    produces. Devalue's `zig/build.zig.zon` lists `paths` relative to `zig/`
    and its `build.zig` defaults the corpus to `../v5/testdata/golden.json`,
    so the plan's "may require an upstream build/fixture-layout change" (lines
    81–82) is accurate. Devalue's CI (`.github/workflows/zig.yml`) runs
    `just test-zig` and `just lint-zig`; there is no release or archive step
    today, matching line 87–88 "not an archive that exists today".
  - `mise settings get lockfile` recognises the setting and reports it unset
    here, consistent with lines 92–94.
  - Milestone 4's "existing remote identity analysis": skgo does not compute
    Kit's remote hash itself. `internal/gen/links.go` line 30 says kit hashes
    the authored path, and `internal/gen/emit.go` records the adapter's ids in
    `skgo.remotes.json`; `internal/gen/clients.go` already lowers Go types
    through `polytype/grammar` and `typegrammar`. The milestone's premises
    hold.
  - XcodeGen 2.46.0, mise registry entry, Xcode 26.5 and Zig 0.17.0 were
    verified in round 02 and have not changed.
- Structural requirements of the request remain met: twelve milestones, value
  proposition then end state, Zig core, XcodeGen YAML as source with generated
  projects as gitignored output, all four local/hosted combinations.

## Findings

No material findings remain. The nitpicks below do not block starting
milestone 1 and the host/WebView half of milestone 3.

### 1. Nitpick — "native-enabled SKGo checkouts carry `mise.toml` pins" names no location in a repository that has no root `mise.toml`

Lines 90–92 say the pins live in "Native-enabled SKGo and application
checkouts". skgo's only `mise.toml` is `example/mise.toml`, and the `Justfile`
puts `.tools/bin` on PATH rather than using mise at the root. Whether the
Zig/XcodeGen pins join `example/mise.toml`, a new root file, or a
`native/`-scoped file decides which directory `mise install` must run in for
the fresh-checkout proof of milestone 6 (lines 271–273). Saying which, or
delegating the choice explicitly to milestone 1, removes an ambiguity the
proof would otherwise surface late.

### 2. Nitpick — gomobile remains a source anchor after being demoted to optional

Line 232 makes Go's `c-archive` build the initial host mechanism and gomobile
"optional packaging convenience, not a prerequisite", but line 390 still lists
the gomobile documentation as a source anchor with nothing pointing at the
`go build -buildmode=c-archive` path that actually works for `GOOS=ios` (the
round 01 probe). A reader following the anchors is led to the tool the plan
says not to start with. Replace or supplement the anchor with the Go
`cmd/go` build-mode documentation.

### 3. Nitpick — the upstream archive prerequisite sits inside milestone 1 rather than ahead of it

Lines 197–199 fold "publish/validate the directly installable Devalue package"
into milestone 1 alongside protocol implementation. The publication is work in
another repository with its own CI and layout change (lines 81–82), and the
plan's execution order (line 358) says to begin milestone 1 now. Listing the
archive as the first action under "Execution order", or as a numbered
prerequisite, makes it visible that milestone 1's Zig work can proceed on a
local path dependency only until the archive exists, and that the milestone
cannot close without it.

## Outcome

only nitpicks remain
