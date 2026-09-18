# Adversarial review — upstream generator (`skgo new` via VitePlus + `@skgo/sv`)

Round 01 · 2026-09-18 14:05 local · branch `codex/upstream-generator-implementation`
Range reviewed: `528c14c..b09726f` (5 commits, 29 files, +2847/−31)

## Review target

- Requirements brief: `/private/tmp/skgo-upstream-generator-research/ephemeral/skgo-new-requirements.md`
- Repository instructions: `CLAUDE.md`, `ephemeral/worklog/202609181319-upstream-generator.md`
- Authoritative package decision restated by the caller: the native add-on ships as a
  separately published `@skgo/sv`; `@skgo/sveltekit-adapter` stays runtime-only; the first
  `@skgo/sv` publication is manual and precedes any GitHub Actions trusted publishing.
  Nothing was published during this review.

No caller instruction narrowed the defect classes, files, or subject matter, so no narrowing
was ignored. The read-only boundary, the single-artifact path, and the no-publish constraint
were honoured: the only file written is this one. Two throwaway servers and one browser were
started against the supplied proof projects and then stopped; nothing under
`/private/tmp/skgo-generated-*` or `/private/tmp/skgo-generator-proof-split` was modified.

## Evidence inspected

Source: `internal/newapp/newapp.go`, `internal/newapp/newapp_test.go`, all of
`internal/newapp/gofiles/**`, `internal/sv/{package.json,sv-addon.source.js,sv-addon.js,package_test.go,README.md}`,
`cmd/skgo/main.go`, `cmd/skgo-adapter-changed/main.go`, `.github/workflows/release.yml`,
`internal/gen/{gen.go,emit.go}`, `internal/adapter/package_test.go`, `README.md`.

Supplied proof: every file under `/private/tmp/skgo-generator-proof-split/{logs,review,bin,screenshots}`,
and the generated projects `/private/tmp/skgo-generated-examples-accepted` and
`/private/tmp/skgo-generated-minimal-final`.

Checks I ran myself:

| Check | Result |
| --- | --- |
| `go build ./... && go test ./internal/newapp ./internal/sv ./internal/gen` (repo) | pass |
| `./bin/skgo-examples` (generated binary), `GET /` | 200; SSR markup contains `data-testid="go-message">This value came from a Go query.` and `write-count">0` |
| Browser (`playwright` from the generated project) drives the write interaction | `write-count` 0 → 1, no page or console errors |
| `pnpm storybook --host 127.0.0.1 --port 6007 --ci` in the examples project | Storybook 10.6.0 ready; `index.json` lists 8 stories |
| Screenshots of `example-button--primary`, `example-header--logged-in`, `example-page--logged-in` — **opened and looked at** | Real rendered components (indigo "Button", Acme header with "Welcome, Jane Doe!", full "Pages in Storybook" article). No blank preview, no error boundary. |
| Simulated fresh clone of the minimal project (everything the generated `.gitignore` keeps) | `go build ./...`, `go vet ./...`, `go test ./...` **all fail** — see finding 1 |
| `GET https://registry.npmjs.org/@skgo%2fsv` | `404` (never published) |
| `GET https://registry.npmjs.org/@skgo%2fsveltekit-adapter` | latest published `0.3.7`, while git tags reach `v0.4.1` |

The product itself is in better shape than the proof package suggests: the generated app is
served by Go, the write path is answered by Go, and Storybook genuinely renders. The findings
below are about a real compile-time defect, about the acceptance evidence the brief makes
mandatory, and about the release/first-publish mechanics the caller flagged.

## Findings

### 1. critical — a generated project that is committed and cloned does not compile

`internal/newapp/gofiles/dot-gitignore:2` writes `/web/build/` into the generated project's
root `.gitignore`. `internal/newapp/gofiles/web/dist.go:6` is `//go:embed all:build`, and the
only thing that makes that embed resolvable before the first frontend build is
`internal/newapp/gofiles/web/build/placeholder` — which lives inside the ignored directory.
So the generator writes a file whose entire purpose is to make the Go module compile and then
tells git never to keep it.

Reproduction (a clone is exactly the generated tree minus `bin/`, `node_modules/`,
`.svelte-kit/` and `web/build/`):

```
$ rsync -a --exclude node_modules --exclude .svelte-kit --exclude web/build --exclude bin \
    /private/tmp/skgo-generated-minimal-final/ "$SCRATCH/clone/"
$ cd "$SCRATCH/clone"
$ go build ./...
web/dist.go:6:12: pattern all:build: no matching files found
$ go vet ./...
web/dist.go:6:12: pattern all:build: no matching files found
$ go test ./...
# example.com/skgominimal/cmd
web/dist.go:6:12: pattern all:build: no matching files found
FAIL    example.com/skgominimal/cmd [setup failed]
FAIL    example.com/skgominimal/web [setup failed]
FAIL
$ mkdir -p web/build && : > web/build/placeholder && go build ./...    # recovers
```

