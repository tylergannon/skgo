# Native-shell extraction — adversarial review, round 05

## Target

- Complete extraction, base `fa29e01a27919b6ad607943667db9d30a3535770` → `1c3d3a21` (draft PR #393). Source unchanged since checkpoint.
- Measured against AGENTS.md, `ephemeral/native-plugin/native-plugin-{decisions,contracts,plan}.md`, and the adjudications in `ephemeral/worklog/20261019-native-plugin-implementation.md`.
- Runtime pair: host `4d11e9cd…` (unchanged); refreshed plugin `a98302999c8fbaf1662e3a3d19a568d8ab131f5e318405d4cd06fcc71f0b8dca`.
- Provisional local evidence only. The official-pair private gate is external and deferred, and is not judged here.

## Evidence inspected

- **Code:**
  - `plugins/native-shell/{plugin.go,plugin_test.go,nativebuild/,swiftgen/,sdk/,example/}`;
  - `templateapi/{generator,command}.go`;
  - `internal/gen/plugins.go`;
  - `cmd/skgo/{plugin_project.go,templates.go}`;
  - `internal/{plugininstall,pluginbuild,releaseversion,update}`;
  - `cmd/skgo-release-version`;
  - the workflows `native-plugin.yml`, `native-plugin-tests.yml`, `ci.yml`, `qualification.yml` and `release.yml`;
  - `.gitignore`.
- **Guides:**
  - `README.md` (the native plugin section and lines 369–372);
  - `templateapi/plugin-capabilities.md` and `generator-plugins.md`;
  - `plugins/native-shell/native-shell.md`;
  - `plugins/native-shell/sdk/swift/README.md`;
  - `introduction.md`, `project-types.md`, `plugin.md` and `add.md`.
  - Frontmatter is valid, code fences have empty bodies, and grep found no stale references to `--swift-*`, `skgo native build`, `internal/swiftgen`, `internal/nativebuild` or the deleted pages.
- **Tests run:**
  - Core `go vet` and `go test` for `cmd/skgo`, `internal/{gen,plugininstall,templates,pluginbuild,releaseversion,update,webbuild}` and `templateapi`: green.
  - Native module (`GOWORK=off`) `go vet ./...` and `go test ./...`: green.
  - `go test -tags skgo_qualification -count=1 ./cmd/skgo -run '^TestCapabilityPluginActualCLI$'`: green (37.7 s).
- **Artifact:**
  - Both SHA256 values verified.
  - `go version -m` reports `-buildmode=plugin`, `-trimpath=true`, `CGO_ENABLED=1`, go1.27.2.
  - No `/Users/` strings in the `.so`.
  - All 26 embedded SDK files are byte-present in the `.so`; the scaffold excludes `host_test.go`.
- **Runtime (my own run, scratch copy outside gitignored ancestors, fresh HOMEs, the stated GOWORK/GOMODCACHE/GOPROXY environment):**

  | Run | Step | Exit | Observed |
  |---|---|---|---|
  | A | Install with a wrong SHA | 1 | No pin written |
  | A | Install with the correct SHA | 0 | Pin records the relative slot path |
  | A | Bare `plugin native-shell` | 0 | Prints help |
  | A | `build --platform macos` | 0 | Build succeeds |
  | A | `TinyNative 'r05-7AEC3A57-ñ "q"'` | 0 | stdout `initial`, then the exact supplied value; stderr empty |
  | B | Command before restore | 1 | "unavailable; run skgo plugin restore" |
  | B | `plugin restore` | 0 | Installs the pinned artifact |
  | B | `sdk --out` | 0 | READMEs byte-identical to the tree |
  | B | Go mutation (initial `seed-r05x`, command stores `"go:"+next`), then rebuild and launch with `mut-r05y` | 0 | Prints `seed-r05x`, then `go:mut-r05y`; neither string appears in any Swift source |

  - Run B shows that the values come from Go.
  - `.cache/final-reviewed/native-run.txt` (18:27) predates the refreshed `.so` (18:35); the table above is the current-pair evidence.

## Prior findings

| Round 04 | Status |
|---|---|
| 1 Identity-check subtraction undocumented | Fixed: `README.md:369-372`, `templateapi/plugin-capabilities.md:45`, `native-shell.md` |
| 2 Stale SDK README | Fixed: plugin paths, CI claim matches `native-plugin-tests.yml`, prebuilt install flow |
| 3 Add-on ownership undocumented | Fixed: `native-shell.md` and `plugin-capabilities.md` |
| 4 Example hang / dependency nitpicks | Remain (nitpicks 3–4 below) |
| 5 Test / CI coverage nitpicks | Partially fixed: `Generate` tested; the rest remain (nitpicks 2, 5) |

## Checked, no defect

- **Generator seam.** Kind and `Transported` refusal, name dedupe, and reserved names (`plugin.go:41`). Ownership manifest, refusal of authored or edited files, protection of reserved core outputs, stale removal (`internal/gen/plugins.go`).
- **CLI seam.** Pinned IDs refuse non-slot providers; an unavailable pin only warns (`cmd/skgo/templates.go:111-121`). The selected module graph must match with no replace (`cmd/skgo/plugin_project.go:34-55`). The digest is re-verified on every load (`plugininstall.InstalledPath`).
- **Release independence.**
  - `TouchesCore` filters per commit.
  - `release.yml` describes `v*` tags only, and the updater reads `releases/latest`.
  - Native publish uses `--latest=false`, refuses an existing tag, verifies the gate run, the candidate run, the digest and the source SHA, and re-verifies the downloaded bytes (`native-plugin.yml:107-168`).
- **Signing and build pairing.** The official CLI carries `disable-library-validation`. `pluginbuild` drops replaces and uses `GOTOOLCHAIN` set to the host's version.
- **Updater.** Native completion flags are removed, and the message says plugin updates are explicit.

## Findings

1. **nitpick — Agent-local cache paths committed to `.gitignore`.** `.gitignore:41,46,48` add `.cache/provisional/`, `.cache/final-current/` and `.cache/final-reviewed/`. These are this session's scratch directories, not project layout. `.cache/` conventions belong to whoever produces them; drop the three lines or use a single generic pattern.
2. **nitpick — Plugin entry points are still partly untested.** `plugins/native-shell/plugin_test.go:12` covers only `Generate`. `Describe` (`plugin.go:31-34`: `Generator=true`, command summary) and `RunCommand` (`plugin.go:74`) have no unit test. That includes the subcommand dispatch and the decision that a bare invocation exits 0 (the old `skgo native` exited 2). Only the deferred gate and `TestCapabilityPluginActualCLI` touch them.
3. **nitpick — Example can hang the gate silently.** `plugins/native-shell/example/native/Sources/ApplicationView.swift:8`: `guard let origin = host.url?.absoluteString else { return }` returns without printing or terminating. If the host fails to come up, the process never exits and the gate relies on an external timeout. It should print to stderr and `exit(1)`.
4. **nitpick — Example pins an unpublished core with no sum, and nothing compiles it in development.** `plugins/native-shell/example/go.mod:9` requires `github.com/tylergannon/skgo v0.41.0` with no `go.sum` entry. It compiles only with the local proxy under `.cache/final-reviewed/proxy`. No development workflow builds the example, so drift appears only in the deferred private gate.
5. **nitpick — Development CI never builds the `.so`, and its path filters miss the builder.** `.github/workflows/native-plugin-tests.yml:9-23` omits `internal/pluginbuild/**` and `cmd/skgo-plugin-build/**`. No workflow outside the dispatch-only release candidate runs `skgo-plugin-build` against native-shell. A change to the builder or to plugin-mode compatibility is first seen at release time.

## Ignored narrowing

None. The operating messages (pair hashes, environment, the fixed base after HEAD advances, plan snapshots kept out of the commit) were treated as constraints, not narrowing. The claim that runtime Go, Swift and Zig code is unchanged since round 04 was not taken on trust. It was partly checked by the byte match of the embedded resources, and the runtime behaviour was re-proved above.

## Outcome

only nitpicks remain
