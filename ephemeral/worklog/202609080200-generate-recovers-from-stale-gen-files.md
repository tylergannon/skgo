# Issue #91: `skgo generate` recovering from a stale generated file

## The trap: filtering packages.Load errors by file doesn't work

First attempt: let `loadApp`'s `packages.Load` errors through unless the
offending file's `packages.Error.Pos` names a `_gen.go` file. Wrong — a route
package reached only through the generator's own `generated/links/<enc>`
symlink produces its type error as a single `ListError` with `Pos: "-"` and
`Msg` holding raw compiler stderr (`"# pkgpath\nfile:line:col: msg"`), not a
structured `Pos`. Had to regex-parse `Msg` for `file:line:col:` lines instead,
and even that only fixed `loadApp`'s own load.

## The real blocker: polytype's own loader has zero tolerance

`generateTypes` → `projectTypes` calls polytype's `grammar.Load(dir)` on the
*same* route package, and it runs **before** `writePackageBindings` rewrites
the stale file (types are generated first, deliberately, so a type that can't
cross to TypeScript fails before any stub is written — see the comment in
`gen.go`). `grammar.Load` (`polytype@v1.0.0-rc.11/grammar/grammar.go`) hard
fails on any `pkgs[0].Errors` with no way to tolerate or filter — it's a
pinned, read-only dependency, so there is no "pass an error-tolerant config"
option. Any fix based on tolerating/filtering errors after the fact cannot
reach this call site.

`generateCodecs`'s own `grammar.Load` calls happen to run *after*
`writePackageBindings`, so they were never actually at risk — only
`generateTypes`'s early one is.

## The fix that actually works: reset-before-load, restore-on-failure

Before `loadApp` runs at all (first thing after `findSourceFiles` in
`gen.Run`), blank every `skgo_remotes_gen.go` beside a wanted source file to a
minimal `package X` placeholder (X read from a *sibling* file's package
clause via `go/parser` with `PackageClauseOnly`, since the stale file's own
clause might not be the thing that's broken — though in practice it usually
still parses fine). This makes every subsequent load (ours and polytype's)
see trivially-valid Go, so the "does this generated file compile" question
never comes up at all — no error-message parsing needed anywhere.
`writePackageBindings` overwrites the placeholder with real content later in
the same run, as it always did.

`Run` now has a named return (`func Run(cfg Config) (err error)`) with a
`defer` that restores every touched file from its saved original content if
`err != nil` when the function returns — a failed run (a genuine developer
type error, say) must leave the tree exactly as it found it, not with
generated files blanked partway through.

This is `internal/gen/gen.go`'s `resetStaleGeneratedFiles` /
`packageNameOf`. `scan.go` (loadApp) is untouched — the whole fix lives
upstream of it.

## Test infra note

`internal/gen/stalegen_test.go` copies `example/` into a temp dir (skipping
`web/node_modules`, `web/.svelte-kit`, `web/build`, `e2e`, `tmp` — none exist
in a fresh checkout anyway), rewrites the `replace github.com/tylergannon/skgo
=> ../` line in the copy's `go.mod` to an absolute path back to this checkout,
then runs the real `go generate ./...` (exec, `GOWORK=off`) from
`<copy>/generated` — exercising the actual `cmd/skgo` binary end to end, not
just `gen.Run` in-process. `go build ./...` afterwards is also run from
`<copy>/generated` rather than the module root, specifically to avoid
`example/web`'s `//go:embed all:build` (there is no built frontend in a fresh
checkout) — `./...` from `generated/` never reaches the `web` package anyway
since the corrupted/rebuilt route packages are only reachable through the
`generated/links/*` symlinks, which is exactly the subtree this issue is
about.

Each run of `TestGenerateRecoversFromAStaleGeneratedFile` recompiles
`cmd/skgo` fresh (via `go tool skgo`, resolved through the sandbox's absolute
`replace`), ~20s. Not slow enough to skip, but don't be surprised.
