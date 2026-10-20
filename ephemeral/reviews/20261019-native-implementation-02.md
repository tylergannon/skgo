# Native plugin implementation review — round 02

## Target

The complete current native-shell extraction on `codex/native-plugin`:

- the core diff against `fa29e01a`;
- all untracked source under `plugins/native-shell/`, including `sdk/`,
  `nativeapp/`, `nativebuild/`, `nativehost/`, `swiftgen/`, `example/` and
  `native-shell.md`;
- the new core files: `cmd/skgo/plugin_project.go`,
  `cmd/skgo/capability_commands_test.go`, `internal/gen/plugins.go`,
  `internal/plugininstall/prebuilt.go`, `internal/templates/commands.go`,
  `templateapi` capability files and guides;
- `.github/workflows/native-plugin.yml` and `native-plugin-tests.yml`;
- the provisional pair:
  - artifact `.cache/provisional/final/native-shell-v0.1.0-skgo-v0.41.0-darwin-arm64.so`,
    sha256 `4539be1c5a74c117a56ff807ee186212f87ea7acc9638656e567f78e21b9b620`;
  - host `.cache/provisional/bin/skgo`, sha256 `d2d86338…705d2e`.

The specification is `AGENTS.md` together with the three
`ephemeral/native-plugin/native-plugin-{decisions,contracts,plan}.md` files.
The prior rounds were `20261019-native-implementation-01.md` and
`20261019-native-runtime-01.md`. This round reviewed the whole implementation
unconditionally.

## Evidence inspected

- **Planning and repo documents, read in full:** the decisions, contracts and
  plan documents, `AGENTS.md`, both earlier reviews, and the worklog
  `ephemeral/worklog/20261019-native-plugin-implementation.md`.
- **Core diff:**
  - Built-in native removal: `cmd/skgo/native.go`, the `--swift-*` flags, the
    Swift cache fields, the `emit.go` native guards, `completeNative`, the
    webbuild native generation, the Gimbal exception and the native identity
    check.
  - New plugin surfaces: capability dispatch (`cmd/skgo/plugin.go` and
    `internal/templates/commands.go`) and generator publication
    (`internal/gen/plugins.go`).
  - Install path: the prebuilt installer and pins
    (`internal/plugininstall/prebuilt.go`) and the application check
    (`cmd/skgo/plugin_project.go`).
  - Build and release: pluginbuild source copying and ldflags, and
    releaseversion's exclusion of `plugins/`.
- **Native module:** `plugin.go` in full, `nativebuild/build.go` in full, and
  `example/` (go.mod, README, `.gitignore`, `internal/skgo/config.go`,
  `value.remote.go`, `ApplicationView.swift`).
- **Dependency boundary:** `go list -deps` shows the plugin's non-test graph
  reaches only `github.com/tylergannon/skgo/templateapi` from core.
- **Guides:** `plugins/native-shell/native-shell.md`,
  `templateapi/{generator-plugins,plugin-capabilities,create-your-own}.md` and
  `cmd/skgo/plugin.md`.
  - All have the required frontmatter.
  - Their `go`/`text` fences are empty and name `ExampleGenerator`,
    `ExampleCommandPlugin`, a repo path, or `skgo plugin -h`.
  - Both named Example functions exist.
- **Release builder:** `cmd/skgo-release/main.go`. The official host is
  `go install -trimpath` with `CGO_ENABLED=1` and `GOWORK=off`, which matches
  the new prebuilt host check.
- **Tests:** these passed:
  - `go vet` on the touched core packages.
  - `go test ./cmd/skgo/ ./internal/gen/ ./internal/plugininstall/ ./internal/templates/ ./templateapi/ ./internal/releaseversion/ ./internal/update/ ./internal/webbuild/`.
  - `GOWORK=off go test ./...` in `plugins/native-shell`.
- **Prior findings rechecked:**
  - Native tests now run on development revisions through
    `native-plugin-tests.yml`.
  - No fabricated `tylergannon/skgo` go.sum lines remain.
  - `swiftRemotes` is gone; the pin is the only selection source.
  - The example contains no `{{…}}` placeholders.
  - Relative pin storage is implemented and unit-tested at `prebuilt_test.go:60`.

