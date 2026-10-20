# Native-shell runtime validation 01

2026-10-19 · independent local validation via the CLI and the built app · Claude Opus 5.5

**Verdict: the generated Swift query and command really run against real Go. With the artifact as given, the shipped user flow `skgo plugin native-shell build` fails on the current example.** The failure comes from source drift during this validation. Another session edited the plugin source and the example at 17:45:30, after the provisional `.so` was built at 17:42. The artifact being tested still includes `native/Generated.swift` in the Xcode project only when `skgo-native.json` lists `swiftRemotes`. The example no longer lists them, so the rebuilt Xcode project leaves out the generated client and Swift compilation fails. The candidate needs rebuilding from current source and revalidating before it can stand for the current tree.

## Evidence class

This is **provisional local evidence, not qualification of an official distributed executable.** The host `skgo v0.41.0` is a synthetic versioned build from this worktree, served by a private file proxy. No `v0.41.0` release exists: the latest tag is `v0.40.1`, and `gh release view v0.41.0` says "release not found". The plugin is a local candidate. Its embedded module version is `(devel)`, so its `v0.1.0` identity comes only from the file name and the pin. The independent published-pair and private release gate (skgo-project, GitHub-hosted macOS ARM64, `repository_dispatch` publisher) is outside this validation and was not exercised.

## Environment and artifact identity

```text
macOS 27.0.1 (26A434), arm64; Xcode 27.0 (27A266a); go1.27.2 darwin/arm64
worktree HEAD fa29e01a (dirty; concurrently edited by another session during this run)
GOWORK=off
GOMODCACHE=/Users/tyler/src/skgo-worktrees/native-plugin/.cache/provisional/modcache
GOPROXY=file:///Users/tyler/src/skgo-worktrees/native-plugin/.cache/provisional/proxy,https://proxy.golang.org
GONOPROXY=none GONOSUMDB=github.com/tylergannon/skgo
GOCACHE=/Users/tyler/Library/Caches/go-build (set explicitly for fake-HOME runs)

K=.cache/provisional/bin/skgo
  sha256 d2d86338badbfca98b1678e6b38b68be7cac77f7db477ab72e9b87b544705d2e
  `skgo buildinfo`: skgo v0.41.0, mod github.com/tylergannon/skgo v0.41.0 h1:W0yBPiYyCkvp1iGY1GLcD7xPbsBSmN5yAIueykzT410=,
  -trimpath=true CGO_ENABLED=1 GOARCH=arm64 GOOS=darwin
A=.cache/provisional/native-shell-v0.1.0-skgo-v0.41.0-darwin-arm64.so   (mtime 17:42:04)
  sha256 f922f53c1ed734cfbc338ae9e016ea1658d62536b1c0d258f133e2367bb09845
  go version -m: path github.com/tylergannon/skgo/plugins/native-shell, mod (devel),
  dep github.com/tylergannon/skgo v0.41.0 h1:W0yBPiYyCkvp1iGY1GLcD7xPbsBSmN5yAIueykzT410=, -buildmode=plugin -trimpath=true
proxy v0.41.0.zip sha256 875b8e4619bcd5cd8bddabd55d14a4819d2910bfd47fe97e4a0e938da0ca0043, .info Time 2026-10-19T00:00:00Z
example pin (plugins/native-shell/example/skgo-plugins.json): native-shell v0.1.0, skgoVersion v0.41.0,
  absolute artifact path to A, sha256 f922f53c…, selections readValue + writeValue
App binary (as given) native/build/macos/DerivedData/Build/Products/Debug/TinyNative.app/Contents/MacOS/TinyNative
  sha256 b0680a97d7bb0b80c925fd40435a6d23d1cebdf49bc14ba85dccbd17344188c7 (mtime 17:42; unchanged by anything in this run)
```

`.cache/provisional/example-skgo-plugins.json` pins an older digest, `2d159949…`. It is a stale side copy and is not what the example uses.

Scratch directory: `S=/private/tmp/claude-501/-Users-tyler-src-skgo-worktrees-native-plugin/69c984c8-0e5f-43ce-a3e1-53a00b33e4ee/scratchpad`. Logs, copies and outputs referenced below are there.

