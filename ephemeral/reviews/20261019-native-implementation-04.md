# Native plugin implementation review — round 04

## Target

The whole current native-shell extraction on `codex/native-plugin`:

- the core diff against `fa29e01a`;
- all untracked source: `plugins/native-shell/**`, the new core files and their
  guides, and `.github/workflows/native-plugin{,-tests}.yml`;
- the provisional pair in `.cache/final-reviewed/`, built from the frozen tree
  through a private module proxy. This is not official release qualification.

| Item | Path | SHA-256 |
|---|---|---|
| Host | `.cache/final-reviewed/bin/skgo` | `4d11e9cd…587f5f7` (verified) |
| Plugin | `.cache/final-reviewed/native-shell-v0.1.0-skgo-v0.41.0-darwin-arm64.so` | `fe4dde3a…a44802fc` (verified) |

The specification is `AGENTS.md`,
`ephemeral/native-plugin/native-plugin-{decisions,contracts,plan}.md` and the
adjudications in `ephemeral/worklog/20261019-native-plugin-implementation.md`.
Prior rounds are `20261019-native-implementation-0{1,2,3}.md` and
`20261019-native-runtime-01.md`. The worklog rejected round 03 #5 (a main-only
publication rule) as an unrequested release rule. I accept that adjudication and
do not re-raise it.

**Freeze held.** I took SHA-256 hashes of every tracked and untracked file
(excluding `ephemeral/reviews/`) at the start, and checked them again at the
end. Only the worklog changed: it gained one `review:` line recording this
pair.

The host's module source in
`.cache/final-reviewed/modcache/github.com/tylergannon/skgo@v0.41.0` is
byte-identical to the tree for every touched core file I compared:

- `cmd/skgo/{templates,plugin,plugin_project}.go`;
- `internal/gen/{plugins,gen}.go`;
- `internal/plugininstall/prebuilt.go`;
- `internal/templates/{commands,discover}.go`;
- `templateapi/generator.go`;
- `internal/pluginbuild/build.go`.

## Method

- **Runtime:** I drove the actual pair myself in a disposable copy of
  `.cache/final-reviewed/example` under the session scratchpad, outside every
  gitignored ancestor. Before starting I deleted its pin, `Generated.swift`,
  output record, `.skgo/` and `native/build`. Every command used the requested
  `GOWORK`, `GOMODCACHE`, `GOPROXY`, `GONOPROXY` and `GONOSUMDB` values, plus a
  fresh `HOME` per scenario.
- **Core code:** I read the full core diff myself. I ran `go vet` and `go test`
  on `cmd/skgo`, `internal/{gen,plugininstall,templates,pluginbuild,releaseversion,update,webbuild}`
  and `templateapi`; all passed.
- **Native module and workflows:** an independent helper pass. It reported
  that `GOWORK=off go vet ./... && go test ./...` pass, and that
  `go test -tags skgo_qualification -count=1 ./...` passes with Zig 0.17 and
  `swiftc`, including `TestZigRemoteCalls` and the Darwin Swift compile
  witness. I reproduced or read the code behind every helper finding I adopted
  below.

## Observed behavior through the actual CLI

| Step | Exit | Observed |
|---|---|---|
| Ordinary `go generate ./...` before install | 0 | `skgo_gen.go` is byte-identical (`fb505615…`). No `Generated.swift`. |
| `install --prebuilt … --sha256 000…` | 1 | `checksum mismatch: got fe4dde3a…`. No pin written. |
| `install --prebuilt ../native-shell-…so --sha256 fe4d… --select …#readValue --select …#writeValue` | 0 | Pin stores a relative artifact path. Slot is `prebuilt/<host-sha>/<artifact-sha>/`. |
| `plugin native-shell help` / no arguments | 0 / 0 | Lists `setup`, `prepare`, `build` and `sdk`. |
| `plugin native-shell build --platform macos` straight after install | 0 | 92 s. Writes `native/Generated.swift` and `internal/skgo/skgo.plugin-outputs.json` through the host, then `BUILD SUCCEEDED`. |
| `TinyNative 'r04-b84ac27e1c6e-ñ "q" \ end'` (value generated at launch) | 0 | stdout is exactly `initial`, then the supplied value (checked with `cmp`). stderr is empty. |
| Go mutation: initial value `seed-c7949579`, command stores `"go:"+next`, rebuild, launch `mut-0d2e94ea` | 0 | `seed-c7949579`, then `go:mut-0d2e94ea`. Neither string is in any Swift source. The generated Swift calls reach the real Go service. |
| Fresh `HOME`: `skgo help`, `skgo add -h`, `skgo plugin --help` | 0 / 0 / 0 | Each prints a `warning: plugin native-shell unavailable; run skgo plugin restore`. **Round 03 #2 is fixed.** |
| Fresh `HOME`: `plugin native-shell help` | 1 | `unavailable; run skgo plugin restore`. |
| `plugin restore`, then `generate --plugins` | 0 / 0 | Installs. The output mtime is unchanged on the identical regeneration. |
| Ordinary `go generate` after plugin generation | 0 | `Generated.swift` and its output record survive. |
| Append `// hand edit` to `Generated.swift`, regenerate | 1 | `authored or edited; refusing to overwrite it`. The edit is preserved. |
| Same `.so` bytes copied into `SKGO_PLUGIN_DIRS`, then a command and `generate --plugins` | 1 / 1 | `pinned plugin native-shell requires its verified artifact; refusing provider …/manual/x.so`, then `could not load its verified artifact`. **Round 03 #1 is fixed.** |
| App `go.mod` selects `v0.40.1`, then a command | 1 | `application selects skgo v0.40.1; plugin host is v0.41.0`. |

