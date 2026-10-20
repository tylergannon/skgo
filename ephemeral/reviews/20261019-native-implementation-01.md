# Native plugin implementation — adversarial review, round 01

Date: 2026-10-19

## Target

This review covers the worktree `/Users/tyler/src/skgo-worktrees/native-plugin`, branch
`codex/native-plugin`, based on `fa29e01a`. It covers the complete uncommitted diff and the
untracked source:
- `plugins/native-shell/` (module, nativeapp, nativebuild, sdk, example)
- `templateapi/{generator,command}.go` and their examples and guide pages
- `internal/plugininstall/prebuilt.go`
- `internal/gen/plugins.go`
- `internal/templates/commands.go`
- `cmd/skgo/{plugin_project,capability_commands_test}.go`, with `testdata/capability-plugin`
- `.github/workflows/native-plugin.yml`

The review measured the work against `AGENTS.md`/`CLAUDE.md`,
`ephemeral/native-plugin/native-plugin-decisions.md`, `native-plugin-contracts.md`
and `native-plugin-plan.md`, plus the beside-code guide and godoc conventions. Implementation is
ongoing, and this review judges the software as it stood when I read it.

## Evidence inspected

- Every modified, deleted and untracked file above, plus the surrounding code in
  `internal/gen`, `cmd/skgo`, `internal/templates`, `internal/pluginbuild`,
  `internal/releaseversion`, `internal/webbuild`, `internal/update` and the CI workflows.
- `go test` passes for `./internal/gen ./internal/plugininstall ./internal/templates
  ./templateapi ./cmd/skgo ./internal/webbuild ./internal/update ./internal/releaseversion`.
  `ExampleGenerator` and `ExampleCommandPlugin` pass.
- With `GOWORK=off`, from `plugins/native-shell`, `go vet ./...` is clean and `go test ./...`
  passes. The sdk Zig/Swift tests are tagged `skgo_qualification`; I did not run them.
- `go test -tags skgo_qualification -count=1 ./cmd/skgo -run '^TestCapabilityPluginActualCLI$'`:
  - The first run at 17:39:23 failed with `forwarding: ... older project has no skgo.json`.
  - The fixture was edited at 17:39:24 to write `skgo.json`, and a rerun against the current
    tree passed in 15.7s.
  - That finding was resolved during the review and is not reported.
- I ran the real program on a scratch copy of `plugins/native-shell/example`. The setup used an
  isolated HOME/GOMODCACHE, the provisional `.cache/provisional` host (v0.41.0) and the
  `native-shell-v0.1.0-skgo-v0.41.0-darwin-arm64.so` artifact.
  - `skgo plugin install --prebuilt … --sha256 …` succeeds.
  - `skgo plugin --help` lists native-shell.
  - `skgo plugin native-shell help` works.
  - `skgo generate --plugins --web web --out internal/skgo` writes `native/Generated.swift` with
    `core.call("uy1bk5/readValue", kind: .query…)` and `writeValue`.
  - After I edited `Generated.swift` by hand, regenerating refuses with "authored or edited;
    refusing to overwrite".
  - The core outputs (`skgo.remotes.json`, `value.remote.ts`) differ from the committed copies
    only in formatting.
- Grep confirms that core no longer references `swiftgen`, `nativebuild` or `nativeapp`, and that
  the plugin imports only `templateapi` and its own packages.
- The guide pages (`templateapi/generator-plugins.md`, `templateapi/plugin-capabilities.md`,
  `plugins/native-shell/native-shell.md`) have the required frontmatter, and their fences are
  empty with valid Example or path names. The docs conventions are met.

## Findings

### 1. Issue — incomplete requirement: the moved native-shell tests run nowhere

- **Where they are not run:**
  - `go.work` uses only `.` and `./example`.
  - No step in `.github/workflows/ci.yml`, `qualification.yml` or `Justfile` names
    `plugins/native-shell`.
  - The `native-plugin.yml` candidate job (lines 42–58) runs only `cmd/skgo-plugin-build` and no
    `go test`.
- **Tests affected:**
  - the Swift emitter tests moved from `internal/swiftgen`
  - `nativebuild` and `nativeapp`
  - the sdk Zig/Swift qualification tests, which used to run in `qualification.yml`
- **What the contract asked for:** it says the existing emitter tests are "moved or adapted", and
  it asks for independent proof of the plugin.
- **Impact:** the moved tests are dead code. `go test ./...` from the repo root does not reach
  the module, so a regression in Swift generation or the native build reaches a candidate `.so`
  untested. Nothing short of the private gate would catch it, and that gate needs Xcode.
