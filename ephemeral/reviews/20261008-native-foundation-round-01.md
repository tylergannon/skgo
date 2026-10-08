# Adversarial review: native foundation, round 01

Date: 2026-10-08. Worktree `codex/zig-native-foundation` at `d362ced` plus
uncommitted `native/` and `.gitignore`.

## Target

`native/core` (`build.zig`, `build.zig.zon`, `src/root.zig`, `src/cli.zig`,
`src/tests.zig`, `src/kit-arguments.json`), `native/remote_test.go`,
`native/mise.toml` and `native/mise.lock`, judged against milestone 1 of
`ephemeral/plans/zig-native-client.md`, `CLAUDE.md`, and Kit 3.0.0 as
installed at `example/web/node_modules/@sveltejs/kit`.

Caller constraints honoured: read-only except this file; artifact path as
supplied. The caller did not narrow the subject matter, so no narrowing was
ignored. `example/native/` and `ephemeral/reviewer-logs/` appeared in the
worktree while this review ran (another session is working milestone 3 here);
they are outside this round and were not inspected.

## Evidence inspected

- Pinned Kit: `src/runtime/shared.js` (`create_remote_arg_reducers`,
  `to_sorted`, `stringify_remote_arg`, `stringify_command_arg`,
  `url_friendly_base64_encode`), `client/remote-functions/command.svelte.js`,
  `client/remote-functions/query/index.js`, `client/remote-functions/shared.svelte.js`
  (`remote_request`), `server/remote-functions.js` (envelope shapes),
  `server/csrf.js` (`is_remote_forbidden`).
- skgo: `remote.go` (`Call`, `RemoteConfig`, `NewRemotes`, `ServeHTTP`,
  `serveCommand`, `notFound`), `remote_caller.go` (`matchCaller`).
- devalue Zig package: `zig/README.md` contract (enumeration order, reducer
  once-per-shared-value, exclusivity), public API in `zig/src/root.zig`.
- Checks run:
  - `mise exec -- zig build test` in `native/core` after clearing the cache:
    6/6 pass in 335 ms. `zig fmt --check src build.zig` clean.
  - `go test ./native/ -count=1 -v`: `TestZigRemoteCalls` passes in 4.3 s.
    `go vet ./native/` clean, `gofmt -l native` empty.
  - `zig fetch` of the pinned URL reproduces hash
    `devalue-0.1.0-QJ2gKoMeBAC-jGq_M30rVJvF7jT-GzcB0MwMxEWxoIVk`.
  - The archive unpacked into scratch builds and passes its own 33 tests with
    no sibling checkout (`zig build test` at the archive root).
  - All six fixture payloads decoded by hand and checked against Kit's rules:
    `__skrao` wrapping with UTF-16 key order on the query path only, integer
    index keys enumerated first, U+1F600 (surrogates D83D DE00) sorting before
    U+E000, empty payload for `undefined`, `[null]` for `null`.
  - Mutation checks, each restored byte-identical afterwards:
    - compare code points instead of UTF-16 units: Zig fixture test fails;
    - rename `__skrao`: Zig fixture test fails;
    - rename the `payload` query parameter: Zig test and Go test fail;
    - misread the HTTP-200 `error` tag: Zig test and Go test fail;
    - CLI omits the `Origin` header: Go test fails (403 at the real dispatcher).
  - GitHub state of `tylergannon/devalue`: no tags, no releases; PR #3
    ("publish an installable Zig source package") squash-merged as
    `20a8d45`; `compare/16bc1b5...main` reports `diverged`.

## Findings

### 1. Issue: the devalue pin points at an unreachable pre-squash commit

`native/core/build.zig.zon:8` pins
`https://github.com/tylergannon/devalue/archive/16bc1b502e07a6535b5cde10dda0ff1933bf3ee0.tar.gz`.
That commit is not on `main` (squash-merged as `20a8d45`), not on any branch,
and there is no tag or release. The hash is real and the archive builds and
tests standalone today, so this is functional. But the plan's milestone 1 text
asks for "the actual published URL"; a commit reachable only through a
deleted PR branch is retained by GitHub incidentally, and if it is garbage
collected every fresh build fails with no change in this repository. Root
`build.zig` and `build.zig.zon` blobs are identical between the pin and
`main` (`378174c`, `b80fdcc`), so repinning to `20a8d45` (or a tag) and
re-running `zig fetch --save` is a small change. Milestone 6's "builds do not
depend on an unrecorded local path" is only as durable as this pin.

