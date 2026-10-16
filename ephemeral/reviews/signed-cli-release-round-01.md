# Signed CLI release review, round 01

Outcome: no findings

Target: the working changes in `/Users/tyler/.codex/worktrees/27bb/skgo`, including the new Go release command, signing credential handling, workflow changes, and Bob runner configuration. Reviewed against the user's request for Gimbal's signed/notarized CLI delivery and Homebrew publication, distributed Linux/macOS workloads, and required credential/setup audit. Repository instructions and the agent-protocol/adversarial-review skills apply. No caller scope narrowing was supplied.

## Evidence inspected

- The complete tracked diff and all new implementation/configuration files; surrounding release-version calculation, CLI build identity/plugin support, qualification recipes, and existing CI.
- Gimbal's `origin/main` versions of `.github/workflows/release.yml`, `scripts/ci-signing.sh`, and `scripts/sign-notarize-macos-cli.sh` from `/Users/tyler/src/gimbal`.
- GitHub read-only API checks: SKGO's `release` environment restricts deployments to `main`; repository-level `DOPPLER_TOKEN` exists; `homebrew-skgo` is public with a writable `SKGO release publisher` deploy key. Runner registration now includes online `amritanandamayi-skgo`, `bob-skgo-release-1`, and `bob-skgo-release-2` with the labels used by the workflows.
- Doppler secret-name-only responses show the six required Apple signing/notary names and `TAP_DEPLOY_KEY` in `skgo/stg`. This inspection did not read or display secret values and does not independently prove that their values or the GitHub token are usable.
- Bob's running container inventory confirms both new release runners while the existing SKGO and Gimbal containers remain present. The new runner has `sudo`, `gh`, and `zsh`, and executes as the intended runner user.

## Findings

No material findings remain in the current changes.

During review, the implementation's additional `spctl --assess --type execute` gate was reproduced against the installed signed Gimbal CLI: `codesign --verify --strict --verbose=2 /opt/homebrew/bin/gimbal` succeeded, while `spctl` rejected it with `the code is valid but does not seem to be an app`. The author removed this extra gate before this report. The current signing path requires strict code-signature verification and Apple's explicit `Accepted` notarization result, matching Gimbal's standalone CLI approach. This repaired defect is not an outstanding finding.

The adaptation preserves the relevant Gimbal mechanisms: temporary keychain and profile, Developer ID identity/team verification, hardened runtime with the plugin library-validation entitlement, ZIP submission to notarytool, and packaging the accepted signed executable. Desktop packaging, Sparkle, and app stapling were correctly omitted. The workflow uses its own `GITHUB_TOKEN` for same-repository release uploads and a separate tap deploy key for Homebrew.

## Focused verification and test assessment

`go test -count=1 ./cmd/skgo-release ./cmd/skgo-release-version ./internal/releaseversion` passed. The version command has no direct test files; its existing version-analysis package tests passed. `go vet ./cmd/skgo-release` and `git diff --check` also passed.

The new release tests are load-bearing for the boundaries they cover: the archive test checks independently supplied bytes, the complete entry set, and executable permissions; the checksum test requires all three platforms and compares to the independently known SHA-256 of `abc`; the cask test checks a literal versioned download URL, checksum, platform restriction, and executable; the cleanup test checks refusal to delete an unowned runner directory. Removing these behaviors would fail the relevant assertions. These tests do not exercise real Apple signing, the Actions graph, or publication.

This is an implementation/setup review, not completed release qualification. No end-to-end SKGO Actions release, real SKGO notarization, uploaded release archive, or Homebrew installation has yet been demonstrated in the evidence inspected. Those remain necessary before reporting the requested working release system as delivered.