## 1. Command discovery: passes

Run from the example with the official host `$K`:

| Command | Exit | Observed |
|---|---|---|
| `skgo plugin` / `skgo plugin -h` | 0 | plugin usage, then `native-shell: Generate Swift clients and prepare or build the Apple application shell` |
| `skgo plugin native-shell` (also `-h`, `--help`, `help`) | 0 | lists `setup --bundle-id ID`, `prepare`, `build`, `sdk --out PATH`; build flags `--platform macos\|iphone\|simulator --preset NAME --jobs N` |
| `skgo plugin native-shell build -h` | 0 | flag usage (`-jobs`, `-platform` default macos, `-preset`) on stderr |
| `skgo plugin native-shell bogus` | 1 | `unknown native-shell command "bogus"` |
| `skgo plugin nosuch` | 1 | `plugin command "nosuch" is unavailable; see help and plugin diagnostics` |
| `skgo help` | 0 | lists add-on `native-app` (`--set bundle-id`, required), contributed by the plugin (the restore probe reports `"Addons":["native-app"]`) |

Every invocation also prints two unrelated warnings about leftover state in the real `~/.skgo`: a host directory `33fcea76…` belonging to another executable, and the voice-recorder source template. This is noise, not a failure. `skgo --version` falls into the Go vet-tool usage text ("skgo is a tool for static analysis of Go programs"). That is pre-existing and minor; `skgo buildinfo` is the identity command.

## 2. Restoration without setup: passes

These runs used an empty fake home, `HOME=$S/home`, so that restoration actually had something to do.

```text
$ HOME=$S/home $K plugin native-shell          # before restore
plugin native-shell unavailable; run skgo plugin restore: open $S/home/.skgo/plugins/prebuilt/d2d86338…/f922f53c…/native-shell-v0.1.0-skgo-v0.41.0-darwin-arm64.so: no such file or directory   (exit 1)
$ HOME=$S/home $K plugin restore
Installed prebuilt native-shell@v0.1.0 for skgo v0.41.0 ($S/home/.skgo/plugins/prebuilt/d2d86338…/f922f53c…/native-shell-v0.1.0-skgo-v0.41.0-darwin-arm64.so).   (exit 0, 0.22 s)
```

- The restored file's sha256 is `f922f53c…`, identical to the pin. The slot is keyed by host digest (`d2d86338…`, equal to the host executable's sha256) and then by artifact sha.
- `HOME=$S/home $K plugin` and `… plugin native-shell` then discover the commands (exit 0).
- The only files written outside the project were the slot `.so`, `probe.json`, a host `.install.lock` and Go telemetry. In the project, the file snapshot was unchanged by restore: no shell setup, no `native/` writes.
- Limitation: the pin is a local absolute path, so "restore downloads those bytes" was exercised only as a local file copy. HTTPS download was not exercised.

### Rejection cases: all pass except the app-core check during restore

Each case ran in its own scratch project with `HOME=$S/h-<case>`:

| Case | Result |
|---|---|
| wrong `sha256` in pin | exit 1 `plugin native-shell checksum mismatch: got f922f53c…, expected 0000…`; nothing installed |
| correctly named but byte-tampered artifact | exit 1 `checksum mismatch: got d3caf729…, expected f922f53c…`; nothing installed |
| pin `skgoVersion v0.40.0` with v0.41.0 artifact name | exit 1 `plugin pin identity does not match artifact name` |
| pin `version v0.1.29` with v0.1.0 artifact name | exit 1 same message |
| artifact file named `…-skgo-v0.40.0-…` | exit 1 `plugin native-shell v0.1.0 requires official skgo v0.40.0; executing host is v0.41.0` |
| artifact not named `ID-vX.Y.Z-skgo-vX.Y.Z-OS-ARCH.so` | exit 1 naming error |
| artifact path missing | exit 1 `open …: no such file or directory` |
| app `go.mod` selects skgo v0.40.0 | **restore succeeds** (installs). `generate --plugins` then rejects it: exit 1 `application selects skgo v0.40.0; plugin host is v0.41.0; use matching versions` |
| project-built host (`go tool skgo generate --plugins`) | refused, but the message is `plugin native-shell unavailable; run skgo plugin restore` (wrong advice). `go tool skgo plugin restore` then fails with a raw `plugin was built with a different version of package internal/goarch` loader error. The clearer `prebuilt plugins require the matching official skgo executable` text in `prebuilt.go` was never reached. Minor UX issue only; the refusal itself is correct |

