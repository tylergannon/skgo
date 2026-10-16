# Signed CLI release review, round 02

Outcome: no findings

Target: follow-up commit `2fd1229` (actual release archive/plugin qualification), the merged qualification toolchain correction `076d99f`, and their integration in the current release workflow. Reviewed against the original user request, repository instructions, and the existing round 01 review. No caller scope narrowing was supplied. Review remained read-only except this report.

## Evidence inspected

- The complete follow-up diff, `cmd/skgo-release/archive_test.go`, its platform workflow invocations, and the surrounding release command/archive implementation.
- The real plugin builder, host identity handling, and the independent template-plugin fixture used by the test.
- Gimbal's `origin/main` release CLI script and packaged-CLI qualification recipe. Gimbal also tests the packaged CLI after macOS signing and before distribution.
- The merged qualification Go-pin fix, current workflow pins, and native startup toolchain constraints/tests. The Go library minimum and the native development toolchain qualification are different requirements; running the full existing test recipe with Go 1.27.2 is consistent with its native development contracts.

## Findings

No material findings or genuine nitpicks remain in this follow-up.

The archive test is load-bearing for the claimed packaged-plugin capability. It extracts and executes the actual downloadable archive rather than building an alternate source CLI, checks executable permissions, and compares the executable's identity to the required release version and literal Go toolchain pin. It builds an independent external source module against that host, requires discovery of a specific fixture export, invokes the export through the packaged CLI, and compares the generated receipt to literal fixture contents. A missing export, failed dynamic load, broken option binding, or nonexecuting plugin cannot satisfy the final positive assertion.

Both Linux architecture jobs and the macOS job run this test against their own archive before uploading artifacts. The macOS call occurs after signing/notarization, so its eventual successful execution will cover the hardened executable's ability to load a separately built Go plugin. Explicitly requesting the test without its required release archive fails; the build tag excludes this expensive release-specific test from normal source tests without allowing a requested qualification to silently skip.

## Focused checks

- `go test -count=1 ./cmd/skgo-release ./internal/releaseversion` passed.
- The four ordinary release-command tests also passed when compiled with `-tags release`.
- `env -u SKGO_RELEASE_ARCHIVE -u RELEASE_VERSION go test -tags release ./cmd/skgo-release -run '^TestPackagedCLI$' -count=1 -v` failed as required, at `archive_test.go:30`, reporting that the archive and version variables are required. This is a verified negative case, not a successful archive qualification.
- `go test -count=1 ./internal/hotreload -run '^TestNativeStartupDiagnostics$' -v` ran its named cases and passed.
- `go vet -tags release ./cmd/skgo-release` and `git diff --check` passed.

No release archive has been built in the reviewed evidence. The positive packaged-CLI/plugin test, real Apple acceptance, complete Actions release, and Homebrew installation therefore remain execution work; this review does not claim those outcomes.
