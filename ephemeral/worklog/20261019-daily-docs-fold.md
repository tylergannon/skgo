# Docs fold through 4591dbaa

First documentation fold for skgo; no earlier marker. Ingested: every tracked
file under `ephemeral/` at `4591dbaa` (1,144 files, 16 MB) and the landed
commits `f7a9ec69..4591dbaa` (guide-page tree to HEAD). The next fold starts
after `4591dbaa`; find it with `git log --grep '^docs: fold'`.

## Ledger

Eight read-only agents dispositioned a slice each; their claims were
spot-checked against HEAD and Kit 3.1.0 before any edit.

| Input | Files | Disposition |
|---|---|---|
| `worklog/` Sept to 3 Oct | 133 | delete; 3 lessons folded (below) |
| `worklog/` 4 to 19 Oct | 91 | delete; 3 lessons folded; 10-19 files all landed (#393, #394, #401, #404, #405, #406) |
| `worklog/` undated topic files, loose `ephemeral/*` files, `issue-364/` | 83 | delete; 1 lesson folded |
| `reviews/` | 218 | delete; every reviewed change is on main |
| `issue-resolution/`, `issue-resolutions/`, `low-effort-agent/`, `bugs/` | 212 | delete; issues closed or their notes already posted to #247, gimbal#424, gimbal#428 |
| `research/`, `plans/`, `sprints/`, `survey/`, `brief/`, `tractor/`, `launcher-planning/`, `js-dependencies/` | 171 | delete; `registry.json` link repointed to a `4591dbaa` permalink |
| probes (`experiments/`, `cpu/`, `gen-cpu-20261007/`, `*-probe/`), `semantic-index/`, `SEMANTIC-INDEX.md`, `index/` | 231 | delete; the semantic index's token cache (`ephemeral/inspiration/`) is gone, and the index README and all four mission dossiers were stale or finished |
| `projects/dev-server/` | 3 | retain: changed 2026-10-15 and routed from AGENTS.md; three facts corrected (#331 reached main, #330 closed by fb8640df) |
| `guide-pages/SKILL.md`, `sveltekit-current/SKILL.md` | 2 | retain: repo skills routed from AGENTS.md |
| commits `f7a9ec69..4591dbaa` | 29 | 7 made a durable doc false (#368, #377, 2dcc49a2, #393, #394, #402, #403); corrected below |

## Folded

- `ephemeral/sveltekit-current/SKILL.md`: `$app/env` replaces `$app/environment`;
  public `$app/paths` is `asset`/`resolve`/`match`; sync writes types to
  `node_modules/$app/types/` (rediscovered four times); corrected `paths.origin`
  and the transport bullet (browser-only keys are allowed).
- `ephemeral/guide-pages/SKILL.md`: check claims against code, plain claims,
  no status narration, agent setup detail in `skills/skgo/SKILL.md` (Tyler's
  corrections in the README positioning work and #364); `ExampleLoad_parent`
  does not exist, `ExampleLoad_sharedState` does.
- Guide pages and README false at HEAD: `remote-functions.md` (a pending
  boundary keeps a query out of the document), `creating-a-project.md` (routes
  ignored in `go.mod`, #403), `project-structure.md` and `cmd/skgo/faq.md`
  (`.skgo/` ignored, #377; native-shell and prebuilt cache rows, #393),
  `cmd/skgo/cli-overview.md` (cask), `cmd/skgo/build.md` (the scaffold's
  `just build` never calls `skgo build`), `cmd/skgo/add.md` and
  `templateapi/create-your-own.md` (built-in `tailwindcss`),
  `cmd/skgo/skills.md` (#394 fixed the example disagreement), `README.md`
  (`--prebuilt` install, pinned prebuilt discovery, one status narration).
- `SVELTEKIT_COMPATIBILITY.md`: raw-material links pinned to `4591dbaa`; logs
  that were never committed said so; "this working branch" is #351.
- `Justfile`: release comment matches #402 (merge announces a candidate,
  dispatch publishes); `just e2e MODE ARGS...` now passes arguments to
  Playwright, replacing the index README's wrong single-scenario gesture
  (`pnpm test -- live` ran everything, four incidents).
- `AGENTS.md`, no added lines: "After a merge that publishes a release" became
  "After a release publishes" (since #402 no merge publishes); the guide-pages
  route also names the README, which its writing rules cover.
- `internal/toolchain/pins.go` godoc: skgo-project's docgen reads the constants
  by name (the #364 `NodeMinimum` rename).
- Test comments citing the deleted `ephemeral/inspiration` cache and a
  never-committed `csp-ground-truth.md` (`refresh_test.go`, `csp_test.go`,
  `internal/remotearg/kitgolden_test.go`, `internal/formdata/formdata_test.go`).

## Telemetry

`skill_issue: polytype ... severity=bug` (`worklog/20261008-generated-native-api.md`):
the polytype skill reference still says the grammar refuses recursion; false
since typegrammar admits productive named recursion (v1.10.0
`typegrammar/validate.go`). Filed tylergannon/polytype#170.

## Not folded: leads for issues or code, not docs

- `event.go` `mayWriteCookies` error text says only remote functions or commands
  may write cookies; the `SetCookie` godoc lists commands, forms, server loads,
  page actions and `handle`.
- "listening on" logs before the bind in `example/cmd/main.go`, the `skgo new`
  scaffold and the native-shell example; about nine worklogs hit a stale server.
- Malformed URI: `middleware.go` answers a plain 400 without `handle`; Kit runs
  `handle` and renders its error page. Locked by `TestSharedRoutingContracts`,
  absent from `differences.md`.
- A failed `skgo new` leaves a populated directory, so the advised retry fails.
- Gimbal's `release-macos.sh` calls `go tool skgo native build`, removed in #393.
- PR #289 (native iOS probe) edits paths #393 moved; likely orphaned.
- `adapter-suite-runtime.md` noted Svelte's string `+=` on goja as a possible
  SSR allocation cost; unmeasured, dropped.
- `plugins/native-shell/sdk/swift/README.md` reported as overclaiming Go-run
  Swift checks; the text read at HEAD did not clearly claim it, left alone.
- `internal/sv/sv-addon.source.js` always overwrites `scripts.test`, dropping
  sv's `test:e2e` chain; the sv unanswered-question error has no `skgo:` prefix.
- The native-shell example's `ApplicationView.swift` hangs with no URL; a hung
  Swift test is bounded only by the 20-minute job timeout.
- `hotreload.BuildHost` has no callers; the Windows prerender-control path has
  never run; `.gitignore` carries agent-local `.cache/*` entries and the dead
  `/ephemeral/inspiration` line.
- `native-plugin-tests.yml` path filters miss `internal/pluginbuild/**` and
  `cmd/skgo-plugin-build/**`.
- Stale comments in `internal/gen/emit.go`, `internal/gen/scan.go`, `prerender.js`.
- `internal/gen/entries.go` emits inline `import()` types in the TypeScript
  branch, which Tyler said he dislikes; the JSDoc branch needs them.
- README links #176, closed as not planned; the "not supported" row is true.

## Corrections deliberately not folded

- "Tests protect supported capabilities, not a catalogue of one-off mistakes":
  Tyler removed it from AGENTS.md in #408; not re-added.
- A returned command or Monitor is not a finished check; keep full output and
  exit status before filtering (four incidents, gimbal#428): Gimbal-side
  defect, and AGENTS.md is held at 47 lines.
- Kit's query-cache GC rule (rediscovered three times) is stated in
  `example/e2e/features/remote.feature`, where the next agent meets it.

## Proof

`go vet` and `go test -count=1` on `.` (Refresh/CSP), `internal/remotearg`,
`internal/formdata`, `internal/toolchain`; `just --list` parses; the e2e
passthrough was checked in a replica recipe (default mode, quoted `--grep`,
JSON reporter, bad mode);
`check-doc-indexes.py --repo .` PASS (no docs index in this repo; `docs/` is
the website). Kit facts read from a 3.1.0 install in a sibling worktree; this
worktree has no `node_modules`. Independent review: three bugs (a gopls
claim, this marker's grep, the dev-server handoff) and four design findings,
all repaired or recorded above. Round two: a `paths.origin` prerender fallback,
three guide pages claiming the scaffold sets `paths.origin` (`differences.md`,
`migrating.md`, `glossary.md`), and the README route meeting the fence rules;
repaired. Left as nits: `SVELTEKIT_COMPATIBILITY.md` 3.0.0 entry still cites
local logs and gitignored screenshots; `example/web/vite.config.ts` comment
says Kit fixes the origin at build time unconditionally.
