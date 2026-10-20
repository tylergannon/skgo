# Native plugin implementation review — round 03

## Target

The whole current native-shell extraction on `codex/native-plugin`:

- the core diff against `fa29e01a`;
- all untracked source: `plugins/native-shell/**`, the new core files and their
  guides, and `.github/workflows/native-plugin{,-tests}.yml`;
- the provisional pair, both built from the current tree through a private
  module proxy. This is not official release qualification.

| Item | Path | SHA-256 |
|---|---|---|
| Host | `.cache/final-current/bin/skgo` | `78548c50…eab379` |
| Plugin | `.cache/final-current/native-shell-v0.1.0-skgo-v0.41.0-darwin-arm64.so` | `83ff46be…3c00f` |

Host buildinfo: `skgo v0.41.0 h1:ODxzT…`, `-trimpath=true`, `CGO_ENABLED=1`.

The specification is `AGENTS.md` and
`ephemeral/native-plugin/native-plugin-{decisions,contracts,plan}.md`. Prior
rounds were `20261019-native-implementation-0{1,2}.md` and
`20261019-native-runtime-01.md`.

**Source was not entirely frozen during the review.** The prepared example's
`go.mod` and `go.sum` and its frontend pins were rewritten in place in
`plugins/native-shell/example` between 18:08 and 18:12. The source example
now equals `.cache/final-current/example` minus the two private-proxy
`skgo v0.41.0` go.sum lines, which the worklog says are to be removed. This
review describes the tree as of 18:17.

## Method

- **Runtime:** I drove the actual pair myself in disposable copies under the
  session scratchpad. None of them are under the `.cache` ancestor. Every
  command used the requested `GOWORK`, `GOMODCACHE`, `GOPROXY`, `GONOPROXY` and
  `GONOSUMDB` values, plus a fresh `HOME` per scenario.
- **Core code review:** an independent Opus pass, which also ran scratch
  experiments in a repository copy.
- **Native module and release review:** a separate independent Opus pass.

I reproduced or read the code behind every finding below that I adopted from
those passes. Each finding says which.

## Observed behavior through the actual CLI

The copy is of `.cache/final-current/example`, with its pin, `Generated.swift`,
output record, `.skgo/` and `native/build` deleted first.

### Before install

| Step | Exit | Observed |
|---|---|---|
| Ordinary `go generate ./...` in `internal/skgo` | 0 | `skgo_gen.go` is byte-identical. No `Generated.swift`. |
| `install --prebuilt … --sha256 000…` | 1 | `checksum mismatch: got 83ff46be…, expected 000…`. No pin written. |
| Same bytes named `…-skgo-v0.40.1-…` | 1 | `requires official skgo v0.40.1; executing host is v0.41.0`. |
| Same bytes named `native-shell-v0.1.1-…` | 1 | `artifact declares version "v0.1.0", expected "v0.1.1"`. |
| Same bytes named `renamed.so` | 1 | Naming error. |
| Same bytes named `…-linux-amd64.so` | 1 | `does not target executing host darwin/arm64`. |

### Install and generation

| Step | Exit | Observed |
|---|---|---|
| `install --prebuilt ../native-shell-v0.1.0-skgo-v0.41.0-darwin-arm64.so --sha256 83ff… --select …#readValue --select …#writeValue` | 0 | Pin artifact stored as `../native-shell-…so` (relative). Slot is `~/.skgo/plugins/prebuilt/<host-sha>/<artifact-sha>/`. |
| `skgo plugin` / `plugin native-shell help` | 0 / 0 | Lists the provider. Help lists `setup`, `prepare`, `build` and `sdk`. |
| `plugin native-shell bogus` / `plugin nosuch` | 1 / 1 | Clear errors. |
| `skgo generate --plugins --web ../../web --out .` | 0 | Writes `native/Generated.swift` (`1e665de1…`) and `skgo.plugin-outputs.json`. Core `skgo_gen.go` is unchanged. |
| Repeat generate | 0 | No writes; mtimes unchanged. |
| Ordinary `go generate` with the pin present | 0 | Plugin-free. |

### Build and launch