Artifact contents:

- No `/Users/` strings.
- No Zig product trees, only the Swift source literal `../core/zig-out/lib`.
- `dep github.com/tylergannon/skgo v0.41.0 h1:Ev2sF…`, the same sum as the host.
- Every dependency shared with the host has the same version.
- `-trimpath=true`, `CGO_ENABLED=1`, `-buildmode=plugin`.

**The provisional tiny gate passes on the reviewed pair:**

1. The first line printed is the known initial value from a real query.
2. A generated Swift command then changes the value.
3. The second line is a real query result equal to the value supplied at
   launch.

This remains provisional local evidence. The official executable and
skgo-project's private gate are deferred and external.

## Prior findings

| Finding | Status |
|---|---|
| 03 #1: pin shadowed by an unverified same-ID build | **Fixed.** `cmd/skgo/templates.go:84-121` refuses any provider for a pinned ID whose path is not the verified slot. `pinnedEntry` matches the path. Verified by the runtime above and by `TestCapabilityPluginActualCLI`. |
| 03 #2: an unrestored pin breaks unrelated help/add | **Fixed.** Verified at runtime. |
| 03 #3: native witnesses do not run on development revisions | **Fixed.** `native-plugin-tests.yml` has a macOS `compiler-and-wire` job: tagged `go test`, `zig build test library` and `swift test`. Its path filters cover the core packages the tagged tests reach. The `native` filter in `ci.yml` now includes `cmd/skgo/`, `internal/templates/` and `templateapi/`. |
| 03 #4: README/introduction promise removed behavior | **Fixed** for those two passages. Related stale text remains (findings 1 and 2). |
| 03 #6: dead `nativehost` duplicate | **Fixed.** Deleted. `nativeapp/files/native/loopback/host_test.go` tests the shipped scaffold. |
| 03 #7: byte-identical authored file adopted | **Fixed.** `checkPluginOutput` refuses any existing file without a prior record (`internal/gen/plugins.go:291-310`). |
| 03 #8: `sdk --out` semantics and migration list | **Mostly fixed.** Only the deleted guide pages are still unlisted. |
| 03 #9 to #12: nitpicks | Mostly unchanged. Folded into nitpicks 4 and 5 where still relevant. |

## Findings

### 1. Issue — removing the native-identity guard is an unrecorded subtraction, and the README still promises it

**Where:**

- The diff deletes this check from `internal/templates/project.go:73-75`. For
  an older project without `skgo.json` but with a `native/` directory, it
  refused to proceed:

  > older project has native identity: verify its display name and bundle
  > configuration, then create skgo.json …

- `README.md:369-371` still says:

  > Older projects must supply `skgo add --name NAME` explicitly; existing
  > native identity requires verification before creating that file.

**What is missing:**

- No guide, worklog entry or subtraction list mentions the removal:
  - `native-shell.md:75-82`;
  - `templateapi/plugin-capabilities.md:45`;
  - the worklog.
- No test replaces it.

**Why it matters:**

