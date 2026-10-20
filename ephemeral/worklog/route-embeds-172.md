# Route embeds (#172)

- The issue asked for a Go-owned equivalent of Kit's `read`. A Go `Read(url)`
  has no caller: `$app/server` is server-only in Kit, every server-only module
  in skgo is Go or a throwing stub, and Go never holds a Vite asset URL. The
  real defect was that `//go:embed`, Go's own replacement, could not work in a
  route package because the link tree held only `.go` symlinks and embed
  refuses symlinked files.
- The root checkout's `example/web/node_modules/@sveltejs/kit` was 3.0.0 while
  the pin is 3.1.0. Run `vp install` in the task worktree before mapping Kit.
- Route links are now directory symlinks, so anything that canonicalizes paths
  (EvalSymlinks) sees through them. `stagePackageOverlay` canonicalized overlay
  paths and then failed to mark `_links` as a branch, so the staged view's
  link pointed at the physical route tree and missed new generated files; it
  now marks the lexical ancestors real too.
- `materializeIn` refuses imports through symlinked directories. Route links
  are exempt because the private view's own generation recreates them; compare
  against the physical (`EvalSymlinks`) link root or `/var` vs `/private/var`
  defeats the check on macOS.
- The retained source walk skipped directory symlinks, so route Go files had to
  be recorded at link addresses explicitly or capture reports them missing.
- Hot-reload resources are keyed by the link path Go reports, but an editor's
  write lands at the authored path; `CompileHost` aliases both.
- Links must exist before the first package load of a generation: the overlay
  shows Go files at link addresses but not embedded files, so sync happens in
  `newGeneration`, not only in `publish`.
- `modfile.AddIgnore` turns a lone `ignore` line into a block and moves its
  comment inside, the same edit `go mod edit -ignore` makes.
- An authored `web/src/routes/go.mod` breaks only the route-root link (Go walks
  up the link path to find a module). It is left alone unless the route root
  has Go files, where generation fails and names the file.
- Adding the `ignore ./web/src/routes` directive edits go.mod, and go.mod's
  bytes key the generation cache, so a cold run that made the edit persisted a
  plan the next run missed (`TestGenerationCacheFreshProcessesAndRecovery`).
  `runCached` syncs links before computing the key.
- `webbuild.treeHash` (the `skgo build` input fingerprint) refused every
  directory symlink; it now hashes the link text of one that resolves inside
  the app, whose target the walk already reads.
- The pending-browser dev session test needs `vp install` in `example/e2e`
  (bddgen); without it it fails with "Failed to find executable bddgen".
- Hot reload records an embed both as its files and as a pattern's directory
  prefix. Editing an existing file only exercises the file entries; only a new
  file matching a pattern needs the prefix entry. A dev test that only edits a
  globbed file cannot tell the two apart.