| Step | Exit | Observed |
|---|---|---|
| `skgo plugin native-shell build --platform macos` | 0 | 76 s, `BUILD SUCCEEDED`. |
| `TinyNative 'r03-50bd37fa802a-ñ "q" \ end'` (value generated at launch) | 0 | stdout is exactly `initial`, then the supplied value byte for byte. stderr is empty. |
| Go mutation: initial value `seed-dfa1e31f`, command stores `"go:"+next`, rebuild (10 s), launch `mut-a3d36c7f` | 0 | `seed-dfa1e31f`, then `go:mut-a3d36c7f`. Generated Swift reaches the real Go service. Neither printed value exists in the Swift or in the launch argument as printed. |
| SIGINT 4 s into a build | 1 | `skgo build: signal: killed`. No surviving children. |

### Protection and restore

| Step | Exit | Observed |
|---|---|---|
| Append `// hand edit` to `Generated.swift`, regenerate | 1 | `authored or edited; refusing to overwrite it`. The edit is preserved. |
| Append a byte to the installed slot `.so`, then generate, then run a command | 1 / 1 | `installed plugin native-shell checksum mismatch; run skgo plugin restore`. |
| Fresh `HOME`, then `plugin native-shell help` | 1 | `unavailable; run skgo plugin restore`. |
| `restore` with the local artifact missing | 1 | `open …: no such file`. |
| `restore` with the artifact replaced by different bytes | 1 | `checksum mismatch`. |
| `restore` with the artifact present | 0 | Slot installed, then generate runs. |

### Selections and the application check

| Step | Exit | Observed |
|---|---|---|
| Pin selections narrowed to `readValue` | 0 | `writeValue` is absent from the output. |
| Selections empty | 0 | Emits all remotes, the same `1e665de1…`. |
| Selection `#nope` | 1 | Unknown selection. |
| App `go.mod` selects v0.40.1, then a command, then generate | 1 / 1 | `application selects skgo v0.40.1; plugin host is v0.41.0`. |
| App `go.mod` selects v0.40.1, then `restore` | 0 | Installs. Restore does not check the app; use does. |
| App `go.mod` with `replace … => ../x` | 1 | `require an official skgo dependency without a local replacement`. |
| `go.work` using the app and a core checkout | 1 | Rejected, but the message reads `application selects skgo ;` with an empty version. |
| Project-built `go tool skgo` (command, restore, generate) | 1 | `require the cgo-enabled, -trimpath official skgo executable`. |

### Client-only use, no `native/` directory at all

| Step | Exit | Observed |
|---|---|---|
| `install … --set out=Client/API.swift`, then `generate --plugins` | 0 | Writes `Client/API.swift`. |
| `plugin native-shell sdk --out Client/SDK` | 0 | 188 KB of SDK source, no build products. |
| `--out ../../escape`, `--out /tmp/abs` | 1 | Refused. |
| Same commands through the `/tmp` symlinked path | 0 | Root is resolved. |
| `build` without a shell | 1 | `open …/native/skgo-native.json: no such file`. |

### Artifact contents

- `strings` finds no `zig-out`/`zig-pkg` trees, only the Swift source string
  `"../core/zig-out/lib"`.
- No `/Users/` paths.
- 9.4 MB.
- `go version -m` shows `dep github.com/tylergannon/skgo v0.41.0 h1:ODxzT…`
  with no replacement.

**The provisional tiny gate passes on the current pair:**

1. The first line printed is the known initial value from a real query.
2. A generated command then changes the value.
3. The second line is a real query result equal to a value supplied
   independently at launch.

This is still provisional local evidence. The official executable and
skgo-project's private gate remain external.

## Prior findings

