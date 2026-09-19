# Adversarial review — upstream installer choices, round 02

## Target

Branch `codex/upstream-installer-choices` through `19aee24`
(`ad3eaf3` + `428c999 fix(newapp): keep a unit-only Vitest choice` +
`19aee24 fix(newapp): describe Chromium only where it was installed`), against
`AGENTS.md`/`CLAUDE.md`, the contract in
`ephemeral/worklog/202609181700-upstream-installer-choices.md`, and round 01
(`202609181750-upstream-installer-choices-opus-round-01.md`).

Scope note: the launch prompt's statement that finding 1 is addressed and its
proof summary were treated as claims to verify, not as scope. The whole branch
was re-considered; the report limits were honoured as report limits only.

## Evidence inspected

- `git diff ad3eaf3..19aee24`: `internal/newapp/newapp.go` (`finish`),
  `internal/newapp/gofiles/README.md.tmpl`, `internal/newapp/newapp_test.go`,
  worklog additions. Pinned `sv@1.0.0-next.7` `addons/vitest-addon.ts` re-read
  for where `playwright` comes from.
- `go vet ./internal/newapp/ ./cmd/skgo` clean; `go test -count=1
  ./internal/newapp/` passes.
- Live, with a binary rebuilt from `19aee24` (`--sv-addon
  file:…/internal/sv --skgo-replace <checkout>`, stdin `/dev/null`):
  1. `skgo new unit2 -- --add vitest=usages:unit` — the round-01 failing input —
     **exit 0**; no `playwright install` was issued; generated README says only
     "installed the frontend dependencies and made an initial frontend build",
     no Chromium mention. In the result: `just test` → 1 file / 1 test passed;
     `just build` → `bin/unit2`; `./bin/unit2` answered `GET /` 200 with
     `<h1>Welcome to SvelteKit</h1>` in the document (rendered in Go, upstream
     minimal page intact).
  2. `skgo new def2` (unattended default) — exit 0; `playwright` present in
     `web/package.json`; README keeps the Chromium sentence and the fresh-clone
     `playwright install chromium` instruction; `just test` → 2 files / 2 tests
     passed (the browser-backed component test still runs).

## Round-01 finding 1 — resolved

`finish` now reads `web/package.json` after install and runs
`pnpm --dir web exec playwright install chromium` only when `playwright` is a
devDependency, which is exactly the condition under which upstream's Vitest
add-on declares it (`vitest-addon.ts:37-41`). The choice survives rather than
being refused. The README template follows the same fact.

The test weakness is also corrected at its root: the `upstream` fake now adds
Playwright only for a `component` usage and fails `playwright install` without
the dependency, as pnpm does, so
`TestCreatePassesExplicitChoicesThroughAndAppliesNothingTwice` would fail if the
install became unconditional again; `TestCreateInstallsChromiumForAChosenComponentVitest`
and the default test's command-5 assertion pin the other direction.

## Findings

No material findings remain. No new defect was found in the correction or
surfaced elsewhere on re-inspection.

## Nitpicks

Round-01 nitpicks A–D are unchanged and still nitpicks:

- A. `skgo new x -- --add tailwindcss` without a terminal (or `-- --help`) fails
  as `VitePlus did not produce web/package.json: …`, without a `skgo:` prefix or
  any hint that a named add-on's option was unanswered.
- B. Late refusals (interactive `library` pick; drizzle/paraglide/better-auth)
  leave a populated target that `emptyDir` then rejects on the advised retry.
- C. The add-on overwrites `scripts.test`, dropping sv's `&& … test:e2e` chain
  when `--add playwright` was chosen.
- D. Pre-existing: concurrent `--sv-addon file:` runs race on the shared
  `sv-addon-qualification` cache directory.

## Outcome

only nitpicks remain
