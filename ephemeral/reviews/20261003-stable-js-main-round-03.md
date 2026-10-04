# Adversarial review — stable JavaScript dependencies on current main, round 03

Reviewed 2026-10-03 08:23–08:40 PDT in worktree
`/Users/tyler/Codex/2026-10-02/task-18/skgo-js-dependencies`, under the cached
`ephemeral/js-dependencies/logs/agent-protocol-SKILL.md` and
`adversarial-review-SKILL.md` (re-read this round; unchanged, 27 and 57
lines). Read-only except for this file. Operating constraints honoured: local
delivery authority only (no push, PR, dispatch, merge or publication was
performed or needed), bounded validation concurrency (`GOMAXPROCS=2
GOFLAGS=-p=2`), tool output kept under the transport limit, images read one at
a time. The launch prompt contained no scope narrowing to refuse; `--no-doppler`
treated as an operating flag.

## Review target

Branch `codex/stable-js-dependencies` at `5d3e1a0` ("require the qualified
VitePlus bootstrap for new projects", 08:22), four commits over main `bfea45a`
(PR 236). The two dependency commits (`49ce349`, `56405c4`) are unchanged in
content since round 01; `97d8b3d` and `5d3e1a0` add the journal restructure,
the round-01/02 reviews, the VitePlus bootstrap prerequisite and its tests.
Working tree content-clean; no orchestrator process alive; no remote branch
contains `HEAD` and `gh pr list --head codex/stable-js-dependencies` is empty.

Goal measured against: current feasible stable JS dependencies including
released Kit 3; behaviour-preserving migrations; HTTP/server logic in Go and
SSR in goja; native private fields preserved; validated delivery on current
main (now bounded to a local reviewable update); the journal's own maintenance
procedure; CLAUDE.md's evidence rules.

## Evidence inspected

- `git diff 97d8b3d HEAD` in full: `README.md:217-219`;
  `internal/newapp/newapp.go:85-91, 176, 536-548` (the `vp --version` probe);
  `internal/newapp/newapp_test.go:151-153, 710-744`;
  `internal/sv/sv-addon.source.js:120-122`; `sv-addon.js`; journal; worklog;
  `registry.json` additions (storybook, addon-svelte-csf, enhanced-img,
  playwright, browser-playwright).
- Real `vp --version` output for both installed VitePlus versions via mise:
  `vp v1.0.0` and `vp v0.3.3` — the format the probe and the test fixture
  assume.
- Add-on dist rebuilt from source into my scratchpad with the package's own
  tsdown command: byte-identical to the tracked `sv-addon.js`, which contains
  the new refusal string once.
- Orchestrator logs on `bfea45a`: `final-prod-bdd` (10-02 20:08, 174 passed,
  1.2m), `final-svelte-check` (0/0), `local-final-prod-build-fresh` (07:59),
  `local-addon-build` (08:03), `local-bootstrap-test`, `local-final-go-test`
  (08:09, 21 ok), `local-final-vet` (08:10, empty), `local-old-vp-rejection`
  (real CLI refused `vp v0.3.3` before creating anything),
  `local-bootstrap-independent-{positive,side-effect-negative,wiring-negative}`
  (the two mutations fail the new regressions),
  `local-qualified-{ts,jsdoc}-{create,check,test,peers}` (0/0, 2/2, clean),
  `local-browser-inspection-{path,selector}-error` and
  `local-final-browser-inspection` (binary SHA, SSR-bundle SHA, versions, UTC
  timestamps, page errors `[]`).
- Generated apps `generated-local-qualified-{ts,jsdoc}`: `vitest 5.0.1`,
  `vitest-browser-svelte 3.1.0`, `@sveltejs/kit ^3.0.0`, installed
  `vite-plus 1.0.0`.
- Screenshots: `stable-js-final-local-{home,actions}.png` (08:17) differ in
  bytes from every earlier capture. I viewed both: Home shows "Signed in as
  Tyler final local 2026-10-03" with the full capability grid rendered;
  Actions shows the enhanced remote form holding that literal, the receipt
  "Remote Go form received Tyler final local 2026-10-03", the submit button
  enabled, the sibling endpoint answered, and no error boundary or blank
  region. This is a second pair of eyes on the pictures, as CLAUDE.md asks.
- My own run at `HEAD`: `GOMAXPROCS=2 GOFLAGS=-p=2 just test` — all 21
  packages in both modules ok, exit 0, no skips, on the quiet tree with the
  07:59 build (newer than every generated stub).
- CI runners: every workflow is `ubuntu-latest`.

## Status of earlier rounds

- Round 01 F1/F2 and round 02 finding 1 (journal baseline/“in progress”): the
  journal now has one Status paragraph naming `bfea45a`, one Baseline
  paragraph, and one validation section citing logs on that base; no "in
  progress" remains; external delivery is stated as paused. Resolved.
- Round 02 finding 2 (unpinned bootstrap, exact-version exception): `skgo
  new` now probes `vp --version` before `vp create` and refuses anything but
  `vp v1.0.0`; the add-on refuses any other installed VitePlus; README names
  the version; a real 0.3.3 rejection and mutation controls are on record.
  Resolved as a policy choice (see nitpick 1 for its cost).
- Round 02 finding 3 (byte-identical screenshots): new captures with
  provenance and a distinctive literal; independently viewed here. Resolved.
- Round 02 nitpick 4 (ledger-shaped journal): the three narratives were
  collapsed into one section with a short history line. Resolved.
- Round 01 F9 correction stands (`Justfile:22` exports `.tools/bin` first).

## Findings

No material findings remain. The nitpicks below are genuine but none changes
a contract, a wire byte, or the evidence that the update works.

### 1. nitpick — the exact bootstrap gate will stop `skgo new` on VitePlus's first patch, and README gives no install recipe

`newapp.go:544`: `fields[1] != "v"+c.VitePlusVersion` with `"1.0.0"`;
`sv-addon.source.js:120`: `vitePlus.version !== '1.0.0'`. A developer who
runs `npm i -g vite-plus` after VitePlus 1.0.1 ships gets a clear refusal and
must downgrade; the journal says this is deliberate ("prevents … a newer
unqualified combination"). That is coherent with "never claim anything that
hasn't been demonstrated running", but it makes every upstream patch a skgo
release, and `README.md:217-219` names **1.0.0** without saying how to get
exactly that (`npm i -g vite-plus@1.0.0`, or the `mise` pin the example uses).
One sentence closes the second half; the first is a policy to revisit when
the next VitePlus lands.

### 2. nitpick — the final Go suite the journal cites ran 13 minutes before the commit it vouches for

`local-final-go-test.log` and `local-final-vet.log` are 08:09–08:10;
`5d3e1a0` was committed 08:22, after `local-qualified-*` (08:11–08:19). My
`just test` at `HEAD` (21 ok) closes the gap, and `git diff` shows the commit
holds exactly the probe, test, add-on, README and journal changes those logs
exercised, but the journal should cite a run at the committed head or name
the head it ran on.

### 3. nitpick — the new bootstrap test has no platform guard

`newapp_test.go:716-720` writes a `#!/bin/sh` fixture and executes it. CI is
`ubuntu-latest` only and the suite already treats a skip as a failure, so
there is no present impact; a Windows developer would see a confusing
failure rather than the Windows-only skip the retention tests use.

### 4. nitpick — `dev.ts` still waits on a different element than it clicks (carried)

`example/e2e/steps/dev.ts:202-204` waits for `go-dev-endpoint-button`'s
delegated handler and clicks the `enhanced-go-dev-form` button. The journal
now documents the Svelte-symbol dependency ("same component … not a general
form-readiness API") but not the element mismatch. Works because the page
hydrates as one component; the wait should name the element it protects.

## Verified this round (no action)

- The `vp --version` probe runs before any project directory is written
  (`realRunner` checks `VitePlusVersion` before `exec.Command(c.Name,
  c.Args...)`); the side-effect mutation log proves a project is not created
  on refusal; the wiring mutation log proves `Create` passes the prerequisite.
- The add-on's refusal applies only when `node_modules/vite-plus` exists, so
  plain Vite Kit projects are unaffected, as the journal states.
- `vitest-browser-svelte` 3.1.0 peers on `vitest ^4 || ^5`; with the gate in
  place the Vitest 4 path is unreachable through `skgo new` anyway.
- Round-01 verifications stand unchanged: `relative_pathname` mirroring
  (`./seg/` on add, `../seg` on removal, literal `./foo:bar/?q=1` tests);
  four Kit files byte-identical to next.28; `create_server` wrapper matches
  Kit's `builder.js:218-240` template; CORS literals match
  `server-errors.js:194-196`; no runtime JavaScript added; `env.js` lowering
  untouched and `TestEngineLoweringUsesSyntaxRatherThanComments` passing;
  lockfile integrity equals the registry entry; CI/pnpm/mise pins consistent.
- Delivery authority: nothing pushed, no PR, no workflow dispatched for this
  head; the journal says so.

## Outcome

only nitpicks remain
