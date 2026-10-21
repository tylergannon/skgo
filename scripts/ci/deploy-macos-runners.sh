#!/usr/bin/env bash
# Keep two independent skgo Actions workers on Amritanandamayi. Each has its
# own checkout and launch agent; existing workers are not interrupted.
set -euo pipefail
repo=tylergannon/skgo
version=2.338.0
sha256=df4cebda25c86a886ed204e49fee63f5c2e7cec5f447b5c98440a826bbdf9df2

test "$(uname -s)" = Darwin
test "$(uname -m)" = arm64
test "$(scutil --get LocalHostName | tr '[:upper:]' '[:lower:]')" = amritanandamayi

runner() { gh api "repos/$repo/actions/runners" --jq ".runners[] | select(.name == \"$1\") | .$2"; }

for n in 1 2; do
	name=amritanandamayi-skgo
	install="$HOME/actions-runner-skgo"
	if [ "$n" != 1 ]; then
		name=$name-$n
		install=$install-$n
	fi
	mkdir -p "$install"
	if [ ! -f "$install/.runner" ]; then
		if [ -n "$(runner "$name" id)" ]; then
			echo "$name is registered but has no local configuration in $install" >&2
			exit 1
		fi
		archive="$install/runner.tar.gz"
		curl -fsSL -o "$archive" "https://github.com/actions/runner/releases/download/v$version/actions-runner-osx-arm64-$version.tar.gz"
		printf '%s  %s\n' "$sha256" "$archive" | shasum -a 256 -c -
		tar -xzf "$archive" -C "$install"
		rm "$archive"
		token=$(gh api -X POST "repos/$repo/actions/runners/registration-token" --jq .token)
		(cd "$install" && ./config.sh --unattended --url "https://github.com/$repo" \
			--token "$token" --name "$name" --labels skgo-release,amritanandamayi --work _work)
		unset token
	fi
	id=$(runner "$name" id)
	gh api -X POST "repos/$repo/actions/runners/$id/labels" \
		-f 'labels[]=skgo-release' -f 'labels[]=amritanandamayi' --silent
	if [ ! -f "$install/.service" ]; then
		(cd "$install" && ./svc.sh install)
	fi
	service=$(/usr/libexec/PlistBuddy -c 'Print :Label' "$(cat "$install/.service")")
	if ! launchctl print "gui/$(id -u)/$service" >/dev/null 2>&1; then
		(cd "$install" && ./svc.sh start)
	fi
	for _ in $(seq 60); do
		[ "$(runner "$name" status)" = online ] && break
		sleep 2
	done
	if [ "$(runner "$name" status)" != online ]; then
		echo "$name did not come online" >&2
		exit 1
	fi
	echo "$name is online"
done
