# Signed CLI release review, round 04

Outcome: no findings

Target: commit `72ee84a`, the qualification workflow's `mise-action` PATH-export correction, and the independently downloaded signed macOS artifact from Actions run `38847058521` (`v0.34.1`, source commit `67cf123abc9524960eb94127c51776d754c6b420`). Reviewed against the original release request, repository instructions, and earlier review rounds. No caller scope narrowing was supplied. No secret reads or signing operations were performed.

## Workflow correction

No material findings or genuine nitpicks remain.

The actual failed production qualification log shows generation progressed through type projection, then `vp fmt` failed with `mise ERROR No version is set for shim: node`. The action ran in `example` with `export_path: false`; generation subsequently invoked formatting from a different directory. `example/mise.toml` declares the Node/tool pins, so relying on a directory-sensitive shim after leaving that configuration scope explains this failure.

Reviewed the official `jdx/mise-action` v4 `action.yml` and `src/index.ts` through GitHub's contents API. Its `export_path` input controls exporting the PATH additions computed by `mise env --json` in the supplied working directory, and the implementation writes those additions through `GITHUB_PATH` while retaining their order. Setting `export_path: true` therefore provides the pinned executables to subsequent processes regardless of their current directory. The existing npm release jobs already use the same setting. This corrects the observed runner-tool resolution failure without changing the frontend package-manager pins or bypassing VitePlus.

This review establishes the correction's mechanism and consistency with the failure evidence. A new full qualification run is still required to establish the complete production suite's result.

## Independent signed-artifact validation

Downloaded Actions artifact `cli-darwin-arm64` (artifact ID `12023513003`) from run `38847058521` into a temporary directory under `ephemeral`. The contained archive was `skgo_0.34.1_darwin_arm64.tar.gz`, with SHA-256:

```text
2b46364e4e199b0e9751422992e3fbcf005e9abd0535fbd8baf6fb8c5f78fac3
```

Its entries were the expected `skgo`, `LICENSE`, and `README.md`. On the extracted executable, `codesign --verify --strict --verbose=2` succeeded, reporting a valid signature satisfying its designated requirement. Signature inspection identified `Developer ID Application: Tyler Gannon (6H59L2SRR6)`, the Developer ID certification chain, a secure timestamp, the hardened runtime flag, and `com.apple.security.cs.disable-library-validation = true`.

Then ran the requested real packaged-CLI test against that downloaded signed archive:

```text
SKGO_RELEASE_ARCHIVE="$PWD/ephemeral/release-review-04.joiW6P/skgo_0.34.1_darwin_arm64.tar.gz" \
  RELEASE_VERSION=v0.34.1 GOMAXPROCS=4 GOFLAGS=-p=4 \
  go test -tags release ./cmd/skgo-release -run '^TestPackagedCLI$' -count=1 -v
```

`TestPackagedCLI` passed in 1.42 seconds. As assessed in round 02, this verifies the actual archive's release/toolchain identity, discovery and loading of an independently compiled external Go plugin into the signed executable, and exact output from invoking that plugin. This is a positive signed-artifact qualification, not merely a missing-input refusal or a source test.

The original Actions logs separately record Apple's explicit acceptance with notarization ID `1b024a23-84df-4a2c-b017-04351610b384`, followed by successful packaged-plugin tests for all three platform archives. This review did not submit anything to Apple or retrieve signing credentials. `git diff --check` also passed.

The reviewed run's qualification failed, so its npm and GitHub/Homebrew publication jobs were correctly skipped. A published Homebrew installation is not yet demonstrated by this evidence. The temporary downloaded artifact/extraction was removed after verification; the original remains in Actions.
