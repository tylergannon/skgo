# Release-age exception during update

decision: The released v0.28.2 recorder refusal is retained by the consumer owner, not retried here. Its original workspace has no minimumReleaseAge, minimumReleaseAgeStrict or minimumReleaseAgeExclude; its log says pnpm added exactly @skgo/sveltekit-adapter@0.28.2. The disposable stage was deleted, so its after-bytes are unavailable.

source: pnpm v12.9.1 pnpm/crates/package-manager/src/minimum_release_age.rs persists exact exclusions after non-strict installation. Explicit age configuration defaults strict; noninteractive strict refusal precedes persistence. pnpm/crates/config/src/version_policy.rs merges exact versions into unions. This behavior belongs to the qualified package manager; do not disable age policy or give skgo ownership of arbitrary policy edits.

decision: Recognize only the installed, selected adapter version as an additive exception after successful staging. Retain every existing exception and all age/strict settings. Reject other new exemptions, wildcard broadening and removals. Public released consumer retry remains outstanding until a corrected release; deterministic package-manager regression is candidate evidence only.

friction: An initial full suite under ambient Go 1.27.2 reached Staticcheck export-data incompatibility (unicode/utf16 export version 5 versus supported 4). Use the repository Go 1.27.1 toolchain for the qualification run; retain the failed ambient run instead of treating it as a passing check.
