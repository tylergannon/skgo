# Adversarial review: milestone 4, "Go signatures generate usable Swift APIs" (round 05)

## Review target

- Checkout: `/private/tmp/skgo-native-ci-repair`, branch `codex/zig-native-foundation`, HEAD `9db4f48` ("build: install example dependencies through vp"), PR #287, merge-base with `main` `89c09f4`.
- Scope: everything since `70af58c` for milestone 4 of `ephemeral/plans/zig-native-client.md` (lines 241-259), judged against CLAUDE.md, the pinned Kit 3.0.0 source under `example/web/node_modules/@sveltejs/kit`, the plan's shared finite numeric contract, and the repository's CI gates.
- Caller narrowing: none. The launch prompt only set read-only boundaries, the artifact path and scratch-copy mutation checks.
- Change since round 04: one commit, `9db4f48`, touching only `Justfile` (the `install` recipe now runs `mise x -- vp install` in `example/web` and `example/e2e` instead of `pnpm install`) and one worklog line. No Go, Swift, Zig, generator or test source changed since round 03 apart from the `devalue/v5` import paths noted there.

## Evidence inspected

- CI for HEAD (run 37768291757): Linux job "Build, tests, and Conventional Commit" pass in 6m34s with every step successful, including "HEAD uses a Conventional Commit subject", `just install`, build, native compiler install, vet and tests; package lines `ok internal/gen 176.056s`, `ok internal/swiftgen 0.007s`, `ok native 129.610s`. macOS job "Swift client and generated production HTTP calls" pass in 3m21s: `ok native 112.583s`, `ok internal/swiftgen 9.953s`, `ok internal/gen 19.369s`. Neither log contains a `SKIP` line. PR `mergeStateStatus` is `CLEAN`.
- `go run ./cmd/skgo-release-version --check HEAD` on this checkout: exit 0 (the round-04 failure is gone because HEAD is no longer the merge commit).
- Local on HEAD: `go test -count=1 ./native ./internal/swiftgen` ok (9.4s, 1.0s); `go test -count=1 ./internal/gen -run 'TestNative|TestGeneratedSwift|TestWebGrammar'` ok (10.1s). `mise x -- vp install --frozen-lockfile` in `example/web` and `example/e2e` both report "Lockfile is up to date", so the new recipe's lockfiles are in sync and the working tree stayed clean (verified with `git status`).
- `vp` resolves through mise to `example/web/node_modules/.bin/vp` (v1.0.0, pnpm 12.9.1 underneath), matching the `_.path` arrangement in `example/mise.toml` and the zero-npm rule.
- Skip guards: the only `t.Skip` under `native`, `internal/gen`, `internal/swiftgen` is the pre-existing `internal/gen/format_test.go:14` (fixture formatter), unrelated to this milestone. Swift-dependent tests are gated by `_darwin_test.go` filenames and `//go:build` tags, and the macOS job runs them.
- Round-04 scratch mutations stand (Zig suite proven load-bearing by a control mutation; the `errdefer` deletion still survives, see nitpick 2). Round 01-03 generator/runtime mutation results stand because those files are unchanged.

## Findings

No material findings remain. The round-04 issue (red Linux CI with all tests skipped because HEAD's subject failed the Conventional Commit gate) is resolved: HEAD now passes the gate and both CI jobs ran the full proof and passed.

### 1. Nitpick: a non-conventional merge commit remains in the branch history

`9d16418` ("Merge current runtime dependency into native foundation") is still an ancestor of HEAD. The CI gate only inspects HEAD, and a squash-merge of PR #287 will drop it, so nothing fails today. It does contradict the repository's rebase-not-merge convention, and any future push whose HEAD is a merge commit will go red again exactly as in round 04.

### 2. Nitpick: unreachable `errdefer` in `decodeBuffers`

`native/core/src/abi.zig:133` frees `value` if duplicating `message` fails, but `core.receive` (`native/core/src/root.zig:150-197`) never yields a response with both a non-undefined value and a message. Deleting the line in a scratch copy leaves `zig build test` green. Dead code, not a leak.

### 3. Nitpick: unexercised branches carried from round 03

`internal/gen/swift.go:174` (pointer-nil guard) and `swift.go:211` (union `default`) remain uncovered by any test.

### 4. Nitpick: untracked review material in the worktree

`ephemeral/reviewer-logs/` and `ephemeral/reviews/20261008-native-api-round-04.md` are untracked. `ephemeral/` is tracked working material, so these should be committed (or `reviewer-logs/` ignored) before the PR is squash-merged.

## Outcome

only nitpicks remain