Impact: the first thing a Go developer does in a checked-out project — `go build`, `go test`,
or simply opening it so gopls loads the packages — fails with an error that names an embed
pattern and gives no hint that the fix is to run a frontend build. `just build` happens to
survive because `go generate ./...` does not type-check and `vp build` then materialises
`web/build`, but only after `pnpm install`. This is squarely against the brief's outcome
("no manual configuration edits, dependency repairs, or undisclosed setup steps") and against
the DoD scenario *The generated application builds and runs independently*.

The worklog records this exact class of failure already —
"Go's embed package excludes dot-prefixed files … a new app failed to compile before its
first frontend build -> use a non-hidden placeholder that the generated root .gitignore still
excludes." The fix landed for the generator's own output and the same hazard was left in place
for every clone. `!/web/build/placeholder` (or not ignoring the directory) closes it.

### 2. critical — the verification the brief makes mandatory was not produced

The brief is explicit: *"Verification must include inspected browser screenshots of the
running app and rendered Storybook components, plus Vitest output showing actual executed
tests. An independent validator checks that the scenarios detect broken behavior and runs
them. A green exit code, installed dependencies, or the Storybook navigation shell alone does
not establish completion."* `CLAUDE.md` says the same in stronger terms.

What the proof package actually contains:

- `/private/tmp/skgo-generator-proof-split/screenshots/` — **empty**. Zero bytes of browser
  evidence for either starter.
- `/private/tmp/skgo-generator-proof-split/review/agent-review.log` — **0 bytes**. The
  independent-validator record is an empty file.
- `logs/` (29 files) — contains no Storybook log of any kind, for either starter. The
  Storybook scenario shipped with no artifact whatsoever.

What is there is good as far as it goes: `*-vitest-green-before/after.txt` show 2 files / 2
tests executed, `*-vitest-deliberate-red.exit` is `1`, `*-check.txt` shows `svelte-check found
0 errors`, and `*-production-process.txt` shows the binary running under `env -i` with no
child processes — that last one is a genuinely well-chosen, non-derived assertion for
"serving application requests does not require Node".

I ran the two unevidenced scenarios myself and both pass (table above; I opened all three
Storybook screenshots and the two app screenshots and they show real rendered content). So
this is a delivery gap, not a product defect — but under this repository's rules an exit code
plus an empty screenshot directory is precisely the artifact that is supposed to be treated as
unmet acceptance, and a 0-byte validator log reported as a pass is the failure mode the rule
exists to catch.

### 3. issue — the Storybook scenario is not load-bearing, and Storybook compiles components with a different Svelte config than the app

Two problems, one root.

**The stories are not the project's.** Both starters ship exactly
`src/stories/{Button,Header,Page}.stories.svelte` — create-storybook's own framework
templates, along with `Button.svelte`, `Header.svelte`, `Page.svelte` and their CSS. Nothing
skgo generates or configures appears in a story. Remove `@skgo/sv` entirely, swap the adapter
back to `adapter-auto`, and all eight stories still render identically. The scenario
*Storybook displays the generated components* therefore cannot fail for any skgo reason, which
is the load-bearing test the repository instructions prescribe. The generator's own gate,
`verifyFrontend` (`internal/newapp/newapp.go:387`–`425`), only checks that `.storybook/main.*`
exists, that some `*.stories.*` file exists, and that `node_modules/.bin/storybook` is present
(`newapp.go:423`) — files and installed dependencies, which the brief names explicitly as
insufficient.

**Storybook does not get the app's Svelte configuration.** Kit 3 has no `svelte.config.js`;
the generated `web/vite.config.ts` passes the settings inline to `sveltekit({ compilerOptions:
{ runes: …, experimental: { async: true } }, experimental: { remoteFunctions: true } })`.
Storybook's own `vite-plugin-svelte` instance does not see them — its startup log emits

```
[vite-plugin-svelte] no Svelte config found at .../web - using default configuration.   (×4)
```

which does **not** appear in the app's own `vp build`
(`logs/examples-accepted-production-build.txt`, 0 occurrences). The three stock stories are
plain Svelte and are unaffected, which is why this is invisible today. But the one component
skgo actually authors — the examples `+page.svelte`, which uses template `await` and therefore
requires `compilerOptions.experimental.async` — could not be storied under that configuration,
and no check would notice. A scenario whose only subjects are the installer's own boilerplate
cannot surface that.

### 4. issue — an unpinned second installer run under a blanket npm force flag

`internal/newapp/newapp.go:151` invokes
`pnpm dlx --allow-build esbuild create-storybook@latest …` with
`Env: append(os.Environ(), "CI=1", "npm_config_force=true")` (`newapp.go:157`).

*Unpinned.* The brief requires the `sv` installer be resolved from registry metadata with an
explicit bound ("Go selects the highest published `sv` 1.x version … No manually maintained
installer release pointer"), and `resolve` does that correctly (`newapp.go:281`–`287`,
constrained to `v1`). The second installer gets no bound at all. A `create-storybook` major
release changes or breaks every `skgo new` invocation with no skgo release and no way to
reproduce yesterday's generation. The brief's "Storybook setup belongs to their upstream
installers" says *whose* installer, not that its version should float.

