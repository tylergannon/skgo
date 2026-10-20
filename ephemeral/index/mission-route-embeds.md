# Mission: `//go:embed` in route packages (#172)

Decided by Tyler, 2026-10-19:

- A route's own Go file may `//go:embed` files beside it. Keep the code small.
- Kit's `read` from `$app/server` is not a skgo API: kit's guard makes it
  importable only from server-only modules, and in skgo all of those are Go or
  throwing stubs. Go's `embed` + `io/fs` + `net/http` is the replacement. No
  new skgo API.
- Go does not learn Vite's hashed asset URLs; documented as an incompatibility.
- Universal-load SSR `fetch` of imported assets (incl. `data:` URLs) is out of
  scope.

## Verified (probes, Go 1.27.2)

- Today `internal/skgo/_links/<enc>/` holds one symlink per `.go` file. An
  embed in a route file fails: `pattern x.txt: no matching files found`.
  Symlinking the asset too: `cannot embed irregular file x.txt`.
- A directory symlink `_links/<enc>` -> route directory: embed works, and
  `go list -f {{.Dir}}` reports the link path.
- The route root cannot be a directory link while the boundary
  `web/src/routes/go.mod` sits in it (Go sees a separate module).
- With `ignore ./web/src/routes` in the app's go.mod and no boundary go.mod,
  directory links work for the root and for `[slug]`; `go list ./...`,
  `go vet ./...`, `go build`, `go run` all pass. The app go.mod already uses
  `ignore ./web/node_modules` (`example/go.mod`,
  `internal/newapp/gofiles/go.mod.tmpl`).

These are leads, not a design. The builder chooses.

## Anchors

- `internal/gen/links.go`: header comment (l.12-45) says why boundary + file
  links; `sync`, `syncLink`, `syncBoundary`, `prune`, `removeMovedRoot`;
  `links.json` inventory with a `Boundary` record.
- `ephemeral/worklog/route-symlinks-384.md`: why file links replaced copies
  and why the tree moved to `_links/`.
- `internal/gen/retained.go:515-535`: the source-view walk skips directory
  symlinks (dev hot-reload capture).
- `internal/hotreload/host.go:124-170`, `internal/hotreload/retained.go:98-155`:
  embed files already tracked; a changed embedded input forces a complete
  rebuild.
- `internal/gen/package_stage.go:44-200`: generation stage copies embed
  patterns. `internal/devserver/generator.go:130-170`: dev discovery includes
  `EmbedFiles`.
- `internal/check/check.go`, `internal/advice/advice.go`: name link packages.
- Pages describing the layout: `routing.md:98`, `glossary.md:21`,
  `testing.md:26-31`, `debugging.md:22`, `project-structure.md:104`,
  `cmd/skgo/json-output.md:62`. Capability rows: `README.md:259`,
  `differences.md:80`.
- Example route Go sits beside `+server.ts`, e.g.
  `example/web/src/routes/api/todos/server.go`. Handler tests of the built
  example: `example/*_test.go` under `skgo_qualification` (see `getAs` in
  `example/universal_fetch_test.go`).
- Dev integration: `internal/devserver/session_integration_test.go`
  (`skgo_dev_integration`).
- Kit 3.1.0 is installed in this worktree's `example/web/node_modules`
  (`src/runtime/app/server/index.js`, `src/exports/vite/plugins/guard.js`).
  The root checkout's copy is a stale 3.0.0.

## Gestures

`just generate`, `just build`, `just test`, `just vet`;
`go test -tags skgo_dev_integration ./internal/devserver -run <name>`.
Frontend through `vp` only.
