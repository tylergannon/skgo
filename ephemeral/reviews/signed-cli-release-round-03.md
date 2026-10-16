# Signed CLI release review, round 03

Outcome: no findings

Target: commit `48f1acf`, specifically the release smoke-test directory creation in `cmd/skgo-release/main.go`, reviewed against the actual versioned CLI `v0.34.0` at `4a761be2fa6ea0b6ea8ab1963141a5950c5f7aea`. The requested operating boundary prohibited credential fetching, Apple signing, and notarization. No caller scope narrowing was supplied. Review remained read-only apart from this report and the command's automatically cleaned temporary build/cache activity.

## Finding assessment

No material findings or genuine nitpicks remain.

The CLI deliberately treats a nonexistent explicitly configured plugin directory as a discovery error. The old smoke test supplied such a directory. The correction creates the private empty directory before passing it through `SKGO_PLUGIN_DIRS`, propagates creation failures, and leaves existing deferred cleanup responsible for removal. It repairs the release driver's invalid smoke-test input without changing plugin discovery semantics or suppressing a real CLI error.

## Concrete validation

Ran the fixed driver against the published source tag with every signing variable explicitly absent:

```text
env -u MACOS_SIGNING_IDENTITY -u MACOS_NOTARY_PROFILE -u MACOS_KEYCHAIN_PATH \
  GOMAXPROCS=4 GOFLAGS=-p=4 \
  go run ./cmd/skgo-release build v0.34.0 4a761be2fa6ea0b6ea8ab1963141a5950c5f7aea
```

The actual command downloaded and built the tagged module, checked the downloaded origin against the supplied SHA, and printed the real executable's build information identifying `skgo v0.34.0`, `go1.27.2`, `darwin/arm64`, `CGO_ENABLED=1`, and `-trimpath=true`. It then printed the complete CLI help, including the available built-in native-app add-on. Local retained-installation hints were printed as warnings; they did not fail the smoke invocation.

Only after the successful help command did the driver stop with `MACOS_SIGNING_IDENTITY is required` and exit status 1. This is the requested successful boundary check: the next credential-requiring stage was reached and refused before any signing operation. No credentials were fetched, and no Apple signing or notary submission occurred. The observed temporary release staging directory was absent after the command returned, confirming its cleanup on this refusal path.

`go test -count=1 ./cmd/skgo-release` and `git diff --check` also passed.

This proves the corrected directory creation permits the real versioned CLI to complete its buildinfo/help smoke on this Mac. It does not claim a signed archive, plugin qualification from a signed archive, or a completed release; those still require the authorized Actions run.