## Observed behavior through the actual CLI

All runtime commands used the requested `GOWORK`, `GOMODCACHE`, `GOPROXY`,
`GONOPROXY` and `GONOSUMDB` values, plus an isolated `HOME`. They ran in a
disposable copy of `.cache/provisional/final/example`, after removing its pin,
`Generated.swift`, ownership record and `TinyNative.app`.

| Step | Exit | Observed |
|---|---|---|
| `skgo plugin -h` before install | 0 | Usage only. |
| `skgo plugin install --prebuilt ../native-shell-…so --sha256 4539… --select readValue --select writeValue` | 0 | Installed into `~/.skgo/plugins/prebuilt/<host-sha>/<artifact-sha>/`. The pin's `artifact` was written as an **absolute** path (see finding 2). |
| `skgo plugin` | 0 | Lists `native-shell: Generate Swift clients…`. |
| `skgo plugin native-shell help` | 0 | Prints the command help. |
| Ordinary `go generate` in `internal/skgo` | 0 | Core outputs unchanged; no Swift written. |
| `skgo generate --plugins --web ../../web --out .` | 0 | Writes `native/Generated.swift` and `skgo.plugin-outputs.json`. Core `skgo_gen` hashes are byte-identical to ordinary generation. |
| `skgo plugin native-shell build --platform macos` | 0 | About 62 s, `BUILD SUCCEEDED`. |
| `TinyNative 'rev02-0268790ea740-ñ "x" \ end'` | 0 | stdout `initial`, then exactly `rev02-0268790ea740-ñ "x" \ end`; stderr empty. |
| Go mutation: `value = "seed-156e50af05"`, command stores `"go:"+next`, rebuild, launch `mut-0523b74c` | 0 | `seed-156e50af05`, then `go:mut-0523b74c`. The generated Swift reaches the real Go service, and the printed values come from Go, not the app. |
| Append `// hand edit` to `Generated.swift`, rerun `generate --plugins` | 1 | `…Generated.swift is authored or edited; refusing to overwrite it`. The file is left intact. |
| `plugin restore` with a fresh `HOME` | 0 | Reinstalls the same slot path from the pin and verifies the checksum. |
| `plugin install --prebuilt …` in the committed source example | 1 | `prebuilt plugins require an official skgo dependency without a local replacement`. No pin is written (see finding 3). |

The provisional pair's tiny gate behaves as the contract describes: an
independent launch value, a real query before and after, and a generated
command in between.

## Findings

### 1. Issue — the released `.so` embeds Zig build products from the workspace (incorrect implementation)

The embed and the source copy both let workspace build products through:

- `plugins/native-shell/plugin.go:24` has `//go:embed sdk/core sdk/swift …`.
  `go:embed` excludes only `.`- and `_`-prefixed names, so it takes in
  `sdk/core/zig-out/` and `sdk/core/zig-pkg/` whenever they exist.
- `internal/pluginbuild/build.go:331-334` copies the plugin source into the
  isolated build, and its skip list (`.git`, `node_modules`, `.cache`,
  `.build`, …) omits `zig-out` and `zig-pkg`.
- The release candidate job runs `mise exec -- zig build test library` in
  `sdk/core` (`.github/workflows/native-plugin.yml:47-49`) *before*
  `skgo-plugin-build` (line 73). Every published candidate therefore carries
  whatever that step left behind.

The provisional artifact already contains them:

- `strings` on it shows `sdk/core/zig-out/lib/libskgo_native_core.a` and the
  whole `sdk/core/zig-pkg/devalue-0.1.0-…/` tree (tests, testdata, `AGENTS.md`,
  `bench.zig`).
- On disk these are 6.1 MB and 328 KB.
- The artifact is 16 MB, against roughly 9.4 MB earlier.

They are dead weight. `nativebuild/build.go:318` and `:346` explicitly skip
`zig-out` and `zig-pkg` when they materialize the SDK, so no consumer ever uses
them.

