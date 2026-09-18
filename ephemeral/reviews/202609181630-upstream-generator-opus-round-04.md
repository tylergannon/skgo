# Adversarial review — upstream generator (`skgo new` via VitePlus + `@skgo/sv`)

Round 04 · 2026-09-18 16:30 local · branch `codex/upstream-generator-implementation`
Range reviewed: `main...193abb2` (9 commits, 32 files, +3722/−31).

## Review target

- Requirements brief: `/private/tmp/skgo-upstream-generator-research/ephemeral/skgo-new-requirements.md`
- Repository instructions: `CLAUDE.md`, the branch worklog `ephemeral/worklog/202609181319-upstream-generator.md`
- Caller-restated proof requirement: both starters through generation → commit → documented
  `just build` → commit → fresh clone → Go build/test → first `just dev` directly from the clone →
  second `just dev` without reinstall/rebuild → Vitest including a deliberate failure → Storybook →
  standalone binary without Node; and that checkout qualification does not expose tracked package
  directories to `sv`'s shared-store writes. Fresh proof under
  `/private/tmp/skgo-generator-final-validation-round-04` (index: its `README.md`).
- Operating constraints honoured: nothing published, nothing merged, no repository file written
  other than this one. `git status` at the end equals `git status` at the start
  (`?? ephemeral/agent-logs/`, `?? …round-03.md`, plus this file).

No caller instruction narrowed the defects or subject matter, predicted findings or requested a
verdict; nothing was ignored. Rounds 01–03 and their proof directories were read for context only.
I reused round 03's browser driver and shell wrappers as tooling (copied to the round-04 directory,
outside the repository); every log, screenshot and result cited below was produced in this run.

## Evidence inspected

Source: `internal/newapp/newapp.go` (whole file), `newapp_test.go` (the two new tests in full),
`gofiles/{Justfile.tmpl,README.md.tmpl,server.go.tmpl,dot-gitignore,web/dist.go}`,
`cmd/skgo/main.go` diff, root `README.md` diff, `internal/sv/package.json`, the worklog, the
`ba909e2..193abb2` delta in full (the only change since round 03), and the full `main...HEAD` stat.

### Fresh proof

Candidate CLI built from `193abb2`. Both runs used `--skgo-version v0.5.0 --skgo-replace <checkout>`
(no Go tag `v0.5.0` exists before merge) and registry-selected adapter `0.3.7`. Generation was
sequential, in this order, so that the second run is the one that used to damage the checkout:

1. **minimal — checkout qualification**: `--sv-addon file:<checkout>/internal/sv`.
2. **examples — registry selection**: `@skgo/sv@0.5.0`, resolved by the CLI.

| Step | minimal | examples |
| --- | --- | --- |
| `skgo new` | exit 0; log shows `pnpm pack` into the stage and `sv` handed `file:~/Library/Caches/skgo/sv-addon-qualification/package`, add-on reported as `v0.0.0-dev` | exit 0; `sv` resolved `@skgo/sv (v0.5.0)` from the registry |
| commit → documented `just build` → `git status` | exit 0, clean | exit 0, clean |
| second commit → `git clone` → `go build` / `go vet` / `go test ./...` | all exit 0; clone has `web/build/.gitkeep` only, no `node_modules`; status clean after | same |
| **first `just dev`, straight from the clone** (nothing installed or built beforehand; sentinels `stat` → "No such file") | recipe ran `pnpm install --frozen-lockfile` (+189 packages) and `vp build`, then served. Origin port owned by the Go `cmd` process with cwd = the clone, vite on its own port. SSR markup and DOM `h1` = "Welcome to SvelteKit"; edit marker appeared with the window sentinel intact (no reload); 7/7 checks, no console/page errors | Go query value in SSR markup; count `0` → write "Round Four + Validator" → greeting names it, count `1`, still `1` after reload; `POST /_app/remote/…` 200; HMR edit; 12/12 checks |
| **second `just dev`** | `node_modules/.modules.yaml` and `build/skgo.manifest.json` mtimes identical before and after; server log contains no install or build output (0 matching lines); page renders, 5/5 checks | same sentinels unchanged, 0 install/build lines; fresh process starts at count `0`, write → `1`; 10/10 checks |
| `git status` in the clone after each dev run | clean | clean |
| `just test` | 2 files / 2 tests passed (chromium component test + node test), exit 0 | same |
| deliberate failing test appended to `greet.spec.ts` | `1 failed \| 2 passed`, `AssertionError: expected 2 to be 3`, `just test` exit 1; green and clean after `git checkout` | same |
| zero discovered tests (both specs moved away) | "No test files found, exiting with code 1", `just test` exit 1; restored | — |
| `just storybook` | listener's cwd is the clone's `web`; `Button/Primary` renders a button labelled "Button"; `Page/Logged In` renders "Pages in Storybook" and "Jane Doe"; no error display, no console errors | same |
| clone `just build` → `./bin/<app>` from an empty cwd under `env -i PATH=/usr/bin:/bin` | Mach-O arm64, no child processes, `node` not resolvable on that PATH, "(production)"; page renders | read and write both work: greeting "Standalone Binary + Validator", count `1` |
| load-bearing check | — | Go handler mutated in the clone (message text, `writes += 2`), rebuilt: 4 checks fail, driver exit 1, screenshots show "MUTATED…" and count `2`; reverted, rebuilt, clone clean |
| failed setup | fake `vp` exiting 130 → `skgo: VitePlus project creation failed: vp exited with status 130`, exit 1, zero "created the" lines | |