| Finding | Status |
|---|---|
| 01 #1: native tests run nowhere | **Partly fixed.** Plain `go test` runs; the qualification witnesses still do not run (new finding 3). |
| 01 #2: fabricated skgo go.sum | **Fixed** in the tree as of 18:10. The lines exist only in the `.cache` copy. |
| 01 #3: two remote-selection sources | **Fixed.** Selection is pin-only. |
| 01 #4: template placeholders in the example | **Fixed.** |
| 01 #5: absolute pin path | **Fixed** for paths under the project. A distant local artifact still produces a `../../../../var/folders/…` path (nitpick 9). |
| 02 #1: Zig products in the `.so` | **Fixed.** The embed list is narrowed and `pluginbuild` skips product directories. Verified in the artifact. |
| 02 #2: stale host | **Fixed.** The modcache `skgo@v0.41.0` matches the tree for the touched packages, and the runtime above exercised it. |
| 02 #3: example cannot reach an official pairing mechanically | **Fixed in substance.** There is no replace and no `link:`. `skgo update --complete-version VERSION --root .` prepared it (`example-prepare.log`). See nitpick 11 for what remains. |
| 02 #4: SDK qualification imports core internals and runs against checkout core | **Still open.** Folded into finding 3. |
| runtime-01: restore skips the app-core check | **Still open, and harmless.** Use is checked. |

## Findings

### 1. Issue — the checksum pin can be satisfied by a different, unverified build of the same plugin

**Where:**

- `cmd/skgo/templates.go:83-111` (`pluginsAt`). Manual (`SKGO_PLUGIN_DIRS`) and
  managed source-install files load before `pinnedPluginFiles`. A pin is
  "found" by any registry entry with the same `Descriptor.ID` and version.
- `cmd/skgo/plugin_project.go:98-110` (`configureGenerators`). The generator is
  selected by `ID` alone.

**Defect:** Go cannot load a second `.so` with the same pluginpath. The
verified slot therefore fails with `plugin already loaded`, which is demoted to a
`warning:`, and the earlier unverified entry then serves both the pin's
generator and its commands.

**Failure scenario:** a developer once source-installed native-shell through
the legacy path, or has a local build in `SKGO_PLUGIN_DIRS`. They then pin the
released artifact. Every `generate --plugins` and every
`skgo plugin native-shell …` runs the unpinned build. The only signal is a
warning line, and the pin's sha256 has no effect.

**Evidence:**

- The core-review pass ran this through the real CLI with the
  capability fixture. A same-module build in `manual/aaa.so` printed
  `TAMPERED stderr` from the command and wrote `other.txt = "TAMPERED\nv\n"`
  through generation, while the pinned slot got only
  `warning: … duplicate-source load … plugin already loaded`.
- I read both code paths and confirmed the ID-only match.

**Contract:** this contradicts "selects the exact named artifact, verifies
integrity and clearly rejects mismatches" and "duplicate providers must not
silently replace one another".

**Fix:** a pinned ID must resolve to the entry loaded from its verified slot
path. If another source claimed that ID or pluginpath first, fail.

### 2. Issue — an unrestored pin breaks unrelated commands, including `skgo help`

**Where:** `cmd/skgo/templates.go:84-87`, `pluginsAt` returning the
`InstalledPath` error, reached by every command that builds the registry.

**Observed** (fresh `HOME`, inside the example with a committed pin):

| Command | Exit | Observed |
|---|---|---|
| `skgo help` | 1 | Prints usage, then `plugin native-shell unavailable; run skgo plugin restore …`. |
| `skgo add -h` | 1 | Same message. |
| `skgo build -h` / `skgo update -h` | 0 | Unaffected. |

The core pass also saw `skgo add tailwindcss` fail. The same happens after
every host upgrade, because slots are keyed by the host digest, and under a
project-built `go tool skgo`.

**Impact:**

- A teammate's checkout, CI, or the first run after updating skgo cannot even
  read help, or use add-ons unrelated to the pinned plugin.
- The pin should block `--plugins` generation and that plugin's commands only.

### 3. Issue — native compiler and wire witnesses no longer run on any development revision

**Where:**

- `.github/workflows/qualification.yml`: the former `native-linux` and
  `native-macos` jobs dropped `TestZigRemoteCalls` and
  `TestGeneratedModelsCompileAndEnforceTheWireShape`, plus the Zig toolchain
  install.
- `.github/workflows/native-plugin-tests.yml` runs only
  `go test ./...` on `ubuntu-latest`.

**What no longer runs on development revisions:**

- `swiftgen/swift_darwin_test.go` (`darwin && skgo_qualification`).
- `sdk/{binary,remote}_test.go` (`skgo_qualification`).
- `zig build test library` and `swift test`.