- **Reproduction:**
  - `grep -rn 'plugins/native-shell' .github Justfile go.work` finds only the
    `skgo-plugin-build --source` argument.
  - Breaking an assertion in `plugins/native-shell/plugin_test.go` leaves every CI job green.

### 2. Issue — verifiable bug: the example go.sum pins a fabricated skgo v0.41.0

- **Evidence:**
  - `plugins/native-shell/example/go.mod:9` requires `github.com/tylergannon/skgo v0.41.0`,
    which is unpublished; the latest tag is `v0.40.1`.
  - `plugins/native-shell/example/go.sum:46-47` records `h1:W0yBPiYyCkvp1iGY1GLcD7xPbsBSmN5yAIueykzT410=`.
  - That is exactly the `ziphash` of the local file proxy in
    `.cache/provisional/modcache/…/v0.41.0`, a zip built from this working tree.
  - The `replace => ../../..` seen earlier in the review has since been removed.
- **Impact:** the hash can never match a real tag. A real tag is cut from a different tree, and
  the commit time is part of the module zip, so the content differs.
  - Once v0.41.0 is published, any `go mod download` or `go build` of the example fails with
    `verifying github.com/tylergannon/skgo@v0.41.0: checksum mismatch … SECURITY ERROR`.
  - Until then, `GONOSUMDB`/the proxy cannot resolve it at all.
  - The release gate's tiny application is the one place that must use the official pairing, and
    as committed it is unbuildable.
  - The decisions document says local development pairings are provisional evidence only; this
    commits one into a tracked file.
- **Reproduction:** after v0.41.0 is tagged, run
  `cd plugins/native-shell/example && GOWORK=off GOFLAGS=-mod=mod go mod download github.com/tylergannon/skgo`.
  Today, `GOWORK=off go build ./...` fails because v0.41.0 cannot be resolved.

### 3. Issue — incorrect implementation: two independent sources select the Swift remotes

- **The two sources:**
  - Generation uses the selections pinned in `skgo-plugins.json`, through `--select`, `Generate`
    and `GenerationModel.Remotes`.
  - The Xcode project includes `native/Generated.swift` only when
    `nativeapp.Config.SwiftRemotes` from `native/skgo-native.json` is non-empty
    (`plugins/native-shell/nativebuild/build.go:246`).
- **How they diverge:** the example keeps the list in both files.
  - If a developer selects remotes on the pin but leaves `swiftRemotes` empty, the file is
    generated but never compiled. Calls from `ApplicationView.swift` then fail to link with
    "cannot find 'readValue' in scope".
  - If a developer sets `swiftRemotes` but no pin selections, the project references a
    `Generated.swift` that has no remotes, or none at all.
- **Impact:** the plugin contract makes the pin the single selection record. A second list in
  the application config contradicts that, and nothing checks that the two agree.
- **Reproduction:** in a copy of the example, delete `swiftRemotes` from
  `native/skgo-native.json`, then run `skgo plugin native-shell build`. `projectSpec` omits
  `Generated.swift`, and the Swift target no longer sees the generated calls.

### 4. Nitpick — incorrect implementation: the example's `cmd/main.go` contains unrendered template placeholders

- **Where:** `plugins/native-shell/example/cmd/main.go` has the following lines, copied from the
  scaffold template without rendering:
  - line 1: `// Command {{.App}} …`
  - line 21: `var origin = "{{.Origin}}"`
  - line 43: `log.Printf("{{.App}}: listening …")`
  - line 69: `return "{{.OriginHostPort}}"`
- **Impact:** the example's server binary defaults to listening on the literal address
  `{{.OriginHostPort}}`, which is invalid, and its origin is `{{.Origin}}`. Running
  `go run ./cmd` without `-listen` fails. This is the reference application the guide points
  developers to.

### 5. Nitpick — over-engineering: `InstallPrebuilt` writes a machine-absolute path into the committed pin

- **Where:** `internal/plugininstall/prebuilt.go:193` (`p.Artifact, err = filepath.Abs(p.Artifact)`).
- **Effect:** a local `--prebuilt ./x.so` install records `/Users/<name>/…/x.so` in
  `skgo-plugins.json`. That file is project state meant to be restored on other machines.
- **Impact:**
  - `skgo plugin restore` on any other checkout cannot find the artifact.
  - The project file leaks a local path.
  - Either reject non-HTTPS sources in the pin, or record the local file as provisional and keep
    it out of the committed pin.
- **Related nit:** `plugins/native-shell/plugin_test.go:21` uses the Kind `"liveQuery"`, which
  core never emits; core documents `"query.live"`. The test passes for the wrong reason.

## Outcome

material findings remain