*Blanket force.* `npm_config_force=true` is npm's global `--force`, not a targeted engine-check
downgrade. Applied to the whole installer subprocess tree it also suppresses cache-integrity
refusals, overwrite protection and confirmations. It is load-bearing today only for one
read-only lookup, which the generated project's `web/debug-storybook.log` shows actually
happening:

```
[13:51:46.086] [DEBUG] Executing command: npm config get registry --workspaces=false --include-workspace-root
```

The brief says "Upstream internal npm calls are permitted when needed", so the *call* is
authorised and I am not relitigating it — but the standing repository guidance on this exact
`npm config get registry` case says the remedy for a tool that calls npm is that the tool is
out, and it specifically rejects loosening the guard. Since the brief overrides that, the
remaining finding is the mechanism: a narrowly scoped variable, or writing the registry value
create-storybook is looking for, would achieve the same result without handing `--force` to an
unpinned `@latest` installer.

### 5. issue — the manual first `@skgo/sv` publish has no recorded procedure, and the obvious one permanently poisons version selection

`internal/sv/package.json` carries `"version": "0.0.0-dev"`, and
`cmd/skgo-adapter-changed/main.go:263`–`272` (`sameManifest`) deletes the `version` key before
comparing the tree against the published tarball. Consequences of the natural manual first
publish (`cd internal/sv && npm publish --access public`):

1. `@skgo/sv@0.0.0-dev` becomes `latest`.
2. On the next release, `sv-npm`'s changed-check compares the rebuilt tree to that tarball.
   The files are identical and `version` is ignored, so it prints `unchanged` and
   `.github/workflows/release.yml:178`–`181` skips publishing.
3. `skgo new`'s selector (`newapp.go:289`, `compatible := semver.Compare(v, skgoVersion) <= 0`)
   keeps resolving `@skgo/sv` to `0.0.0-dev` for every release until the add-on's *content*
   next changes.

Registry state confirms both halves of the mechanism: `@skgo/sv` is `404`, and publish-only-when-changed
is already demonstrably in effect for the adapter — latest published `0.3.7` against tags
reaching `v0.4.1`. The pairing contract the worklog rests on ("selecting each package's newest
exact version not newer than the running skgo release gives a deterministic compatible pair")
silently degrades to "always the dev stub" for an unbounded window.

The correct manual sequence is
`go run ./cmd/skgo-adapter-changed -dir internal/sv -package @skgo/sv -stamp v0.4.1` and then
publish, so that the first published version participates in the ≤-selection the design
assumes. Nothing in the branch — README, `internal/sv/README.md`, workflow comment or worklog —
records this, and the caller's constraint arrived only in conversation.

Secondary, same area: `tag` now `needs: [version, adapter-npm, sv-npm]`
(`release.yml:185`) and the two publish jobs run in parallel. If `sv-npm` fails on the first
tagged release because trusted publishing is not yet configured for a package that does not
exist, `adapter-npm` may already have published while the Go tag is never created.

## Nitpicks

- **The examples demo asks a question it never answers.** `record` builds
  `Go recorded a greeting for %s.` (`internal/newapp/gofiles/examples/web/src/routes/example.remote.go.tmpl:35`),
  but the page calls `record(name).updates(status())` and renders only `status()`, so the
  command's return value is discarded. Verified in the browser: after submitting "Adversarial
  Reviewer", `go-message` stayed `This value came from a Go query.` and only the counter moved.
  The form's label is "Who should Go greet?" and the greeting never appears, which makes the
  most legible piece of Go-derived output dead code.
- **No freshness check on a committed 756 KB build artifact.** `internal/sv/sv-addon.js` is
  tsdown output committed beside `sv-addon.source.js`, and no test relates them. They are in
  sync right now (I grepped seven distinctive source strings through the bundle), but
  `internal/sv/package_test.go` packs the *committed* file while `release.yml` runs
  `pnpm run build` first and publishes a *rebuilt* one. Local qualification via
  `--sv-addon file:internal/sv` runs whichever stale copy is on disk.
- **`url.QueryEscape` / `decodeURIComponent` disagree on `+`.** `newapp.go:124`–`126` builds
  `starter:…+adapter:…+name:…`, escaping `adapter` and `name` with `QueryEscape`; the add-on
  decodes only `adapter` (`sv-addon.source.js`, `decodeURIComponent(options.adapter)`). A
  `--adapter file:/path with spaces` yields a literal `+`, which both fails to decode and
  collides with the option separator. Harmless for registry versions and for the `[A-Za-z0-9._-]`
  app names the regex allows, but the asymmetry is a trap.
- **The supplied "fresh" projects are not pristine.** Both `+page.svelte` files carry a
  hand-added `<p data-testid="hmr-proof">Frontend edit visible through VitePlus HMR.</p>` from
  the HMR check, and `/private/tmp/skgo-generated-examples-accepted/web/build` was rebuilt from
  that edited source. It is visible in every screenshot of the running app. Fine as HMR
  evidence; it does mean neither directory shows what a developer receives.

## Outcome

`material findings remain`
