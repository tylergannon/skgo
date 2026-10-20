# Issue 364 — documentation drift

trap: skgo-project's `cmd/docgen/versions.go` reads `internal/toolchain/pins.go`
constants by name and fails on a missing one. Renaming a pin breaks the site
build at its next `site/go.mod` bump. `NodeMinimum` -> `Node` landed with
skgo-project #191, which accepts both names.

discovery: `docs/` is the live Pages site (`.github/workflows/pages.yml`),
with its own copy of the feature matrix in `docs/features.html`. README matrix
corrections do not reach it. Editing it needs Tyler's express permission
(granted for #364).

decision: Node is a range, not a floor. VitePlus 1.3.0 declares
`^22.18.0 || ^24.11.0 || >=26.0.0`, narrower than Kit's `>=22.17`; the test
pins `toolchain.Node` to the vite-plus entry in `example/web/pnpm-lock.yaml`,
so a VitePlus bump that changes engines fails `go test ./internal/toolchain`.

friction: `git checkout <file>` to undo a mutation test also discarded the
uncommitted edit under test. Commit first, or mutate with sed and revert with
sed.

unproven claims left in place (tests removed by b701d04c): universal loads
traced during production SSR (`observability.md`, `tracing_renderer.go`);
`checkRouteConfigs` refusing non-empty route `config`.
