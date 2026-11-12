package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

func pin(path, pattern string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	m := regexp.MustCompile(pattern).FindStringSubmatch(string(b))
	if len(m) != 2 {
		return "", fmt.Errorf("missing tool pin in %s", path)
	}
	return m[1], nil
}
func releaseNotes(m candidate, repo string) (string, error) {
	var tools map[string]string
	if err := readJSON("candidate/toolchain.json", &tools); err != nil {
		return "", err
	}
	for _, key := range []string{"go", "node", "vite_plus", "pnpm", "zig", "xcodegen"} {
		if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(tools[key]) {
			return "", fmt.Errorf("invalid frozen %s tool pin", key)
		}
	}
	goVersion, node, vp, zig, xcodegen := tools["go"], tools["node"], tools["vite_plus"], tools["zig"], tools["xcodegen"]
	base := "https://github.com/" + repo
	download := base + "/releases/download/plugins/native-shell/" + m.Plugin
	notes := `Native-shell @PLUGIN@ for the exact official **skgo @CORE@** executable on **macOS ARM64**.

[Plugin](@DOWNLOAD@/@ARTIFACT@) · [SHA256 checksums](@DOWNLOAD@/checksums.txt) · [Candidate manifest](@DOWNLOAD@/candidate.json) · [Publication receipt](@DOWNLOAD@/release.json) · [Official CLI](@BASE@/releases/tag/@CORE@).

Plugin SHA256: ` + "`@SHA@`" + `.
Source: [@SOURCE@](@BASE@/tree/@SOURCE@); core source: @CORE_SOURCE@.
Candidate run: [@RUN@, attempt @ATTEMPT@](@BASE@/actions/runs/@RUN@/attempts/@ATTEMPT@).

Prerequisites: Apple Silicon Mac, full Xcode selected with ` + "`xcode-select`" + `, its license accepted and command-line tools available, Git, curl, and mise installed. The recipe installs an isolated [official global VitePlus CLI](https://viteplus.dev/guide/global-cli). Use the pinned Go @GO@, Node @NODE@, VitePlus @VP@, Zig @ZIG@ and XcodeGen @XCODEGEN@ below. Only macOS ARM64 is qualified; these instructions build an unsigned local application, not a distributable or notarized application.

From a fresh directory, this downloads the official CLI (without rebuilding it), verifies it and installs the qualified plugin into a standalone copy of the tiny example:

` + "```sh" + `
set -eu
workdir=$(mktemp -d)
workdir=$(cd "$workdir" && pwd -P)
cd "$workdir"
export VP_HOME="$workdir/.vite-plus"
curl -fsSL https://vite.plus -o install-vp.sh
CI=true VP_VERSION=@VP@ VP_NODE_MANAGER=no VP_PM_MANAGER=no VP_SELF_SETUP_NO_MODIFY_PATH=1 bash install-vp.sh
global_vp="$VP_HOME/bin/vp"
curl -fL '@BASE@/releases/download/@CORE@/skgo_@CORE_NUMBER@_darwin_arm64.tar.gz' -o skgo.tar.gz
curl -fL '@BASE@/releases/download/@CORE@/checksums.txt' -o core-checksums.txt
expected=$(awk '$2 == "skgo_@CORE_NUMBER@_darwin_arm64.tar.gz" { print $1 }' core-checksums.txt)
test "${#expected}" = 64
printf '%s  skgo.tar.gz\n' "$expected" | shasum -a 256 --check
tar -xzf skgo.tar.gz
host=$(find "$workdir" -type f -name skgo -print -quit)
test -n "$host"
mkdir bin
ln -s "$host" bin/skgo
export PATH="$workdir/bin:$PATH"
export GOWORK=off
skgo buildinfo --json
curl -fL '@BASE@/archive/@SOURCE@.tar.gz' -o source.tar.gz
tar -xzf source.tar.gz
cp -R skgo-@SOURCE@/plugins/native-shell/example tiny-native
cd tiny-native
cat > mise.toml <<'TOOLS'
[tools]
go = "@GO@"
node = "@NODE@"
zig = "@ZIG@"
"aqua:yonaskolb/XcodeGen" = "@XCODEGEN@"
TOOLS
mise trust
export MISE_ACTIVATE_AGGRESSIVE=true
mise install
# Keep child processes on these pins even when a global shell config adds tools.
export PATH="$workdir/bin:$(dirname "$(mise which go)"):$(dirname "$(mise which node)"):$(dirname "$global_vp"):$(dirname "$(mise which zig)"):$(dirname "$(mise which xcodegen)"):$PATH"
unset GOROOT
export GOTOOLCHAIN=go@GO@
export VP_NODE_VERSION=@NODE@
skgo update --complete-version @CORE@ --root . --vp "$global_vp"
skgo plugin install --prebuilt '@DOWNLOAD@/@ARTIFACT@' --sha256 '@SHA@' \
  --select 'src/routes/value.remote.ts#readValue' \
  --select 'src/routes/value.remote.ts#writeValue'
MISE_TRUSTED_CONFIG_PATHS="$PWD/native/build/macos/.sdk" skgo plugin native-shell build --platform macos
open native/build/macos/DerivedData/Build/Products/Debug/TinyNative.app
` + "```" + `

The example already owns its Apple shell; it does not require setup. Keep the resulting skgo-plugins.json to restore this exact artifact with skgo plugin restore. Updating core requires a separately qualified native-shell pairing.
`
	return strings.NewReplacer("@PLUGIN@", m.Plugin, "@CORE@", m.Core, "@CORE_NUMBER@", strings.TrimPrefix(m.Core, "v"), "@DOWNLOAD@", download, "@ARTIFACT@", m.Artifact, "@BASE@", base, "@SHA@", m.SHA, "@SOURCE@", m.Source, "@CORE_SOURCE@", m.CoreSource, "@RUN@", fmt.Sprint(m.Run), "@ATTEMPT@", fmt.Sprint(m.Attempt), "@GO@", goVersion, "@NODE@", node, "@VP@", vp, "@ZIG@", zig, "@XCODEGEN@", xcodegen).Replace(notes), nil
}
