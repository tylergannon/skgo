#!/usr/bin/env bash
# Rebuild skgo's self-hosted Linux runner image on bob from this checkout's
# HEAD, then replace each runner with it once that runner is idle. A replaced
# container starts unregistered and waits for .skgo-start, so each is
# registered again under its old name before it takes a job.
set -euo pipefail
repo=tylergannon/skgo
dir=.local/share/skgo-release
compose="docker compose -f $dir/scripts/ci/bob-runners.compose.yml"

cd "$(git rev-parse --show-toplevel)"
echo "Building the bob runner image from $(git rev-parse --short HEAD)"
git archive HEAD scripts/ci example/mise.toml | ssh bob "mkdir -p $dir && tar -x -C $dir"
ssh bob "mkdir -p ~/.config/systemd/user \
		~/.cache/skgo-release/cache-arm64 ~/.cache/skgo-release/work-arm64 \
	&& cp $dir/scripts/ci/bob-arm64-emulation.service ~/.config/systemd/user/ \
	&& systemctl --user daemon-reload \
	&& systemctl --user enable --now bob-arm64-emulation.service"
ssh bob "$compose build"

runner() { gh api "repos/$repo/actions/runners" --jq ".runners[] | select(.name == \"$1\") | .$2"; }

for n in 1 2 arm64; do
	name=bob-skgo-release-$n
	while [ "$(runner "$name" busy)" = true ]; do
		echo "$name is running a job; waiting"
		sleep 15
	done
	token=$(gh api -X POST "repos/$repo/actions/runners/registration-token" --jq .token)
	ssh bob "$compose up -d --force-recreate --no-deps runner-$n \
		&& { docker exec skgo-release-runner-$n ./config.sh --unattended --replace \
			--url https://github.com/$repo --token $token --name $name --labels skgo-release,bob \
			> $dir/register-$n.log 2>&1 || { cat $dir/register-$n.log; exit 1; }; } \
		&& docker exec skgo-release-runner-$n touch .skgo-start"
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
