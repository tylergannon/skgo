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