### 2. Issue: a full copy of the devalue package sits untracked in the tree

`native/core/zig-pkg/devalue-0.1.0-QJ2gKoMeBAC-…/` is a complete copy of the
codec (sources, tests, fixtures, licences). Nothing references it: `build.zig:5`
resolves `devalue` through `b.dependency`, which the global cache satisfies, and
`.gitignore` ignores `.zig-cache/` and `zig-out/` but not `zig-pkg/`
(`git check-ignore` reports it unignored). The plan states "SKGo and
applications depend on its package rather than checking in codec copies". The
next `git add native` commits it. Delete the directory; if a vendored package
path is ever wanted, that is `--system` mode and a deliberate decision.

### 3. Issue: CI cannot run the new Go test as wired

`Justfile:57` runs `go test -count=1 ./... ./example/...`, which now includes
`./native`. `native/remote_test.go:24` execs `mise exec -- zig` from
`native/core`. `.github/workflows/ci.yml:21` runs on `ubuntu-latest` with
`jdx/mise-action` pointed at `example` (line 37-40); no workflow installs Zig,
and `native/mise.lock` records only macOS assets for `aqua:yonaskolb/XcodeGen`,
so `mise exec` under `native/` on Linux must either fail to prepare tools or
miss Zig. The test fatals instead of skipping, which is the right behaviour
under "skips are failures", but the consequence is that the first PR from this
branch goes red at `just test` unless CI gains a Zig toolchain or a documented
split. Not reproduced on a runner (no PR exists yet); the causal chain is from
the files cited.

### 4. Nitpick: `receive` is stricter than Kit's client in two harmless spots

`root.zig:186-187` rejects a 2xx `result` envelope without `data`, whereas
`remote_request` (`shared.svelte.js:134`) treats missing `data` as `{}`.
`root.zig:159-164` reads `error.message` from any non-2xx JSON body regardless
of the `type` tag, whereas Kit uses `statusText` unless `type === 'error'`.
Neither changes what skgo's handler produces. `root.zig:170-171` also carries a
dead `!success` branch since that case returned at line 158.

### 5. Nitpick: HTTP-failure and alias coverage stop short of the handler

Milestone 1 lists "HTTP failures" among the fixture coverage. Those are
exercised only at `receive` with literal 502/403 bytes (`tests.zig:49-56`);
no CLI step sees a real non-2xx from the dispatcher (the 403 origin refusal or
405 on a GET command). Mutation 5 above shows the Origin path is load-bearing
indirectly, so this is coverage shape, not a hole. Likewise no fixture carries a
shared object reference through `__skrao`; the devalue README promises reducers
run once per shared value and Kit dedupes on the original, so the clone map in
`Canonical.reduce` should match, but nothing records Kit's bytes for it.

## Things checked and found sound

- Query URL, command body, base path and app dir composition match
  `query/index.js:27` and `command.svelte.js:56-62`; `prepare` omits
  `?payload=` exactly when Kit does.
- `Canonical` mirrors `to_sorted`: plain and null-prototype objects only,
  arrays untouched, marked clones not re-wrapped, previously cloned children
  reused.
- Command path applies only the `__skrag` guard, matching
  `create_remote_arg_reducers(false)` for the finite-model profile.
- CLI sends `Origin`, posts JSON, and leaves redirects unfollowed
  (`cli.zig:27`), which keeps `is_remote_forbidden` satisfied and matches the
  plan's "commands are not followed across redirects".
- Go test expectations are literal strings and counts, not values read back
  from the code under test; the command handler checks the literal transcript
  arrived before acknowledging.
- `checkAllAllocationFailures` covers request and response ownership.
- No unrequested infrastructure: no runner, no evidence manifest, no second
  JavaScript harness; the fixture file is data.

## Operational note

Clearing `native/core/.zig-cache` and `native/core/zig-out` for an uncached
test run removed the ignored `zig-out/bin/skgo-remote` binary; it is build
output and `zig build` recreates it.

## Outcome

material findings remain
