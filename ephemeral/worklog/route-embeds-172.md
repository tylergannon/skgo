# Route embeds (#172)

- The issue asked for a Go-owned equivalent of Kit's `read`. A Go `Read(url)`
  has no caller: `$app/server` is server-only in Kit, every server-only module
  in skgo is Go or a throwing stub, and Go never holds a Vite asset URL. The
  real defect was that `//go:embed`, Go's own replacement, could not work in a
  route package because the link tree held only `.go` symlinks and embed
  refuses symlinked files.
- The root checkout's `example/web/node_modules/@sveltejs/kit` was 3.0.0 while
  the pin is 3.1.0. Run `vp install` in the task worktree before mapping Kit.
