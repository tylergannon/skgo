# Adversarial review: Kit exact pin shared by adapter and development server (round 01)

Date: 2026-10-16 (local)
Reviewer: Claude Fable 5.1, read-only except this file

## Target

- Issue: tylergannon/skgo#339 "fix(dev): published adapter accepts Kit 3.0.1 as peer but refuses to start".
- Caller requirement restated: the development server and the published adapter must share the same exact supported SvelteKit version.
- Branch `codex/issue-339-kit-pin` in worktree `/Users/tyler/.codex/worktrees/skgo-339-kit-pin`.
- The work was uncommitted when the review began and was committed mid-review by the implementing session as `37eea27 fix(dev): share the adapter exact SvelteKit pin`. The review covers that commit's content (identical to the working tree I read):
  - `internal/adapter/package.json` peer `@sveltejs/kit` `^3.0.0` -> `3.0.0`
  - `internal/adapter/package_test.go` asserts the exact peer pin
  - `internal/adapter/skgo-adapter/dev.js` reads the peer pin from its own `package.json` and gates on it, with a new migration message
  - `internal/adapter/development_version_test.go` (new) rejection matrix
  - `ephemeral/worklog/kit-development-pin.md` (new) one-line decision

Caller narrowing: none that limits subject matter. "Do not edit any other files" and the artifact path are operating constraints and were honored.

## Evidence inspected

- Issue #339 body (no comments).
- `git diff` of the five changed files; full `dev.js`; `package.json`; `development_version_test.go`; `package_test.go` helpers `pack`/`packedAdapter`; `kit_queue_compatibility_test.go` helpers and metadata assertions.
- Other Kit version sources: `internal/kitpatch/kitpatch.go` (lines 26, 100-130, 407-426), `cmd/skgo/kit_patch.go`, `internal/newapp/newapp.go:991`, `internal/update/complete.go:108,137` + `frontend.go:64`, `internal/advice/catalog.go:17`, `internal/adapter/skgo-adapter/compat/kit-3.0.0-queue.json`, `example/web/package.json`, `example/web/pnpm-workspace.yaml`.
- Documentation: `SVELTEKIT_COMPATIBILITY.md` lines 40-100, `README.md:14`, `docs/index.html:29`.
- `vp add --help` from the example's installed VitePlus (confirms `-D` and `--save-exact` exist).
- Positive-path tests that exercise `development()` with the real Kit: `internal/adapter/development_test.go` (`TestDevelopmentSnapshotUsesLiveKitGraphAtGoHandler`, no skip path) and `internal/devserver/session_integration_test.go`.

Checks run:

```
cd example/web && vp exec go -C ../.. test -count=1 -v \
  -run 'TestDevelopmentEnforcesPublishedKitPin|TestThePublishedPackageDeclaresWhatAnAdapterDeclares' ./internal/adapter
--- PASS: TestDevelopmentEnforcesPublishedKitPin (0.90s)   (4/4 subtests)
--- PASS: TestThePublishedPackageDeclaresWhatAnAdapterDeclares (0.00s)
ok   github.com/tylergannon/skgo/internal/adapter 1.121s

go vet ./internal/adapter   -> clean
```

Manual positive probe (scratchpad only, no repo edits): imported the linked `skgo-adapter/dev.js` from `example/web` with `SKGO_DEV_DIR`/`SKGO_DEV_ORIGIN` set and called `development({config:{paths:{},alias:{},files:{},env:{}}})` against the installed Kit 3.0.0. Result: `ACCEPTED ['plugins']`. The gate admits the qualified version and rejects 3.0.1, 2.55.0 and 3.0.0-next.28 with the exact message.

Load-bearing analysis of the new test:
- The `{pin: "3.0.1", installed: "3.0.0"}` case rewrites the packed `package.json` peer and expects the gate to follow it. A `dev.js` that kept the literal `"3.0.0"` fails this case (message would say `@sveltejs/kit@3.0.0` and the case expects `3.0.1`, plus the gate would not throw at all). A `dev.js` that read the wrong field (undefined) fails every case on message mismatch. So the "shared pin" claim is mechanically enforced.
- The test runs against the packed tarball, so it also proves `package.json` is shipped at `../package.json` relative to `skgo-adapter/dev.js` in the published layout.
- Gap: see finding 1.

## Findings

### 1. issue: the focused test proves only rejection; acceptance of the pinned version is proved only by the slow integration test

Evidence: `internal/adapter/development_version_test.go:17-23` has four cases, all expected to throw. There is no `{pin: "3.0.0", installed: "3.0.0"}` case asserting `development()` returns. A mutation such as `if (true) throw ...` or comparing against the wrong installed field while keeping the message template passes all four subtests.

