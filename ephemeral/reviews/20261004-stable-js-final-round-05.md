# Adversarial review — stable JavaScript release, final round 05

Reviewed on 2026-10-04 UTC in
`/Users/tyler/Codex/2026-10-02/task-18/skgo-js-dependencies`. Read
`AGENTS.md`, the cached exact `agent-protocol-SKILL.md` and
`adversarial-review-SKILL.md`, and the pinned `sveltekit-current/SKILL.md`.
No delegation. Read-only except for this artifact; ordinary focused tests used
temporary directories. No implementation, documentation, configuration or
other active worktree was edited, and no delivery action was taken.

## Target and scope

PR [237](https://github.com/tylergannon/skgo/pull/237), head
`f572a58369807cc798adcd95e1e85f2fc1c0dd23`, against released main
`bfea45a9dc5be011f9e4f37204f3b37eea350fde`. The merge base is exactly that
released main. The tree was clean before the review.

Before the artifact was finalized, the parent clarified the journal and
worklog in the working tree. I read those two prose-only diffs independently;
they distinguish the failed saved local logs from successful reported retry
streams and cite f572a58's successful CI. Implementation remains f572a58.

The authoritative goal is released stable Kit 3 and all feasible current
stable JavaScript dependencies across the repository, example, generated
applications and locks; justified retained versions and peer reconciliation;
behavior-preserving migrations; HTTP owned by Go, SSR in Goja, Node at build
time, and the native private-field performance fix retained. Delivery is an
authorized green-CI squash merge through the existing release workflow,
expected to produce the pre-1.0 feature release v0.16.0.

I considered core gaps affecting the separately owned adoption measurements.
Ownership instructions limit edits, not review subject matter; no area was
declared safe or excluded because another owner handles it. The parent also
reported separate site issues 105/106/107 and core issue 223 during this
review. Their implications and evidence limits are recorded below, without
duplicating their patches.

## Evidence inspected and independently verified

- Read the relevant released-main diff: all changed source, tests,
  manifests, generator defaults, workflow/toolchain configuration, README,
  compatibility journal, worklog and prior review rounds. Inspected each
  lock's importer, package and snapshot changes, treating the manager and
  application YAML documents separately. Inspected the distributed add-on's
  new bootstrap refusal, Vitest alignment and Storybook rule against source.
- Rechecked the official npm registry directly with read-only network access.
  Current selections agree: Kit 3.0.0, Svelte 5.57.1, Vite plugin 7.3.1,
  VitePlus/core 1.0.0, svelte-check 4.7.6, Playwright 1.63.0,
  playwright-bdd 9.2.1, sv 1.0.1, sv-utils 1.0.0, tsdown 0.23.0,
  Storybook 10.6.1, vitest-browser-svelte 3.1.0 and pnpm 12.9.1.
  [Kit's official release](https://github.com/sveltejs/kit/releases/tag/%40sveltejs%2Fkit%403.0.0)
  also identifies the stable release, published October 1.
- Retained versions have real constraints. Kit peers on TypeScript `^6.0.0`
  and svelte-check accepts 5/6, so 6.0.3 is the latest compatible stable
  despite latest 7.0.2. Node 24's latest matching types are 24.19.1 despite
  latest major 26. VitePlus 1.0.0 declares the Vite core alias at 1.0.0 and
  Vitest/browser-playwright at 5.0.1; standalone Vitest 5.0.3 is not a
  drop-in replacement for this shared family. Storybook's current optional
  peer still excludes VitePlus 1.0.0; the exception is exact and its real
  build evidence exists. Sources are the respective
  [Kit](https://registry.npmjs.org/@sveltejs/kit),
  [VitePlus](https://registry.npmjs.org/vite-plus),
  [svelte-check](https://registry.npmjs.org/svelte-check),
  [Storybook](https://registry.npmjs.org/storybook) and
  [pnpm](https://registry.npmjs.org/pnpm) registry metadata.
- Investigated inherited prereleases rather than equating stable direct
  packages with an entirely stable transitive graph. `@polka/url` next.29
  satisfies sirv 3.0.2's `^1.0.0-next.24`; its stable latest 0.5.0 does not.
  `get-tsconfig` beta.6 is an exact dependency of rolldown-plugin-dts 0.28.6
  under current tsdown; stable latest 4.14.3 is outside that dependency.
  devalue 5.9.4 is the latest compatible 5.x and Kit requires `^5.9.4`,
  despite standalone latest 6.0.2. These are upstream constraints, not stale
  direct pins that should be overridden blindly.
- Downloaded the exact official Kit next.28 and 3.0.0 tarballs into memory.
  Every installed stable Kit file matched the official stable tarball.
  Compared the upstream change inventory and relevant server/client sources:
  redirects, fetch, cookies, actions, CSP, load diagnostics, rendering,
  remote forms/commands/prerenders, manifest generation and adapter wrapper.
  Independently confirmed the journal's four byte-identical files:
  remote-functions, data/index, csrf and generate_manifest/index.
- Kit's `relative_pathname` has exactly the two arms now shared by the Go
  handlers. Kit's builder emits the exact `create_server(manifest)` wrapper
  accepted by the adapter. The extraction fixture has an eager server module
  that throws, lazy nodes, and literal manifest data; it rejects unknown
  wrappers rather than evaluating them. The two CORS literals match the
  stable diagnostic source.
- `env.js`, `engine_transform_test.go`, `document.go` and `example/ssr_test.go`
  are unchanged from released base bfea45a. Inspected syntax-based lowering,
  native-private-field retention and the independent 32 MiB ceiling after
  500 real SSR documents. The historical performance fix remains in the
  released base, including its later large-array traversal repair.
- Ran bounded independent focused regressions with `GOMAXPROCS=2
  GOFLAGS=-p=2`, `-count=1`: manifest extraction and engine lowering;
  based-root/colon/prerendered trailing-slash handler tests; and the real
  bootstrap runner's qualified/unqualified version test. Complete successful
  package outputs were:

  ```text
  ok  github.com/tylergannon/skgo/internal/adapter  1.093s
  ok  github.com/tylergannon/skgo                   0.347s
  ok  github.com/tylergannon/skgo/internal/newapp   0.596s
  ```

- Read saved exact-head CI `ci-f572a58.log`: checkout of the full reviewed
  SHA, clean dependency installation with pnpm 12.9.1, canonical `just build`,
  `just vet`, and all 21 test-bearing packages passing. Independently queried
  [CI run 37168579712](https://github.com/tylergannon/skgo/actions/runs/37168579712):
  `headSha` is f572a58, status completed, conclusion success. PR237 is
  mergeable/CLEAN. The duplicate same-repository PR-event job is intentionally
  skipped by the unchanged workflow condition; the push job actually ran.
  The only Go `t.Skip` in source is a Windows-specific formatter fixture and
  is inapplicable on this macOS host and the Ubuntu runner.
- Independently checked PR237's current title, `feat: adopt stable SvelteKit
  3 and qualify current tooling`, and its explicit consumer-floor paragraph.
  `go run ./cmd/skgo-release-version` returned **v0.16.0**. The previous
  patch-classification finding is resolved.
- Both application dependency documents are byte-identical to 4dbcc11;
  later manager synchronization changed their separate manager documents.
  Final code changes after the prior runtime/browser proof are the example
  package `dev`/`build` entries, now `vp dev`/`vp build`.
- Read `final-dev-bdd.log` and `final-prod-bdd.log`: 174 passed each, no skips,
  including source-edit behavior. Read generated TypeScript/JSDoc checks,
  unit/Chromium tests and builds: zero check errors/warnings, 2 tests each;
  the same JSDoc app's Storybook build and peer check passed. Read bootstrap
  negative-control logs and the prior independent mutation reviews. These
  earlier runs establish unchanged application behavior; I did not relabel
  them as browser runs on f572a58 or repeat whole suites merely for commits.
- Opened both final local production screenshots myself. Home shows the
  signed-in literal `Tyler final local 2026-10-03` and full capability grid.
  Actions shows that literal input and Go receipt, enabled submission and
  the sibling Go endpoint result. Neither has a blank region or error
  boundary. Capture provenance and no-page-error assertions are recorded in
  `local-final-browser-inspection.log`.

## Findings

No material findings remain. These two nitpicks do not block the focused
dependency release.

### 1. nitpick — enhanced-form readiness waits on a sibling click handler

`example/e2e/steps/dev.ts:202-204` waits for the delegated handler on
`go-dev-endpoint-button` and then clicks the `enhanced-go-dev-form` submit
button. `go-dev/+page.svelte:10-12,38-44` attaches enhancement through a form
action while the endpoint uses a separate delegated click handler. Their
shared component makes this pass today, and all source-edit scenarios passed,
but the sibling's handler is an indirect readiness signal that can stop being
valid after component changes. Wait for the actual form enhancement or prove
the signal's ordering where the fixture is maintained.

### 2. nitpick — the new executable fixture assumes a Unix shell

`internal/newapp/newapp_test.go:716-725` creates an extensionless `#!/bin/sh`
executable and invokes it directly without a platform guard. It passes on
the reviewed macOS/Ubuntu environments. Windows cannot run this fixture as
written; the existing formatter shell fixture already acknowledges that
constraint in `internal/gen/format_test.go:12-13`. This is test portability,
not evidence of a current release failure.

The proof-record concern raised during this review is resolved by the
parent's prose clarification, inspected before completion.
`package-build-20261004.log` contains a successful build;
`package-dev-20261004.log` contains the initial socket EPERM failure and
`just-test-20261004.log` contains initial permission failures ending in
`FAIL`. The journal now says the successful local retries exist in tool
streams rather than those saved files, and cites exact-head clean-install CI
37168579712 as the saved final Go/build evidence. The on-disk `@vite/client`
response supports the local core version but is not a test transcript. No
failed capture has been counted as a passing run in this review.

## Adoption and release limits

The repository's passing scenarios prove their literal behavior, not complete
native Kit parity or a 104/104 external measurement score. The existing
once-navigation count assertions, source-edit dependency ordering and absence
of the named `TestPrerenderGoLoadFailureNamesAuthoredRoute` test were inspected;
this PR does not change them. The parent reports a native-Kit experiment
challenging a once-navigation assertion, but supplied no exact native pin or
experiment artifact for independent verification. I therefore do not infer
either a proved native parity result or a demonstrated dependency regression
from that report. Separate issues 105/106/107 and core 223 remain adoption
work and should remain visible in that owner's results.

The main release workflow still must qualify both browser modes on the merged
SHA before package publication and tagging. This review verifies readiness
for that existing process; it does not claim that v0.16.0 has been published.
No implementation or environment blocker remains for this review.

## Outcome

only nitpicks remain
