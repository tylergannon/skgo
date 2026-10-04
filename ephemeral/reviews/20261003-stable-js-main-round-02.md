# Adversarial review — stable JavaScript dependencies on current main, round 02

Reviewed 2026-10-02 20:01–20:10 PDT (2026-10-03 UTC) in worktree
`/Users/tyler/Codex/2026-10-02/task-18/skgo-js-dependencies`, under
`ephemeral/js-dependencies/logs/agent-protocol-SKILL.md` and
`adversarial-review-SKILL.md`. Read-only except for this file. The launch
prompt contained no scope narrowing to refuse; `--no-doppler` was treated as an
operating flag.

## Review target

Branch `codex/stable-js-dependencies` at `97d8b3d`, three commits over main
`bfea45a` (PR 236, "reuse compiler dependency exports during read-only
checks", 19:49): `49ce349` stage, `56405c4` qualify, `97d8b3d` docs
(journal, worklog, round-01 review). The two code commits are content-identical
to round 01's `796451d`/`f1d0618` — `git diff f1d0618 56405c4` is exactly
main's own `bfea45a` delta. Working tree: content-clean apart from an
uncommitted edit to `SVELTEKIT_COMPATIBILITY.md` by the primary session; an
orchestrator was running final qualification in this worktree concurrently
(generated apps 19:57, dev BDD to 20:02, production build 20:05).

Goal measured against: current feasible stable JS dependencies including
released Kit 3; behaviour-preserving migrations; HTTP/server logic in Go and
SSR in goja; native private fields preserved; validated delivery on current
main; the journal's own maintenance procedure.

## Evidence inspected

- Full diff `main...HEAD` (code, config, e2e, docs) and `git show 97d8b3d`;
  `git show bfea45a -- internal/gen/scan.go`.
- Installed Kit 3.0.0 `src/core/adapt/builder.js:218-240` (server-instance
  template) against the new `readKitManifest` regexes; next.28 source under
  `/tmp/skgo-stable-js-source-task18/kit-next28` (round-01 checks stand).
- `internal/newapp/newapp.go:160-176, 783-795`; `internal/sv/sv-addon.source.js`;
  `README.md:215-230`; `Justfile:20-22`.
- Orchestrator logs: `main-go-test` (19:46, 21 ok), `main-vet`, `main-prod-bdd`
  (174 passed), `main-generator-ts` (19:43, global `vp` 0.3.3),
  `final-generated-ts-*`, `final-generated-jsdoc-*` (19:57), `final-dev-bdd`
  (20:02, 174 passed, 4.5m), `final-prod-build` (20:05).
- Generated apps on disk: `generated-main-vp1-ts`, `generated-main-vp1-jsdoc`
  (package.json, pnpm-workspace.yaml, installed svelte/kit/vite-plus) and
  `generated-main-stable-ts` (`pnpm peers check`, read-only).
- `ephemeral/screenshots/*` byte comparison.
- My own run: `just test` (through `just`, so `.tools/bin` first on PATH) on
  the quiet tree after the 20:05 production build, 20:06–20:08 — all 21
  packages in both modules ok, exit 0, including the build-freshness guard,
  production SSR retention and `internal/adapter`.

## Corrections to round 01

- **F9 was wrong.** `Justfile:22` is `export PATH := justfile_directory() /
  ".tools/bin" + ":" + env("PATH")`, so `just test` does supply the matching
  Staticcheck. My failure came from invoking `go test` directly. The worklog's
  friction entry is correct; this round's `just test` was run through `just`.
- F3 (two different apps behind one Storybook sentence) is fixed in the journal
  as committed in `97d8b3d`.
- F1/F2 (stale baseline, unrecorded post-rebase evidence) are resolved for
  `49cc2ea` by `97d8b3d` and the `main-*` logs; the same gap now exists one
  base later (finding 1).
- F4's "rot vector" is no longer hypothetical (finding 2).

## Findings

### 1. issue — "validated delivery on current main" is not yet true or recorded for `bfea45a`

*Incomplete requirement.* The committed journal (`97d8b3d`,
`SVELTEKIT_COMPATIBILITY.md:158-159`) ends its Delivery integration section
with "Qualification on final main `bfea45a` … **in progress**"; the
uncommitted edit adds the generated-app results and still says "Complete final
main `bfea45a` browser/Go qualification … **in progress**". Observed on
`bfea45a` during this review: generated TS and JSDoc apps — `svelte-check` 0/0,
Vitest 2/2 each, Storybook build and `pnpm peers check` clean; dev BDD 174
passed (4.5m); production build at 20:05. Not yet observed or recorded on
`bfea45a`: production BDD, `just vet`, a `just test` by the delivering session
(mine is below), any screenshot. A journal that lands on main saying
"in progress" is a stale claim the moment it lands. Impact: the authoritative
goal's last clause is unmet until these are run on the final head and the
journal names that head.

### 2. issue — `skgo new` does not ensure the qualified toolchain, and the compatibility exception covers one exact VitePlus version

*Incorrect implementation of "current feasible stable JS dependencies" for
generated projects.* Evidence:

- `internal/newapp/newapp.go:160-162`: `vp := options.VP; if vp == "" { vp =
  "vp" }` — project creation runs whatever `vp` is on the user's PATH; nothing
  checks its version. `README.md:217` names VitePlus with no version.
- `internal/sv/sv-addon.source.js`: the Storybook peer exception is added only
  when `vitePlus?.version === '1.0.0'`.
- Reproduction, from the orchestrator's own machine where `vp --version` is
  `v0.3.3` (the version skgo itself pinned in `example/mise.toml` until this
  branch): `main-generator-ts.log` (19:43) created a project with
  `vite-plus 0.3.3`, `vitest 4.1.11`, `@vitest/browser-playwright 4.1.11`, and
  pnpm printed `[WARN] Issues with peer dependencies found`. In
  `generated-main-stable-ts/web`, `pnpm peers check` reports: `✕ unmet peer
  vite-plus — Installed: 0.3.3, Wanted: "^0.1.15 || ^0.2.0" by
  storybook@10.6.1`.

So a developer following the README today gets neither the Vitest 5 family
this update qualified nor the exception that silences Storybook's stale peer,
and the uncommitted journal line "An older global `vp` can select an older
managed test family; it is not evidence for this upgraded toolchain" records
the condition without closing it. Impact: the generated-app half of the
dependency goal holds only on a machine that already has VitePlus 1.0.0.
Minimal closure: `skgo new` refuses or warns on `vp` < 1.0.0 (it already
refuses missing executables at `newapp.go:790-794`), README names the version,
and the exception keys on the qualified range with a loud message outside it.
(Checked and discarded: `vitest-browser-svelte` 3.1.0 peers on `vitest ^4.0.0
|| ^5.0.0`, so its unconditional pin does not break the Vitest 4 case.)

### 3. issue — the "new post-rebase screenshots" are byte-identical to the pre-rebase ones

*Proof defect.* `cmp ephemeral/screenshots/stable-js-main-home.png
stable-js-production-home.png` and the `-actions.png` pair: identical bytes
(19:49 vs 19:24). The journal (`97d8b3d`, Delivery integration) says "The
primary agent inspected new post-rebase Home and Actions screenshots".
Deterministic headless rendering can reproduce a PNG exactly, but identical
bytes are also precisely what a copy produces, so this evidence cannot
distinguish a second look from none. CLAUDE.md: "open the box" and "never hand
a human a picture you have not looked at". No capture exists for `bfea45a` at
all. Closure: capture with a distinguishing in-frame fact (the served
revision or adapter identity the dev server already logs, or a visible
timestamp) and record the capture command and time next to the log names.

### 4. nitpick — the journal is turning into a run-by-run ledger

`SVELTEKIT_COMPATIBILITY.md` now carries three validation narratives (initial
`21573e1`, integration `49cc2ea`, final `bfea45a`), each listing log names and
timings. The journal was requested and its procedure asks for honest records,
but CLAUDE.md's "Build the software … not ledgers … evidence manifests" and
"Don't narrate" apply to its shape: at adoption it should state what was
validated at the final head, with one line of baseline history, and drop the
intermediate rounds.

### 5. nitpick — e2e fixture coupling (carried from round 01)

`example/e2e/steps/fixtures.ts` `interactive()` depends on Svelte's internal
`Symbol('events')` (`svelte/src/internal/client/dom/elements/events.js:19,149`
in 5.57.1); `steps/dev.ts:202-204` waits on `go-dev-endpoint-button` and then
clicks the `enhanced-go-dev-form` button. Both fail loudly; low impact.

## Verified this round (no action)

- `bfea45a` changes only read-only dependency loading in `internal/gen`
  (`preserveDependencyExports` around `packages.Load`); the dev session's
  regeneration on the combined tree returned every committed
  `skgo_remotes_gen.go`/`types.ts` content-identical, so the merged generator
  emits what the branch committed.
- Kit's `generateServerInstance` template is exactly `import { create_server }
  …; const manifest = …; export const server = create_server(manifest);`,
  matching all three regex checks in `readKitManifest`; the production builds
  on `49cc2ea` and `bfea45a` prove the real output passed them.
- Generated apps on `bfea45a` hold Kit 3.0.0, Svelte 5.57.1, VitePlus 1.0.0,
  Vitest 5.0.1, browser-svelte 3.1.0 and the Storybook exception, with no
  skips in their tests.
- Round-01 verifications (redirect mirroring, byte-identical Kit files, CORS
  literal, reproducible `sv-addon.js`, Go-owned HTTP / goja SSR / untouched
  private-field lowering) stand; the code is unchanged.

## Outcome

material findings remain