Impact: the acceptance half of the contract is covered only by `TestDevelopmentSnapshotUsesLiveKitGraphAtGoHandler` (`development_test.go:28`), which boots Vite, Kit and a native app. That test does run under `just test` and has no skip path, so the suite as a whole would catch an always-reject regression, but in the seconds-tier test that exists specifically for the gate the positive case is missing. The repo rule is "assert at the granularity the behaviour deserves"; a gate deserves both sides in the same cheap test, and the positive subtest would cost one more ~0.2s node invocation using the fixture already built. My manual probe above is exactly that case and passes today.

### 2. issue: the migration diagnostic points at a raw `vp add` rather than the project's own exact-pin migration, and the issue explicitly asks the downgrade to preserve the consumer's patch behaviour

Evidence: `dev.js:43-44` tells the consumer to `Run vp add -D --save-exact @sveltejs/kit@<pin>.` The issue says the downgrade "must preserve any consumer patch behavior". skgo already ships that migration: `go tool skgo kit-patch --apply` (`cmd/skgo/kit_patch.go:15`) pins Kit exactly across all dependency sections (`kitpatch.go:223-253`), configures the `patchedDependencies` entry keyed `@sveltejs/kit@3.0.0`, and `--check` verifies the installed queue bytes (`kitpatch.go:407-426`). `newapp.go:991` and `update/complete.go:108` use that path too.

Concrete failure: a consumer who arrived at 3.0.1 through `^3.0.0` and never had the queue patch configured follows the message, gets Kit 3.0.0 without the prerender queue correction, starts cleanly, and hits the unpatched queue defect that `kit-patch` exists to prevent. A consumer whose Kit lives in `dependencies` has it moved to `devDependencies` by `-D`. A consumer using a pnpm `catalog:` reference gets the catalog bypassed. None of these are hypothetical shapes; `kitpatch_test.go:428,517` already test manifests of those shapes. The message should name `go tool skgo kit-patch --apply`, then the frontend install, then `--check` (or at minimum mention the patch), which is the migration the issue asked to be "accurate".

### 3. nitpick: the compatibility journal still publishes the old peer contract, and the commit type does not signal a narrowed published contract

Evidence: `SVELTEKIT_COMPATIBILITY.md:54` "the adapter now requires stable Kit `^3.0.0`" and the dependency table at `:100` "SvelteKit / adapter peer | `3.0.0` / `^3.0.0`". The journal's own preamble (`:40-41`) says publication requires updating the status rather than assuming it happened. The commit is `fix(dev): ...`; the earlier widening to `^3.0.0` was deliberately a `feat` minor bump for a consumer-visible dependency contract change (`:57-58`). Tightening a published peer range is at least as consumer-visible as widening it. Not blocking, but the journal is the one place consumers are told what the adapter requires, and it is now wrong.

### 4. nitpick: the "one supported Kit version" is still several agreeing literals, and only the development path has a runtime gate

Evidence: the exact version lives independently in `internal/adapter/package.json` peer, `skgo-adapter/compat/kit-3.0.0-queue.json` `version`, `internal/advice/catalog.go:17`, `kitpatch.go:26,129`, and the test literals in `package_test.go:160` and `kit_queue_compatibility_test.go:22`. Each is asserted literally, so drift fails some test, but no assertion ties the peer pin to the queue metadata version, which is what `kit-patch --check` enforces at install time. A one-line test (`pkg.PeerDependencies["@sveltejs/kit"] == metadata.Version`) would make the claim mechanical. Separately, `skgo-adapter.js` (production `adapt`) has no installed-version check; with pnpm's default non-strict peers, a production build against Kit 3.0.1 proceeds with an install warning only. That is pre-existing and outside the issue's dev-server scope, but it is the remaining place where "shares the exact version" is a declaration rather than an enforced check.

## Observations (not findings against this change)

- Mid-review the working tree briefly showed a 206-line modification to `example/internal/skgo/params/skgo_gen.go` that was not present at review start and is not in commit `37eea27`; it had disappeared by the time the commit landed. Nothing in this change touches generation. Noted so the implementer can confirm it was a transient regeneration and not a stale committed generator output on `main`.
- The error still surfaces through Kit's wrapper (`Failed to load SvelteKit options from Vite config`) and Go's `Vite exited before adapter readiness` (`internal/devserver/frontend.go:66`). The skgo line is printed first and is specific, which satisfies the issue; a Go-side preflight before spawning Vite was not requested and is not required.
- The decision recorded in the worklog (refuse 3.0.1 rather than qualify it) matches the issue's instruction not to drop the guard without qualifying 3.0.1. No 3.0.1 qualification was claimed, correctly.

## Outcome

material findings remain
