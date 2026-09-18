# Adversarial review — upstream generator (`skgo new` via VitePlus + `@skgo/sv`)

Round 02 · 2026-09-18 15:00 local · branch `codex/upstream-generator-implementation`
Range reviewed: `main...e8934d5` (6 commits, 31 files, +3247/−31), clean working tree apart
from untracked `ephemeral/agent-logs/`.

## Review target

- Requirements brief: `/private/tmp/skgo-upstream-generator-research/ephemeral/skgo-new-requirements.md`
- Repository instructions: `CLAUDE.md`, `ephemeral/worklog/202609181319-upstream-generator.md`
- Caller-restated decision: the native add-on ships separately as `@skgo/sv`;
  `@skgo/sveltekit-adapter` stays runtime-only; the first `@skgo/sv` publication is manual and
  precedes GitHub Actions trusted publishing. Nothing was published during this review
  (`GET registry.npmjs.org/@skgo%2fsv` is still `404`).

No caller instruction narrowed the defects, files or subject matter, predicted a conclusion,
or asked for a verdict, so nothing was ignored. Read-only boundary honoured: this file is the
only thing written in the repository. One APFS clone of a proof project was made in the
session scratchpad, built and served there, then stopped; nothing under `/private/tmp/skgo-proof-*`,
`/private/tmp/skgo-generated-*` or `/private/tmp/skgo-generator-proof-split` was modified.

## Evidence inspected

Source: `internal/newapp/newapp.go`, `newapp_test.go`, all of `internal/newapp/gofiles/**`,
`internal/sv/{sv-addon.source.js,package.json,README.md,package_test.go}`,
`internal/adapter/skgo-adapter.js`, `cmd/skgo/main.go`, `cmd/skgo-adapter-changed/main.go`,
`.github/workflows/release.yml`, `internal/gen/{gen.go,emit.go}`, `README.md`, the worklog,
and the round-01 review.

Proof: every file under `/private/tmp/skgo-generator-proof-split/{review,screenshots}`, the
`proof-*` logs, `packages/`, and the generated project `/private/tmp/skgo-proof-examples-final`.
**All six screenshots were opened and looked at.**

Checks I ran:

| Check | Result |
| --- | --- |
| `go build ./... && go test ./internal/newapp ./internal/sv ./internal/gen ./cmd/skgo-adapter-changed` | pass |
| Scratch clone of `skgo-proof-examples-final`, documented `just build` | exit 0, **then `git status` = ` D web/build/placeholder`** — finding 1 |
| Built binary from that clone, browser drives the write with the name `Round Two + Reviewer` | `write-count` 0 → 1, `go-greeting` = `Go recorded a greeting for Round Two + Reviewer.`, no page/console errors; screenshot opened — real page, real state |
| `md5` of the two Storybook screenshots | identical (`bc59c007…`) |
| `shasum` of `review/agent-review.log` vs my round-01 review | identical |

Round-01 status: finding 1 (clone does not compile) fixed for the as-generated tree but see
finding 1 below; finding 2 (no screenshots) largely fixed; finding 4 (`@latest` +
`npm_config_force`) fixed — `create-storybook` and `@storybook/sveltekit` are pinned to
`10.6.0` and the force flag is gone (`newapp.go:111`, `159`–`172`); finding 5's procedure is now
recorded in `internal/sv/README.md` and guarded by `-require-published`; the greeting and
`+`-escaping nitpicks are fixed and I confirmed both in the browser.

## Findings

### 1. critical — the documented `just build` deletes the tracked embed placeholder, so the round-01 clone failure returns after the first build

The generator now tracks `web/build/placeholder` (`newapp.go:202`–`207`, add-on rule
`!/build/placeholder` in `sv-addon.source.js`). But the adapter that clears `web/build` on every
frontend build restores a *different* file: `internal/adapter/skgo-adapter.js:184`–`188`
writes `${out}/.gitkeep`, with a comment saying its purpose is exactly to avoid "making every
frontend build appear to delete tracked source". `.gitkeep` is ignored by the add-on's own
`/build/*` rule; `placeholder` is not restored by anything except the one-shot write inside
`Create`.

Reproduction (scratch APFS clone of `/private/tmp/skgo-proof-examples-final`, which starts
with a clean `git status`):

```
$ just build            # the README's production instruction; exit 0
$ git status --short
 D web/build/placeholder
$ ls -a web/build
. .. .gitkeep app.html client error.html index.html skgo.manifest.json ssr
```