All of them run only inside the release candidate job
(`native-plugin.yml:42-52`). There they run against the dispatch ref's
checkout core through `replace => ../..`, not against `core_version`. That is
round-02 #4.

**Failure scenario:** the SDK tests import `internal/ssr`, `remotearg`,
`formdata`, `kithash` and `envspec`.

- A core change to one of those wire formats, or a swiftgen change that emits
  uncompilable Swift, passes every core and native development check.
- It surfaces only when someone dispatches a native release, or not at all if
  the checkout core at that time matches the change.

The tag filtering makes these silent non-runs rather than reported skips.
Under `AGENTS.md`, a check that cannot run has not passed.

**Related:** `ci.yml`'s `native` path filter now gates
`TestCapabilityPluginActualCLI`, the only real-CLI plugin test. That filter
omits `cmd/skgo/` and `internal/templates/`, which is where findings 1 and 2
live. Changes to plugin dispatch, discovery and pin resolution do not trigger
it.

**Fix:** run the existing tagged native tests, including darwin/Zig/Swift, in
`native-plugin-tests.yml` on relevant development revisions. That workflow
should also trigger on core wire packages. Add `cmd/skgo/` and
`internal/templates/` to the `native` filter.

### 4. Issue — user-facing docs still promise removed native behavior and an unproved companion

**`README.md:340-343`** still says:

> Native applications also regenerate Swift bindings and build macOS by
> default; select another build with `--native-platform simulator
> --native-preset synthetic`.

Those flags and that behavior were removed from `update`. Following the text
produces an unknown-flag error.

**`introduction.md:47`:**

- It says "Generated Swift clients for your Go queries and commands are also
  released". After this change no core release supplies Swift clients until a
  native-shell pair is qualified and published.
- It links "Native targets", whose guide was deleted.

The plan requires: "do not claim a native companion exists during the initial
gap". `native-shell.md` gets this right; these two pages contradict it.

### 5. Issue — publication does not bind the candidate to `main`

**Where:** `native-plugin.yml:131-138`.

The candidate run is checked for `conclusion`, `path`, `event` and
`head_sha == SOURCE_SHA`, but not for `head_branch`. `workflow_dispatch` runs
the workflow file from whichever ref is dispatched.

**Failure scenario:**

1. Someone runs `gh workflow run native-plugin.yml --ref some-branch`.
2. The job builds unmerged source with that branch's own candidate job
   definition.
3. If the gate passes, publish creates the immutable
   `plugins/native-shell/vX` release and tag at a commit not on `main`.

That tag is also a Go nested-module version (nitpick 12). The gate's checks
live in skgo-project and were not reviewed. The publisher itself should
require `head_branch == "main"`, or that `SOURCE_SHA` is reachable from
`main`.

### 6. Issue — the tested `nativehost` package is a dead duplicate; the shipped loopback host is untested

**Where:**

- `plugins/native-shell/nativehost/`.
- `plugins/native-shell/nativeapp/files/native/loopback/host.go`.

**What is wrong:**

- The bodies are identical apart from the package clause. I diffed them with
  the header lines stripped.
- The scaffold ships `loopback`. Nothing imports `plugins/native-shell/nativehost`
  except its own tests and `native_targets_example_test.go`. That example served
  the deleted `native-targets.md` guide.
- `TestRecoveryRetainsApplicationAndOrigin` exercises only the dead copy.
  `loopback` has no tests.
- The shipped file's doc comment still says `Package nativehost`.

**Failure scenario:** an edit to the scaffolded `loopback/host.go` regresses
recovery. The tests stay green, and every newly set-up shell gets the
regression.

**Fix:** delete `nativehost/` and point its test at the scaffold source.

### 7. Nitpick — a byte-identical authored file is silently adopted, then later deleted

**Where:** `internal/gen/plugins.go:295-298` adopts an existing file whose bytes
equal the proposal. The stale-removal loop (`:248-263`) later deletes it when
the plugin stops producing it, or its `out` changes.

The core pass observed `generated/fixture.txt` deleted this way. The realistic
case is a migrating consumer whose old `--swift-out` file happens to match.

