# junkyard Go CLI: `skgo` and `remote-oracle`

Source root: `junkyard/go/` (module `github.com/tylergannon/sveltekit-adapter-go`, `go 1.27`, sole direct dep `golang.org/x/tools v0.49.0` — `junkyard/go/go.mod:L1-L10`).

## Purpose

`cmd/skgo` is the previous attempt's front door: a three-verb generator CLI (`remotes generate | build | stage-b`) plus a `test` wrapper that makes `go test` reach route-colocated packages. `cmd/remote-oracle` is a proof fixture server that mounts the runtime `remote.Handler` with hard-coded document bindings; it is mostly process artifact but shows the untyped `remote.Binding` API in use.

## Key concepts

- **Verb dispatch.** `skgo test` is special-cased before the `remotes` subcommand tree; everything else must be `skgo remotes {generate|build|stage-b}` — `junkyard/go/cmd/skgo/main.go:L14-L46`, usage text at `L80-L82`.
- **`remotes generate [--check]`** runs Stage A (`remotegen.Generate`): scans `*.remote.go`, writes `.remote.ts` stubs, runs go-gen-jsonschema, writes inventories. `--check` reports drift and exits non-zero without writing — `junkyard/go/cmd/skgo/main.go:L84-L117`. Flags: `--go-root`, `--app`, `--remote-root` (default go-root), `--marker-pkg` (tests only).
- **`remotes build` is the production pipeline in one gesture**: Stage A → remove stale capture → `sh -c "vp build"` in the app dir with `SKGO_MODE=production` and `SKGO_CAPTURE=<app>/.skgo/remotes-kit.json` in env → read the capture the Vite plugin wrote → Stage B writes the registrar → `go build -trimpath -o <out> <main>` — `junkyard/go/cmd/skgo/build.go:L15-L83`. This is the two-stage design: **Kit's real remote ids are captured from Kit's own build, never computed in Go** (see `remotegen.md`).
- **`readCapture` polls up to 5s** for the capture file because the Vite plugin may write it after `vp build` exits — `junkyard/go/cmd/skgo/build.go:L85-L103`. The error message names the likely cause ("is the skgo Vite plugin installed?").
- **`remotes stage-b --capture FILE`** is the same Stage B step exposed standalone for a dev server front door — `junkyard/go/cmd/skgo/main.go:L48-L78`.
- **`skgo test` = `go test` plus the link packages.** `./...` never expands into `.skgo/links/<base32>` symlinks (they are outside module package discovery because of the `src/routes/go.mod` boundary), so the wrapper reads `.skgo/links.json` and appends each `./<link>` to the package list, inserting *before* `-args` if present — `junkyard/go/cmd/skgo/test.go:L16-L68`. It forwards stdin/stdout/stderr and returns `go test`'s own exit code. `-C DIR` keeps Go's own meaning (chdir first), `--go-root` is the alias; otherwise it walks up to the nearest `go.mod` — `L70-L119`.
- **Tests prove the wrapper end to end** by building a temp module with a route package under `(group)/[id]` reachable only via the link, then checking `./...` alone would miss its deliberately failing test — `junkyard/go/cmd/skgo/test_test.go:L13-L46`, `L58-L90`, `-args` placement `L170-L191`.
- **`remote-oracle`** wires `remote.NewHandler(manifest, []remote.Binding{...}, version)` with untyped `Query`/`Command`/`Live` closures over a JSON file, and picks primary/secondary modules out of the manifest — `junkyard/go/cmd/remote-oracle/main.go:L83-L112`, `L115-L138`. The handler bodies show the raw `any` contract: query arg arrives as a Go value tree (`arg != "document.json"`), command input as `map[string]any` with `float64` numbers, and `refreshes.RefreshRequested()` opts into post-command refresh — `L140-L186`. The `live` handler polls a file on a ticker and yields on change, returning `ctx.Err()` on disconnect — `L188-L225`. Everything else in that file (events JSONL, run-id, lane, `omit-refresh`/`wrong-live` controls) is proof machinery.

## Citations

- `junkyard/go/cmd/skgo/main.go:L14-L46` — verb dispatch
- `junkyard/go/cmd/skgo/build.go:L46-L82` — generate → vp build (env SKGO_MODE/SKGO_CAPTURE) → StageB → go build
- `junkyard/go/cmd/skgo/test.go:L23-L52` — link-package injection into `go test`
- `junkyard/go/cmd/skgo/test.go:L57-L68` — `-args` boundary handling
- `junkyard/go/cmd/remote-oracle/main.go:L87-L104` — untyped Binding wiring example

## Reusable verdict

| Component | Verdict | Why |
|---|---|---|
| `skgo test` wrapper (`test.go`) | REUSE WITH CHANGES | The problem it solves (route dirs named `[id]`/`(group)` are unreachable by `./...`) is exactly the new brief's symlink problem. Keep the mechanism; rename the inventory path and drop `--go-root` if the new CLI has a config file. |
| `remotes build` orchestration (`build.go`) | REDESIGN | It depends on a Vite capture plugin to learn Kit's remote ids. The new brief says the Go generator emits handlers "at exactly the URL paths SvelteKit would use" — so either compute the hash in Go or keep this capture step; decide first (see `remotegen.md` Gotchas). Also needs `+page.server.ts` generation and `polytype` instead of `go tool gen-jsonschema`. |
| `remotes generate --check` semantics | REUSE AS-IS | Drift-report-without-writing, non-zero exit, is the right CI shape. |
| `readCapture` polling | REUSE AS-IS (if capture stays) | Small, correct, names the failure. |
| `remote-oracle` | REDESIGN / skip | Proof fixture only. Take the `Binding` usage example, discard events/lanes/controls. |

## Gotchas

- `go 1.27` in `go.mod`; the code uses `strings.SplitSeq`, `for range 3`, `t.Chdir`, `os.CopyFS` (Go 1.24+ APIs). Do not lower the go directive below 1.24.
- `remotes build` hard-codes `sh -c` and defaults `--build-cmd` to `vp build` (Vite+), so it only works with a Node toolchain present and the skgo Vite plugin installed; without the plugin the capture never appears and the build fails after 5s.
- The `-C` flag is consumed by the wrapper *and* honored as Go semantics; do not also pass `-C` through to `go test` (it is stripped at `test.go:L84-L104`).
- `skgo test` deduplicates link packages already named explicitly (`test_test.go:L92-L118`); without a `links.json` it is byte-for-byte plain `go test` (`L120-L133`).

## Recipes

- To add a verb to the CLI: start at `junkyard/go/cmd/skgo/main.go:L26-L45`.
- To make `go test ./...` cover route packages behind symlinks: copy `junkyard/go/cmd/skgo/test.go:L23-L68` and point it at your link inventory.
- To see the whole build pipeline order (what runs before what): `junkyard/go/cmd/skgo/build.go:L46-L82`.
- To wire a hand-written (untyped) binding into the runtime handler: `junkyard/go/cmd/remote-oracle/main.go:L87-L104`.
