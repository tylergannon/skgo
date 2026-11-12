set -eu
workdir=$(mktemp -d)
workdir=$(cd "$workdir" && pwd -P)
cd "$workdir"
export VP_HOME="$workdir/.vite-plus"
curl -fsSL https://vite.plus -o install-vp.sh
CI=true VP_VERSION=1.3.0 VP_NODE_MANAGER=no VP_PM_MANAGER=no VP_SELF_SETUP_NO_MODIFY_PATH=1 bash install-vp.sh
global_vp="$VP_HOME/bin/vp"
curl -fL 'https://github.com/tylergannon/skgo/releases/download/v0.46.0/skgo_0.46.0_darwin_arm64.tar.gz' -o skgo.tar.gz
curl -fL 'https://github.com/tylergannon/skgo/releases/download/v0.46.0/checksums.txt' -o core-checksums.txt
expected=$(awk '$2 == "skgo_0.46.0_darwin_arm64.tar.gz" { print $1 }' core-checksums.txt)
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
curl -fL 'https://github.com/tylergannon/skgo/archive/45e471a8e6e2677ffb8a6a49f4a680a78e7d4d21.tar.gz' -o source.tar.gz
tar -xzf source.tar.gz
cp -R skgo-45e471a8e6e2677ffb8a6a49f4a680a78e7d4d21/plugins/native-shell/example tiny-native
cd tiny-native
cat > mise.toml <<'TOOLS'
[tools]
go = "1.27.3"
node = "24.22.0"
zig = "0.17.0"
"aqua:yonaskolb/XcodeGen" = "2.46.0"
TOOLS
mise trust
export MISE_ACTIVATE_AGGRESSIVE=true
mise install
# Keep child processes on these pins even when a global shell config adds tools.
PATH="$workdir/bin:$(dirname "$(mise which go)"):$(dirname "$(mise which node)"):$(dirname "$global_vp"):$(dirname "$(mise which zig)"):$(dirname "$(mise which xcodegen)"):$PATH"
export PATH
unset GOROOT
export GOTOOLCHAIN=go1.27.3
export VP_NODE_VERSION=24.22.0
skgo update --complete-version v0.46.0 --root . --vp "$global_vp"
skgo plugin install --prebuilt 'https://github.com/tylergannon/skgo/releases/download/plugins/native-shell/v0.1.0/native-shell-v0.1.0-skgo-v0.46.0-darwin-arm64.so' --sha256 '1956b2dc4c64b2e67d6cb91393ff2efa9fce8f4a4a7ddf05489d438223c13b8f' \
  --select 'src/routes/value.remote.ts#readValue' \
  --select 'src/routes/value.remote.ts#writeValue'
MISE_TRUSTED_CONFIG_PATHS="$PWD/native/build/macos/.sdk" skgo plugin native-shell build --platform macos
open native/build/macos/DerivedData/Build/Products/Debug/TinyNative.app
