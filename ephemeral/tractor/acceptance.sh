#!/bin/sh
# The acceptance gate for a skgo mission: the suite decides, not the agent.
# Fails loudly rather than skipping — a check that cannot run has not passed.
set -e
cd "$(dirname "$0")/../.."

go vet ./... ./example/...
go test -count=1 ./...
(cd example && go test -count=1 ./...)

# The e2e suite against a production build with vite stopped.
if lsof -ti:5173 >/dev/null 2>&1; then
	echo "acceptance: vite is running on 5173; the prod run must not proxy" >&2
	exit 1
fi
(cd example/web && ORIGIN=http://127.0.0.1:8080 mise x -- vp build)
go build -o /tmp/skgo-acceptance ./example/cmd
/tmp/skgo-acceptance -listen 127.0.0.1:8080 &
SERVER=$!
trap 'kill $SERVER 2>/dev/null || true' EXIT
sleep 2
(cd example/e2e && BASE_URL=http://127.0.0.1:8080 EXPECTED_MODE=prod mise x -- pnpm exec playwright test)