Impact: every developer's first `just build` or `just serve` dirties the tree with a deleted
tracked file. The natural `git commit -a` / `git add -A` removes it, and the next clone is back
to round 01's `web/dist.go:6:12: pattern all:build: no matching files found` for `go build`,
`go vet`, `go test` and gopls. The proof did not catch this because the commit-and-clone check
(`logs/proof-final-commit-clone.txt`) commits the tree *immediately after generation*, and the
production binaries were built with a bare `go build ./cmd`
(`logs/proof-examples-production-build.txt`: "go build ./cmd completed successfully"), not the
project's documented `just build` — which is what the DoD scenario *builds and runs … using the
project instructions* specifies. No Go test relates the adapter's restored filename to the
generator's tracked one (`internal/sv/package_test.go:72` only greps the ignore rule). The two
halves must agree on one name; note that a fix on the adapter side changes `@skgo/sveltekit-adapter`
content and therefore the version `skgo new` must select.

### 2. issue — the brief's independent validator still does not exist; its slot holds a copy of the previous review

The brief: *"An independent validator checks that the scenarios detect broken behavior and
runs them."* `review/agent-review.log` was 0 bytes in round 01; it is now byte-identical to my
round-01 review (`shasum` equal). That document reports defects in an earlier tree — it is not
a validation of this one, and an adversarial code review is not the load-bearing scenario run
the brief and `CLAUDE.md` both describe. The only record of the scenarios being run against the
remediated tree is `review/remediation-proof.log`, written by the implementer about its own
work, including adjudicating round-01 finding 3 in its own favour. Finding 1 above is what that
gap costs: a validator executing the scenarios as written ("build … using the project
instructions") would have hit it.

### 3. issue — two screenshots do not show what the proof log says they show

- `screenshots/examples-app.png` shows `Writes handled by Go: 1`, the input still reading
  "Svelte developer", and **no greeting line**. `remediation-proof.log` claims that submitting
  "Proof Reviewer" rendered "Go recorded a greeting for Proof Reviewer." No supplied artifact
  shows that state; the image is consistent with a pre-fix build or a reload after the write.
  The behaviour itself is real — I reproduced it and looked at the result — so this is an
  evidence defect, but it is the read/write scenario's only artifact and it does not show the
  write's result.
- `examples-storybook-page.png` and `minimal-storybook-page.png` are the same bytes. Identical
  upstream stories can legitimately render identically, but a pair of files that cannot be told
  apart is not evidence that two projects were each exercised; nothing in either image ties it to
  its starter. Combined with round-01 finding 3 (stories are create-storybook boilerplate, and
  Storybook's vite-plugin-svelte does not receive the app's inline Svelte config), the Storybook
  scenario remains one that no skgo defect can fail. The worklog records a decision not to add a
  skgo story; I am not relitigating that, only recording that the consequence stands.

### 4. issue — merging this branch before the manual bootstrap wedges the release, and the README does not say which comes first

`release.yml` runs on every push to `main`. With `@skgo/sv` unpublished, `sv-npm` now fails by
design (`-require-published`, `cmd/skgo-adapter-changed/main.go` `unpublishedError`), and `tag`
`needs: [version, adapter-npm, sv-npm]`, so no Go tag is created for this merge or any later
one until someone publishes by hand. `adapter-npm` still runs in parallel and is not gated on
`sv-npm`, so a release in which the adapter *did* change publishes it irrevocably while the Go
tag it pairs with is never cut (round-01's secondary point, unchanged).

The bootstrap text (`internal/sv/README.md`, "First publication") says to stamp "the real skgo
release tag `vX.Y.Z`" — but that tag is computed by the workflow from Conventional Commits and
does not exist until the workflow that is blocked on the publication succeeds. The operator
must either predict the version and publish from the unmerged commit, or merge, accept a red
release, publish, and re-run; neither order is written down, and stamping the existing
`v0.4.1` instead publishes an add-on claiming compatibility with a skgo release whose adapter
restores the wrong placeholder (finding 1). Until that publish happens, a released `skgo new`
fails at `selecting the sv add-on` for every user.

## Nitpicks

- **Root README prerequisites are incomplete.** It says "Install VitePlus"; `Create` also
  shells out to `pnpm`, `go`, and the printed instructions require `just`, none of which is
  checked up front. A missing `just` is discovered only after several minutes of successful
  generation, at the first printed command.
- **A failed run leaves a non-empty directory that `emptyDir` then refuses** (`newapp.go:388`),
  so the retry after any upstream hiccup needs a manual `rm -rf`. The failure message is clear,
  which is what the brief requires; the retry ergonomics are not.
- **Still no freshness check between `sv-addon.source.js` and the committed 500-line bundle
  `sv-addon.js`**; `--sv-addon file:internal/sv` qualifies whichever copy is on disk while the
  workflow publishes a rebuilt one.
- **Generated `package.json` carries `"@storybook/addon-svelte-csf": "latest"`** from
  create-storybook beside otherwise pinned `10.6.0` tooling. Upstream's choice, but it is the one
  floating edge left in a generation that is otherwise reproducible.

## Outcome

`material findings remain`