- Dropping native knowledge from core fits the contract ("no
  native-shell-specific dispatch"), so the removal itself is defensible.
- But the decisions say "Record what was removed; do not hide it behind a
  preservation claim".
- Instead the README still advertises the protection. An older native
  consumer such as Gimbal, which has no `skgo.json`, now gets an `skgo.json`
  written with whatever `--name` it passes. Nothing verifies its native display
  identity first, while the README says something does.

**Fix:** record the subtraction in the guide's deliberate-change list, and
correct `README.md:370-371`.

### 2. Issue — the shipped Swift SDK README tells developers to run commands that no longer exist

**Where:** `plugins/native-shell/sdk/swift/README.md:6-24`.

**What it says:**

- "Build and test from the SKGo checkout", with:
  - `mise -C native install`
  - `mise -C native/core exec -- zig build test install`
  - `swift test --package-path native/swift`
  - `go test -count=1 -race ./native`
- "Linux runs the Zig handler integration. CI runs both platforms."

**What is true now:**

- None of those paths exist. The SDK is at `plugins/native-shell/sdk`.
- The Go witnesses need `-tags skgo_qualification` and live in `./sdk`.
- The former Linux `TestZigRemoteCalls` run was dropped. Only the macOS
  `compiler-and-wire` job remains, and that drop is not recorded either.

**Why it matters:**

- This file is embedded in the artifact (`plugins/native-shell/plugin.go:24`)
  and copied into every `skgo plugin native-shell sdk --out` export and every
  `.sdk` build directory.
- The client-only developer the extraction serves is therefore handed
  instructions that fail, plus a CI claim that is false.
- The worklog's `doc_bug` entry says this README was corrected. Only the
  numeric-guard and `--swift` text was. The correct commands already exist in
  `sdk/core/README.md:18-21`.

### 3. Nitpick — `skgo add native-app` survives through the plugin, contradicting the guides

**Where:**

- `plugins/native-shell/nativeapp/plugin.go:25-27` still declares the
  `native-app` add-on.
- `plugins/native-shell/plugin.go:30-36` forwards its descriptor and `Apply`.

**Observed:** with the pin restored, `skgo add -h` lists `native-app: Add an
embedded Go host and shared macOS/iOS application shell`.

**What the guides say:**

- `native-shell.md:81`: the former `skgo add native-app` setup "moves to
  `skgo plugin native-shell setup`".
- `plugin-capabilities.md:45`: lists "built-in `native-app`" as removed.

Both entry points now work. This is harmless as a feature, and only the built-in
was removed, but the docs misstate it. Either drop the add-on descriptor, or say
the add-on remains through the plugin.

### 4. Nitpick — example rough edges carried from round 03

- **Unpublished core:** the committed `go.mod` requires the unpublished
  `skgo v0.41.0` with no go.sum line, and no development workflow compiles the
  example.
  - The helper compiled it with a scratch `-modfile` replace, and it passed.
  - Drift against core APIs is still invisible until gate time.
  - If core does not ship exactly as v0.41.0, the committed version names a
    release that never exists.
- **Possible hang:** if `NativeHost.url` stays nil
  (`native/Sources/NativeHost.swift:22`, or the guard in
  `ApplicationView.swift:8`), the launch-argument path prints nothing and never
  exits. The private gate needs its own timeout.

### 5. Nitpick — agent-local and test-coverage residue

- **Agent-local ignores:** the root `.gitignore` gains `.cache/provisional/`,
  `.cache/final-current/` and `.cache/final-reviewed/`. These are this session's
  scratch directories, not product paths, and they will outlive it. The
  example's own `.gitignore` already covers its outputs.
- **Untested plugin entry points:** no development test calls the native
  plugin's `Describe` or `RunCommand`.
  - Removing `d.Generator = true` (`plugin.go:34`) would fail nothing before the
    release gate.
  - The bare `skgo plugin native-shell` exits 0 with help, where the former
    `skgo native` exited 2.
- **No plugin build on development revisions:** `native-plugin-tests.yml` never
  builds the `.so` (`-buildmode=plugin`, embed set). Its path filters also omit
  `internal/pluginbuild/**` and `cmd/skgo-plugin-build/**`. Within the
  agreed "the gate is the release authority" rule this is acceptable, but it is
  a development blind spot.

### Checked, no defect found

- **Import boundary:** the native module's non-test graph reaches only
  `templateapi` from core. Its tagged tests import core internals, as already
  adjudicated.
- **Generator seam:**
  - selection validation and duplicate rejection;
  - path validation: absolute paths, `..`, `.git`, `node_modules` and symlink
    traversal;
  - reserved core outputs;
  - ownership records and stale removal with hash checks;
  - preflight before publication;
  - plugin generation bypasses the persistent cache;
  - ordinary `go generate` leaves plugin output alone.
- **CLI seam:**
  - management names are reserved;
  - descriptors that advertise a capability without implementing it are
    rejected;
  - duplicate providers are rejected;
  - argument/stream forwarding, panic recovery and cancellation work.
- **Exact artifact:**
  - the name pattern, semver, checksum, host version, official module,
    `-trimpath` and cgo checks, and GOOS/GOARCH match;
  - an ABI probe with ID and version verification;
  - slots keyed by host digest;
  - HTTPS-only redirects.
- **Core release independence:** `TouchesCore` excludes `plugins/` and the two
  native workflows from root versioning. The native tag prefix does not match
  `v[0-9]*`.
- **Native workflow bindings:**
  - inputs pass through `env` and regexes;
  - the official archive is checked against `checksums.txt`;
  - publication binds the gate run, candidate run, digest, versions and source
    SHA, refuses existing tags, uses `--latest=false`, and verifies the
    published bytes.
- **Extraction fidelity:** `swiftgen` and the SDK tests are byte-identical to
  the base copies. The `nativebuild` changes are migration-only.
- **Example minimality:** one value, one query and one command. No test-only
  endpoint, pass flag or expected-value receipt. Generated and build outputs are
  ignored.

## Ignored narrowing

None. The request supplied operating constraints only: a read-only review, the
artifact path, the environment, and the provisional status of the artifacts.

## Outcome

material findings remain
