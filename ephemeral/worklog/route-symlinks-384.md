# Route links as committed symlinks (#384)

- Directory symlinks are the wrong shape. `./...` does not follow a directory
  symlink, so `go test ./...`, `go vet ./...` and staticcheck silently drop
  every route package except the route root. Each link is therefore a real
  directory of relative file symlinks, the form the pre-#147 design used only
  for the route root. Git still records only symlinks, so a route edit changes
  one file.
- The retained source view (`retained.go`, the walk that fills
  `CompilerInputs.files`) skipped every symlink, so the hot-reload host lost
  linked route files ("captured route source missing"). It now reads file
  symlinks and still skips directory symlinks.
- Generation's overlay (`generatedOutput.overlay`) is still required: it shows
  unpublished output and route files at link addresses on a first generation,
  before any link exists. Only the read-only overlay (`skgo check`, `ReadOnly`
  scans) existed for the copies.

## 2026-10-18: link tree moved to `_links/`

- After a route rename or delete, the old link holds `.go` symlinks to files
  that no longer exist. `go list ./...` fails on them (`pattern ./...: stat
  ...: no such file`) before any generator runs, so `go generate ./...` exited
  1 even though skgo repaired the tree. A build tag cannot help: Go stats the
  file before reading constraints, and plain `go generate` passes no tags.
- Generation itself already tolerates stale generated code through the
  generation overlay (removing a function a route's `skgo_gen.go` names
  generates fine). Route code also uses generated types such as
  `PageRequestEvent`, so excluding generated files by tag would break authored
  packages. A generation-only tag would also defeat `PackageSnapshot` reuse,
  which requires identical build flags.
- `_links/` is importable but skipped by `./...` (even `./_links/...` matches
  nothing). `skgo check` passes each link package explicitly to vet,
  staticcheck and advice. `go test ./...` no longer runs route-directory
  tests; the testing doc gives `go test $(go list -deps ./internal/skgo | grep
  /_links/)`.
- Upgrading: the first generate removes the old `links/` tree (logged with the
  import rewrite) and exits 1 once, because that tree was still in `./...`.
  Authored imports of the old path fail generation with a message naming the
  new import path.