## 3. Explicit Swift generation: passes

All flags are those of the example's `//go:generate` line in `internal/skgo/config.go`. `Generated.swift` was backed up to `$S/backup/` and then deleted.

```text
(cd internal/skgo && $K generate --web ../../web --config-package …/internal/app --locals-package …/internal/app --hook-package …/internal/serverhooks)
  → "skgo: generation cache miss", exit 0, native/Generated.swift NOT created   (ordinary path stays plugin-free)
(cd internal/skgo && $K generate --plugins <same flags>)
  → "skgo: wrote …/example/native/Generated.swift", 2.5 s; sha256 1e665de1…; byte-identical to the prior file and to skgo.plugin-outputs.json
repeat --plugins → no output, mtime unchanged (no rewrite of identical output)
go generate ./internal/skgo (scratch copy) → no Generated.swift (plugin-free)
```

The generated file contains `NativeAPI` with `readValue()` (`core.call("uy1bk5/readValue", kind: .query)`), `retainReadValue()` and `writeValue(_:updates:)` (`kind: .command`).

These further checks ran in a scratch copy of the example, with `--cache-dir off`:

- Narrowing the pin's selections to `readValue` rewrote `Generated.swift` and `skgo.plugin-outputs.json`, and `writeValue` was absent (0 occurrences). The pin's selections drive the output.
- An unknown selection `#nope` failed with exit 1: `skgo: unknown generator remote selection(s): src/routes/value.remote.ts#nope`.
- After a hand edit of `Generated.swift`, regeneration failed with exit 1: `… is authored or edited; refusing to overwrite it`. The edit was preserved.
- Client-only use worked with `native/` deleted entirely (no shell and no host): `--plugins` created `native/Generated.swift` with the same sha `1e665de1…`.
- `skgo plugin native-shell sdk --out sdkout` copied `core/` (Zig), `swift/` (Package.swift), `mise.toml/lock` and the `.native-shell-sdk` marker. An absolute `--out` outside the project is refused (`sdk requires a project-relative destination`).

## 4. Native build in the real example: FAILS

```text
$ $K plugin native-shell build --platform macos        (17:46:31, 14.5 s, exit 1; log $S/build.log)
skgo: generation cache miss … skgo: wrote build/
ApplicationView.swift:10:19: error: cannot find 'NativeAPI' in scope
** BUILD FAILED **  xcodebuild … exit status 65
```

`native/Generated.swift` existed throughout (mtime 17:45:13), but the regenerated `native/build/macos/project.json` lists only `native/Sources`. The cause is the change to `native/skgo-native.json` at 17:45:30, made by another session while this validation ran, which removed `swiftRemotes`. The worklog records why: review round 01 moved to "pin-only remote selection, deriving compiled sources after generation". The current `nativebuild/build.go` adds `Generated.swift` whenever the file exists. The artifact under test was built from `.cache/provisional/plugin-source` (17:35), whose `projectSpec` includes it only `if len(c.SwiftRemotes) > 0`. The given `.so` and the current example are therefore incompatible. This is a provisional-candidate staleness defect, not evidence against current source, which was not built or run here.

The failed build did not replace the app binary (still `b0680a97…`). It did rewrite disposable outputs: `.skgo/build.json`, `web/build/*`, `web/.svelte-kit/*`, and the `native/build/macos` project, which now omits `Generated.swift` and had its stale `Generated.o` removed.

## 5. Launch with my runtime-supplied value: passes

The value was generated at launch and never appears in any source file. This run used the executable as given, after the failed rebuild:

