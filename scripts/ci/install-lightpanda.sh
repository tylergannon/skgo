#!/bin/sh
# Lightpanda, the headless browser the client handoff check drives, pinned by
# release and digest. The bob runner image runs this when it is built; a job
# runs it to fetch the same binary on a runner without it.
#
#   install-lightpanda.sh DIR
#
# Does nothing when the pinned release is already on PATH. Otherwise installs
# DIR/lightpanda and, inside a GitHub Actions job, adds DIR to PATH.
set -eu
version=1.0.1
sha256=551747e35c51ba107b5d5277eca4b48e077b2a3b40f09a047bf6670448bc35fd
dir=${1:?usage: install-lightpanda.sh DIR}

if [ "$(lightpanda version 2>/dev/null)" = "$version" ]; then
	echo "Lightpanda $version is $(command -v lightpanda)"
	exit 0
fi
mkdir -p "$dir"
curl -fsSL -o "$dir/lightpanda.download" "https://github.com/lightpanda-io/browser/releases/download/$version/lightpanda-x86_64-linux"
echo "$sha256  $dir/lightpanda.download" | sha256sum -c -
chmod +x "$dir/lightpanda.download"
mv "$dir/lightpanda.download" "$dir/lightpanda"
if [ -n "${GITHUB_PATH:-}" ]; then
	echo "$dir" >> "$GITHUB_PATH"
fi