**Fix:** adopt only with a prior record, or leave adopted files in place on
removal.

### 8. Nitpick — `sdk --out` silently discards edits inside its directory, and is an unlisted addition

The `.native-shell-sdk` marker makes the destination disposable:

- appending `// mine` to `Client/SDK/swift/Package.swift` and rerunning `sdk`
  exited 0;
- the line was gone.

This is defensible for a declared copy, but the guide only says "copy".
Developers are told to build the Zig library inside that directory. The
command itself is new. It is the reasonable replacement for SDK sources that
used to live in the core module, but `native-shell.md`'s list of deliberate
migration changes omits it. That list also omits the move of `skgo add
native-app --bundle-id` to `plugin native-shell setup`, and the deleted
`native-targets`, `native-app` and `native-build` guides.

### 9. Nitpick — diagnostics

- Under a `go.work` that covers core, the rejection reads
  `application selects skgo ; plugin host is v0.41.0`. It should say that a
  workspace or local core is not an official pairing.
- A local artifact outside the project is pinned as a long `../../…/var/folders/…`
  relative path. That is meaningless on another machine. The guides already
  recommend HTTPS URLs for shared pins.
- `restore` installs even when the application selects another core.

### 10. Nitpick — plugin command parsing behavior

- The native command with no arguments prints help and exits 0. The former
  `skgo native` exited 2.
- Unknown flags exit 1 rather than 2.
- A plugin cannot choose its exit code.
- No test calls `RunCommand` parsing.

### 11. Nitpick — example rough edges

- **Unpublished core:** the committed `go.mod` requires the unpublished
  `skgo v0.41.0` with no go.sum line.
  - Nothing compiles the example on development revisions. It is outside both
    `go.work` and the native module's `./...`.
  - Its Go can therefore drift against core APIs unseen until gate time.
  - The gate prepares it with `skgo update --complete-version`, which makes this
    acceptable, but if core does not ship exactly as v0.41.0 the committed
    `go.mod` names a version that never exists.
- **macOS only:** `ApplicationView.swift` uses AppKit and
  `NSApplication.shared.terminate`, so `--platform iphone`/`simulator` cannot
  build the example. That is fine for the macOS-only gate.
- **Possible hang:** if `host.url` is nil the launch-argument path prints
  nothing and never exits. The gate needs a timeout.
- **Broken `check` script:** `web/package.json` `check` names a nonexistent
  `tsconfig.json` and an uninstalled `svelte-check`.

### 12. Nitpick — release tag is a Go module version

`plugins/native-shell/vX` makes
`github.com/tylergannon/skgo/plugins/native-shell@vX` resolvable through the
public proxy. That module's go.mod carries `replace => ../..`. This is harmless
but permanent once fetched; the alternative is a non-semver tag prefix.

### Checked, no defect found

- **Plugin build identity:** `pluginbuild` drops the plugin's skgo replace,
  pins every host module, and builds against the official host's exact
  `skgo v0.41.0` module. The artifact's buildinfo confirms this.
- **Workflow inputs:** all inputs and payload fields pass through `env:` and
  regexes. There is no injection.
- **Publish binding:** publish binds the gate run, the authorization JSON,
  the candidate digest, both versions and `source_sha`. It refuses existing
  tags, uses `--latest=false`, and re-downloads and verifies the published
  bytes.
- **Core release boundary:** core release tooling uses
  `git describe --match v[0-9]*`, and `TouchesCore` excludes `plugins/` and the
  native workflows. `skgo update` reads `releases/latest`.
- **Import boundary:** the native module's non-test import graph reaches only
  `templateapi` from core.
- **Example minimality:** one value, one query and one command. No test-only
  endpoint, no pass flag and no expected-value receipt. Generated and build
  outputs are gitignored.
- **Extraction fidelity:** `swiftgen` and `runtime.swift` are byte-identical to
  the removed core copies. The `nativebuild` changes are migration-only.
- **Tests and vet:** `go vet` and `go test` pass on the touched core packages,
  and `GOWORK=off go vet ./... && go test ./...` pass in `plugins/native-shell`.
  These were run by the two review passes.

## Outcome

material findings remain