```text
$ V="rv-$(openssl rand -hex 8)-ü \"q\""      → rv-c8cbe3eea52359f5-ü "q"
$ perl -e 'alarm 60; exec @ARGV' $APP "$V"
stdout: initial
        rv-c8cbe3eea52359f5-ü "q"
stderr: (empty)      exit 0, 0.26 s
```

Line 1 equals the Go source's initial value. Line 2 is byte-equal to the supplied value, including the non-ASCII character and the embedded quotes. Line 2 comes from a second `readValue()` query after `writeValue(V)`, not from echoing the argument (see `native/Sources/ApplicationView.swift`).

### Does the generated Swift really reach Go? Yes, shown by mutation

A plain launch cannot rule out a coincidental echo, so I ran a full build chain in a copy at `$S/x/plugins/native-shell/example`, from before the 17:45:30 edit and still carrying `swiftRemotes`, which matches the artifact's contract. I ran `vp install --frozen-lockfile` there, with `internal` symlinked so the lockfile's relative adapter link resolves. In the copy's `value.remote.go`, I changed the initial value to a random seed and the command body to `value = "go:" + next`:

```text
$K plugin native-shell build --platform macos   → exit 0, 26.9 s; wrote native/Generated.swift (1e665de1…);
  project.json includes Generated.swift; ** BUILD SUCCEEDED **; app sha256 78ed4ecd…
$APP rv2-77ac22e8e29eba66
stdout: seed-dc78701f
        go:rv2-77ac22e8e29eba66        exit 0
```

Both lines match expectations derived only from values I chose: the seed and the supplied string. The generated query reads Go state, and the generated command executes the Go function. Combined with the real-example failure in section 4, where removing `Generated.swift` from the target breaks compilation, the generated client is load-bearing. Swift code not regenerated by the build (`Sources/`) does not provide `NativeAPI`.

## 6. Open the box: not achieved

I launched the as-given app with no argument: stdout `initial`, then killed it after about 5 s. `screencapture -x` returned exit 0, but the image (`$S/gui.png`) is entirely black, most likely because this session lacks Screen Recording permission. **The window contents (the result label plus the WKWebView) were not visually verified.** A person should look at it once.

## 7. Workspace effects

Comparing file hashes of the example before and after (excluding `web/node_modules` and `native/build`):

- **My actions:** `native/Generated.swift` was deleted and regenerated (identical bytes, new mtime). The failed native build rewrote `.skgo/build.json`, `web/build/{index.html,skgo.manifest.json,client/_app/version.json*}`, `web/.svelte-kit/*` and `native/build/macos/*`. All of these are gitignored, disposable outputs. No tracked source was edited, and `skgo-plugins.json` is unchanged.
- **Not mine (concurrent session):** `README.md`, `cmd/main.go` and `native/skgo-native.json` in the example. In the repo, `nativebuild/build.go`, `nativeapp/config.go`, `cmd/skgo/plugin.go`, `internal/plugininstall/prebuilt.go`, new `.github/workflows/native-plugin-tests.yml` and `ephemeral/reviews/20261019-native-implementation-01.md` among others.
- **Outside the project:** fake homes and copies under `$S` only. The real `~/.skgo` was not modified, apart from host-dir lock reads. No TinyNative process is left running.

## Findings and limitations

1. **Blocker for this candidate:** the provisional `.so` (f922f53c…) predates the pin-only selection change. `skgo plugin native-shell build` on the current example fails to compile. Rebuild the artifact from current `plugins/native-shell` and repeat sections 4–5 on the real example.
2. Restore does not check the app's selected core version; generation does. That satisfies "reject a different core version" at the point of use. Whether restore should also check it is optional.
3. A project-built `go tool skgo` gets misleading advice ("run skgo plugin restore") and then a raw Go plugin-loader error, instead of the official-executable message.
4. The plugin's embedded module version is `(devel)`. Its version identity rests on the file name and the pin alone.
5. The example pin uses a machine-absolute artifact path. The worklog says pins now store project-relative paths, but the provisional pin is still absolute.
6. Not exercised: HTTPS download in restore, `setup`/`prepare` on a fresh app, iOS/simulator platforms, official distributed executable pairing, the private gate and the publisher. GUI contents were not seen.
