# Signed release updater review, round 01

Outcome: no findings

Target: the current updater changes in `internal/update/update.go`, new `release.go`/`release_test.go`, and the adjusted `update_test.go`/`release_age_test.go`. Reviewed against the user's explicit requirement that `skgo update` download the latest actual published GitHub release instead of rebuilding the installed CLI with `go install`, while preserving existing toolchain/project alignment and cached Go-tool behavior. Repository instructions and the agent-protocol/adversarial-review skills apply. No caller scope narrowing was supplied. Review remained read-only except this report and normal test scratch; no secrets or signing operations were used.

## Findings

No material findings or genuine nitpicks remain.

The installed-CLI update path now uses GitHub's `/repos/tylergannon/skgo/releases/latest` response instead of Go module `@latest`, rejects draft/prerelease/nonstable responses, selects the platform's named release archive, and verifies it against the release's `checksums.txt`. Those names match the current release command/workflow contract for `darwin_arm64`, `linux_amd64`, and `linux_arm64`.

The downloader copies the executable bytes out of the verified archive, stages on the destination filesystem, and leaves replacement to `Run` after the downloaded executable reports the selected release version. Replacement uses a rename, preserving the release executable's bytes rather than invoking a compiler or signer. Download/version failures leave the previous installed executable unchanged and clean staging files.

Existing completion behavior is retained: the newly installed executable owns toolchain/project alignment; existing root, VP, and native options are forwarded. Cached and temporary Go-tool executables still route the standalone install into Go's configured installation directory rather than overwriting the cache. Project dependency alignment and subsequent project Go-tool verification continue through their existing paths. The installed-CLI path contains neither `go install` nor a Go module version query.

## Load-bearing test assessment

- The successful HTTP fixture test supplies literal executable bytes, requires those exact bytes and executable permissions in the staged output, and checks the previous installation remains unchanged before verification/replacement.
- Corrupt content, a missing archive executable, missing release assets/checksums, draft metadata, and unavailable release metadata must produce errors while preserving the old executable and leaving no staging debris.
- The wrong-version test requires refusal before replacement or completion and checks both preservation of the installed bytes and removal of the rejected staged executable.
- The standalone/cached invocation test requires use of the injected release download and staged version probe, fails on unexpected commands (including the former `go install` or module query), checks ownership of completion by the installed executable, and checks preservation of cached executable bytes. Its standalone branch also positively checks replacement with the supplied release bytes.
- Existing completion tests still cover exact VP alignment, wrong completion-version refusal, preserved frontend source/configuration, selected-adapter guards, temporary/cache Go-tool routing, retained executable discovery after first compilation, and native completion failure reporting.

These assertions are anchored in supplied fixtures and concrete command/file outcomes; a no-op update cannot satisfy the positive replacement/completion assertions.

## Verification

Ran from `example/web` through its declared VP environment:

```text
vp exec go -C ../.. test -count=1 ./internal/update
vp exec go -C ../.. test -race -count=1 ./internal/update
vp exec go -C ../.. vet ./internal/update
```

All passed, with no skipped-test result. `git diff --check` also passed.

This verifies the updater's HTTP/fixture and alignment contracts. This review did not modify an installed user CLI, fetch live release assets, or run a public-network update against a published GitHub release. Those runtime delivery outcomes are not claimed here.
