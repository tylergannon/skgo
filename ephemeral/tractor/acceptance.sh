#!/bin/sh
# The acceptance gate for a skgo mission: the suite decides, not the agent.
# Fails loudly rather than skipping — a check that cannot run has not passed.
set -e
cd "$(dirname "$0")/../.."

# The ports are overridable because a worktree and its ports are one
# single-writer resource: two runs on the default pair have already contaminated
# each other once. Set SKGO_PORT and SKGO_VITE_PORT to move a run out of the
# way; they must be set together with whatever `vp dev` was told to use, because
# the app's origin is fixed at build time.
: "${SKGO_PORT:=8080}"
: "${SKGO_VITE_PORT:=5173}"
ORIGIN="http://127.0.0.1:$SKGO_PORT"

# A gate that cannot run has not passed. Fail with a message that says so,
# distinctly from an honest red, so a routed-back agent is not told its code
# is broken when the harness is.
for tool in go mise lsof; do
	command -v "$tool" >/dev/null 2>&1 || {
		echo "acceptance: HARNESS FAULT - $tool is not on PATH; this says nothing about the code" >&2
		exit 90
	}
done

# The e2e suite against a production build with vite stopped.
if lsof -ti:"$SKGO_VITE_PORT" >/dev/null 2>&1; then
	echo "acceptance: vite is running on $SKGO_VITE_PORT; the prod run must not proxy" >&2
	exit 1
fi
if lsof -ti:"$SKGO_PORT" >/dev/null 2>&1; then
	echo "acceptance: something already listens on $SKGO_PORT; the suite would test it instead" >&2
	exit 1
fi
(cd example/web && ORIGIN="$ORIGIN" mise x -- vp build)

# The Go checks come after the build: example/web/dist.go embeds `all:build`,
# so in a worktree that has never built the frontend, `go vet` fails with
# "pattern all:build: no matching files found" before a single real check has
# run — a harness fault that reads exactly like a red suite.
go vet ./... ./example/...
go test -count=1 ./...
(cd example && go test -count=1 ./...)
go build -o /tmp/skgo-acceptance-$SKGO_PORT ./example/cmd
/tmp/skgo-acceptance-$SKGO_PORT -listen 127.0.0.1:"$SKGO_PORT" &
SERVER=$!
trap 'kill $SERVER 2>/dev/null || true' EXIT
sleep 2
# `pnpm test`, not `playwright test`: playwright-bdd compiles the feature files
# into `.features-gen/` in a separate step, and `playwright test` alone happily
# runs whatever that directory already holds. A stale one is a green run over
# the previous sprint's scenarios.
(cd example/e2e && BASE_URL="$ORIGIN" EXPECTED_MODE=prod mise x -- pnpm test)
