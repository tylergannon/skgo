# skgo recipes.
#
# Every recipe here DOES something. None of them decides whether the software
# works. There is deliberately no `just check` and no `just acceptance`: a
# single command that exits 0 becomes what agents build toward, and it cannot
# see the app. Whether skgo works is a screenshot of the running app, looked at.

origin := env("ORIGIN", "http://127.0.0.1:8080")
port := env("SKGO_PORT", "8080")

_default:
    @just --list --unsorted

# node deps for the example app and its Gherkin suite
install:
    cd example/web && mise x -- pnpm install
    cd example/e2e && mise x -- pnpm install

# the link tree, the throwing stubs, and the wire types
generate:
    cd example/generated && go generate ./...

# example/web/dist.go embeds `all:build`, so nothing under example/... compiles
# until this has run once. In a tree that has never built, `go vet` fails with
# "pattern all:build: no matching files found", which reads exactly like broken
# code and is not.

# build the frontend (do this first in a fresh tree)
build:
    cd example/web && ORIGIN="{{origin}}" mise x -- vp build

# go vet, both modules
vet:
    go vet ./... ./example/...

# go test, both modules
test:
    go test -count=1 ./...
    cd example && go test -count=1 ./...

# the example server, against the built frontend
serve:
    go run ./example/cmd -listen 127.0.0.1:{{port}}

# vite in dev, for the proxied path
dev:
    cd example/web && mise x -- vp dev

# Start the server yourself first: `just serve` for a production build, with
# `just dev` alongside it for the proxied path. `pnpm test`, never `playwright
# test`: bddgen compiles the feature files in a separate step, and playwright
# alone happily reruns whatever .features-gen/ already holds, which is last
# sprint's scenarios.

# the Gherkin suite against a server you started
e2e mode="prod":
    cd example/e2e && BASE_URL="{{origin}}" EXPECTED_MODE={{mode}} mise x -- pnpm test

# `git worktree add -b` has silently landed an agent on main once, so the
# branch is confirmed rather than assumed. The pinned kit source is not copied
# or linked in: it lives at one absolute path, /Users/tyler/src/skgo/ephemeral/
# inspiration, reachable from any tree.

# a task worktree, on the branch it says it is on
worktree branch:
    #!/bin/sh
    set -e
    root=$(git rev-parse --show-toplevel)
    dest="$root/.claude/worktrees/{{branch}}"
    git -C "$root" worktree add -b "{{branch}}" "$dest" main
    on=$(git -C "$dest" branch --show-current)
    [ "$on" = "{{branch}}" ] || { echo "expected branch {{branch}}, got $on" >&2; exit 1; }
    echo "$dest  ({{branch}})"

# what is already listening, before you bind a port
ports:
    lsof -nP -iTCP -sTCP:LISTEN