Extra probe: with the cached build in place, a route added afterwards
(`web/src/routes/about/+page.svelte`) was served by `just dev` with status 200 and its heading in
the markup, so the build-once sentinel does not freeze the dev route table
(`logs/minimal-dev-newroute-*`).

**Screenshots: 17 files, 12 distinct images, every distinct image opened and looked at.** Each
shows the real page in the state claimed — the examples card with the Go message, count and
greeting (dev and binary), the edit marker under the heading in both dev apps, a rendered Button
and a rendered Page story inside the Storybook preview pane (not the navigation shell alone), and
the mutated page showing the mutated text. Identical hashes are the expected ones (minimal home in
dev-first/dev-second/binary; examples before-write across the three). Storybook images differ
between starters only in bytes; what ties each to its project is
`logs/<starter>-storybook-listener.txt`.

### Shared-store isolation (round-03 finding 2)

- Before my run, `sv`'s link `~/Library/pnpm/store/v11/links/@/sv/1.0.0-next.7/…/node_modules/node_modules/@skgo/sv`
  already pointed at `~/Library/Caches/skgo/sv-addon-qualification/package`, not at the worktree.
- After the `file:` run followed by the registry run, the link is unchanged, the **stage's**
  `package.json` now reads `"version": "0.5.0"` — i.e. the registry extraction did go through the
  link and landed in the stage — and `internal/sv` is byte-identical: all seven SHA-256 sums in
  `logs/internal-sv-before.sha256` equal `logs/internal-sv-after.sha256`, and `git status` shows no
  tracked change. Round 03 reproduced the overwrite twice with this same sequence; it does not
  reproduce now.
- The staged copy's own `sv` peer resolved to `1.0.0-next.7`, the same version the CLI selected.
- `TestCreateHandsSvAnIsolatedPackedAddonNotTheCheckout` asserts the same property with supplied
  bytes ("live checkout bytes" vs "packed bytes" vs a simulated extraction), not values read back
  from the code under test.

Disclosure: my registry run left the published `0.5.0` bytes in the user-cache stage. That is the
designed landing place; the next `file:` qualification `RemoveAll`s and repacks it
(`newapp.go:235`).

### Round-03 status

- Finding 1 (fresh-clone `just dev` fails): **fixed.** `Justfile.tmpl:9`–`21` installs and builds
  only while `node_modules/.bin/vp` / `build/skgo.manifest.json` are missing; proven above on real
  clones for both starters, first and second start, and guarded by
  `TestGeneratedDevRecipeBootstrapsAFreshCloneOnce`, which runs the real generated Justfile under
  `just` and fails (not skips) when `just` is absent. `README.md.tmpl:13`–`17` now says what a clone
  gets and names the one remaining manual step.
- Finding 2 (qualification exposes the checkout): **fixed**, as above.

Also run: `go build ./...`, `go vet`, `go test ./internal/newapp ./internal/sv ./internal/gen ./cmd/...`
— all pass (`logs/repo-go-test.txt`).

### Not provable before merge (unchanged from round 03, not a finding)

The released path with no `--skgo-replace` — `go mod tidy` resolving
`github.com/tylergannon/skgo v0.5.0` and its `tool` directive from the proxy — cannot execute until
the tag exists. Likewise a machine with no Playwright Chromium cache: this host already had one
from generation, so the clone's `just test` did not exercise the README's
`playwright install chromium` step.

## Findings

No material findings remain. Genuine nitpicks only:

1. **nitpick — a clone's `just test` / `just storybook` / `just build` still assume `just dev` ran
   first.** Only `dev` bootstraps (`Justfile.tmpl:12`–`14`); `build` calls
   `node_modules/.bin/vp` directly (`:25`) and `test`/`storybook` call `pnpm` scripts. A collaborator
   or CI job whose first command is `just build` or `just test` gets "No such file or directory".
   The README does say `just dev` performs the install, so this is disclosed, but CI rarely starts
   with `dev`.
2. **nitpick — the qualification stage is one fixed per-user path** (`newapp.go:233`) that each
   `file:` run deletes and recreates. Two concurrent qualifications, or a qualification concurrent
   with any registry-path `skgo new`, race on it. Qualification-only, and `sv`'s own shared install
   directory already makes concurrent `skgo new` unsafe (round-03 nitpick), so this adds little.
3. **nitpick — the release version still depends on an unenforced PR title.** `@skgo/sv` exists
   only at `0.5.0`; a `fix:` squash title would release `v0.4.x`, whose `skgo new` finds no add-on
   `<=` itself and fails for every user. Documented in `internal/sv/README.md` and the worklog;
   nothing checks it.
4. **nitpick — carried, unchanged:** generated `package.json` pins `"@storybook/addon-svelte-csf": "latest"`
   (upstream installer's choice); `just storybook` is fixed to port 6006 while `dev` takes
   `SKGO_DEV_PORT`; root `README.md` names only VitePlus as a prerequisite though `go`, `pnpm` and
   `just` are required; a failed run leaves a non-empty directory the retry refuses; killing
   `just dev` reports `recipe dev failed with exit code 1`, which reads like an error on a normal
   Ctrl-C-style stop.
5. **nitpick — a stale link remains in the user's pnpm store**
   (`…/@skgo/sveltekit-adapter -> <this worktree>/internal/adapter`, worklog line 37). Nothing
   extracts that name any more and `internal/adapter` was untouched by this run, but it dangles
   once the worktree is deleted. Machine state, not branch content.

## Outcome

`only nitpicks remain`