**Impact:**

- The published bytes depend on step order and on runner state, not on
  `source_sha`. The same commit yields different digests depending on what ran
  first.
- The artifact ships a runner-built static library and a vendored dependency
  checkout that nothing reads, and it roughly doubles in size.

**Fix:** narrow the embed to the source the SDK needs (`build.zig`,
`build.zig.zon`, `src`, Swift `Package.swift`, `Sources`, `Tests`), or drop
these directories in the source copy.

### 2. Issue — the provisional host predates the current core, so runtime evidence does not cover the revisions (incomplete requirement)

The timestamps show the gap:

- The host `.cache/provisional/bin/skgo` and the proxy's `v0.41.0.zip` date
  from 17:37.
- Core sources changed until 17:51: `prebuilt.go` at 17:51:25,
  `plugin_project.go` at 17:41, and `gen/plugins.go` at 17:40.

Diffing modcache `skgo@v0.41.0` against the tree shows the host lacks these
revisions:

- the trimpath/cgo host check (`prebuilt.go:136-138`);
- relative pin storage (`:193-201`, `:285-289`);
- the wrapped probe error (`:342`);
- `reservedPluginOutput` and the context preflight in `gen/plugins.go`;
- the SIGTERM context;
- `pluginsAt`;
- the `ProjectName` change, plus more.

The runtime showed the stale behavior directly. The installed pin stored the
artifact as an absolute path, although the worklog states that pins now store
project-relative paths. The current code does do that, and
`prebuilt_test.go:60` asserts it.

**Impact:** the build and launch evidence above qualifies a pre-revision core.
Revisions that touch install, restore and generation have only Go-test
coverage, never an actual CLI run. A pairing built from today's tree also needs
a new host and a new `.so`, since a plugin will not load into a host built from
different package sources. Rebuild both from the current tree, then rerun the
tiny gate before treating this candidate as current.

### 3. Issue — the committed example cannot run on any official pairing without unspecified source rewriting (incomplete requirement)

The committed example is pinned to the checkout:

- `plugins/native-shell/example/go.mod` has
  `replace github.com/tylergannon/skgo => ../../..`.
- `web/package.json:14` links `../../../../internal/adapter`.

`checkPluginApplication` (`cmd/skgo/plugin_project.go:33-…`) rejects any
replacement. Run in the committed example, install fails with
`prebuilt plugins require an official skgo dependency without a local replacement`.

The README instead tells the qualifier to change the dependency, remove the
replace, swap in "the corresponding released adapter" and run `vp install`.
Those steps cannot run frozen, because the lockfile records the link. That is a
hand transformation that lives nowhere in code.

**Impact:**

- The release contract makes "the tiny app in `plugins/native-shell/example`"
  the complete gate, and publication authorizes `source_sha`. Yet the source
  that actually runs is a rewritten one: a different `go.mod`, a new `go.sum`
  resolved from the network, a different `package.json`, a regenerated
  lockfile.
- Which adapter version is "corresponding" is left for the deferred
  skgo-project gate to invent.

This is not a reason to add machinery. But the deferred gate must own and
record this transformation explicitly, or the example must reach a releasable
pairing with one mechanical step. Otherwise the gate's evidence does not
describe the committed example.

### 4. Nitpick — release-time SDK qualification tests the checkout's core, not the paired official core

`plugins/native-shell/sdk/{binary,remote}_test.go` (tag `skgo_qualification`)
import core internals: `internal/ssr`, `internal/adapter`, `internal/remotearg`,
`internal/formdata`, `internal/kithash` and `internal/envspec`.

The candidate job runs them with the module's `replace github.com/tylergannon/skgo => ../..`
(`native-plugin.yml:42-46`). They therefore exercise the SDK's wire format
against the core at the dispatch ref, which can be newer than `core_version`,
the core the artifact actually pairs with.

**Impact:** low, because the tiny gate exercises the real pairing. Still,
"qualification" there can pass against a core the artifact never runs with.

## Outcome

material findings remain
