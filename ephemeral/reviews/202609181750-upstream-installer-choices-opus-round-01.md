# Adversarial review — upstream installer choices, round 01

## Target

Commit `ad3eaf3` (`feat: keep sv's installer choices in skgo new`) on
`codex/upstream-installer-choices`, complete diff, against `AGENTS.md`/`CLAUDE.md`
and the generator contract in
`ephemeral/worklog/202609181700-upstream-installer-choices.md`.

Scope note: the launch prompt's outcome list and proof summary were treated as
claims to test, not as review scope or a safe-area declaration. The "material
findings with concrete failing inputs" and five-finding limits were honoured as
report limits only.

## Evidence inspected

- Full diff of `cmd/skgo/main.go`, `internal/newapp/newapp.go`,
  `internal/newapp/newapp_test.go`, `internal/sv/sv-addon.source.js`, README
  files; the surrounding `Create`/`finish`/`verifyFrontend`/`realRunner` code.
- Pinned upstream: `sv@1.0.0-next.7` `packages/sv/src/cli/create.ts`,
  `addons/vitest-addon.ts`, `addons/storybook.ts`; `vite-plus@0.3.0`
  `packages/cli/src/create/templates/remote.ts`.
- `go test ./internal/newapp/` — passes.
- Live runs of a binary built from this commit (`--sv-addon file:…/internal/sv
  --skgo-replace <checkout>`, stdin `/dev/null`, vp 0.3.1, pnpm 12.4.2):
  1. `skgo new unit -- --add vitest=usages:unit` → **fails, exit 1** (finding 1).
  2. `skgo new notypes -- --template demo --no-types` → succeeds; in the
     result `just test` ran 2 files / 2 tests green and `just build` produced
     `bin/notypes`. A combination the author did not list as proven; it holds.
  3. `skgo new tw -- --add tailwindcss` (the worklog's documented trap) → exit 1,
     no project (nitpick A).
- The author's proof directories under `/private/tmp` hold agent event logs
  only; no screenshots or transcripts of the four claimed runs were available to
  look at, so those runs were not independently confirmed beyond run 2 above.

## Findings

### 1. issue — a developer's unit-only Vitest selection makes `skgo new` fail after everything is installed

Requirement missed: "compatible choices survive" and the worklog constraint that
supported upstream type/add-on options "must either work end to end or fail
explicitly without being advertised as completed".

Reproduction (run live, above):

```
skgo new unit -- --add vitest=usages:unit
…
Error: ERR_PNPM_RECURSIVE_EXEC_FIRST_FAIL
  × Command "playwright" not found
skgo: installing the browser required by Vitest failed: pnpm exited with status 1
exit=1
```

The same happens in a terminal when the developer picks Vitest in sv's add-on
list and leaves only "unit testing" ticked at sv's own
"What do you want to use vitest for?" question — i.e. through the interactive
surface this commit exists to expose.

Cause: `inspectChoices` (`internal/newapp/newapp.go`, `c.vitest = …
"vitest" != "" && Scripts["test:unit"] != ""`) correctly treats the selection as
made and does not reapply Vitest. But `finish` still runs
`pnpm --dir web exec playwright install chromium` unconditionally
(`newapp.go`, "installing the browser required by Vitest"). Upstream
`vitest-addon.ts:37-41` adds `playwright` / `@vitest/browser-playwright` only
when `usages` includes `component`, so with unit-only there is no `playwright`
binary. The failure arrives after vp create, sv add, create-storybook and two
installs, and leaves a populated half-project with no Go files; the message
blames a browser install rather than the choice.

The unit test hides it: `TestCreatePassesExplicitChoicesThroughAndAppliesNothingTwice`
(`newapp_test.go:210`) passes exactly `vitest=usages:unit` and asserts success,
because the fake runner accepts `playwright install` for a project whose
`devDeps` never contained `playwright`. The test therefore certifies the input
that fails against real upstream — the expectation was not anchored in what
upstream actually writes.

Either outcome the contract allows is absent: the browser install is not
conditional on `playwright` being a devDependency (so the choice would survive),
nor is unit-only refused up front in `svCreateArgs`/`inspectChoices`.

## Nitpicks

- **A. Misleading failure when sv stops at an unanswered add-on option.**
  `skgo new tw -- --add tailwindcss` without a terminal: sv prints the plugins
  question, reaches EOF, vp exits 0, and skgo reports
  `VitePlus did not produce web/package.json: open …: no such file or directory`
  — no `skgo:` prefix, no mention that a named add-on's option was left
  unanswered, although the worklog records this exact trap and the README
  example was changed for it. It does fail explicitly, hence not material.
  `skgo new x -- --help` lands on the same message.
- **B. Late refusals leave a populated target.** A `library` pick at sv's
  interactive template question, or a drizzle/paraglide/better-auth pick, is
  refused only by `inspectChoices` after vp has created and installed `web/`;
  the retry the message asks for ("create it again without them") then trips
  `emptyDir` until the developer deletes the directory by hand.
- **C. The add-on overwrites `scripts.test` wholesale**
  (`sv-addon.source.js`, `data.scripts.test = 'pnpm run test:unit --run'`). With
  `--add playwright`, sv's chained `… && npm run test:e2e` is dropped from
  `test`; `test:e2e` itself survives.
- **D. Not from this commit, observed while reproducing:** two concurrent
  `skgo new --sv-addon file:…` runs share
  `~/Library/Caches/skgo/sv-addon-qualification` and one died with
  `ERR_PNPM_LOCKFILE_RENAME_FILE`. Qualification-only path.

## Outcome

material findings remain
