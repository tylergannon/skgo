# skgo recipes.
#
# Every recipe here DOES something. None of them decides whether the software
# works. There is deliberately no `just check` and no `just acceptance`: a
# single command that exits 0 becomes what agents build toward, and it cannot
# see the app. Whether skgo works is a screenshot of the running app, looked at.

origin := env("ORIGIN", "http://127.0.0.1:8080")
port := env("SKGO_PORT", "8080")
devport := env("SKGO_DEV_PORT", "5173")

# Where `just serve` copies the server's log. The Gherkin suite reads it,
# because one claim in it is about a line only an operator ever sees: what the
# rendering engine wrote to the console. A browser never sees that, so a
# scenario about it has nowhere else to look.
log := env("SKGO_LOG", justfile_directory() / "example/e2e/server.log")

_default:
    @just --list --unsorted

# node deps for the example app and its Gherkin suite
install:
    cd example/web && mise x -- pnpm install
    cd example/e2e && mise x -- pnpm install

# the link tree, the throwing stubs, and the wire types
generate:
    cd example/generated && go generate ./...

# example/web/dist.go embeds `all:build`; its tracked placeholder keeps a fresh
# checkout compilable before the frontend build has run.

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
    #!/usr/bin/env bash
    set -o pipefail
    go run ./example/cmd -listen 127.0.0.1:{{port}} 2>&1 | tee "{{log}}"

# The example app in dev: `vp dev` behind, Go in front. Go renders the document
# — pulling one module at a time out of the `goja` environment the adapter
# declares in the dev server — and forwards modules, assets and the HMR socket
# to vite, so the browser only ever talks to Go.
#
# `vp` must be the project-local binary: kit checks the SSR environment with
# `instanceof` against the project's own `vite`, and a mise-global copy of the
# same version fails that check.

# the example app in dev, both halves
dev:
    #!/bin/sh
    set -e
    cd "{{justfile_directory()}}/example/web"
    mise x -- node_modules/.bin/vp dev --host 127.0.0.1 --port {{devport}} --strictPort &
    vite=$!
    trap 'kill "$vite" 2>/dev/null' EXIT INT TERM
    cd "{{justfile_directory()}}"
    go run ./example/cmd -listen 127.0.0.1:{{port}} -proxy http://127.0.0.1:{{devport}} -origin "{{origin}}" 2>&1 | tee "{{log}}"

# Start the server yourself first: `just serve` for a production build, with
# `just dev` alongside it for the proxied path. `pnpm test`, never `playwright
# test`: bddgen compiles the feature files in a separate step, and playwright
# alone happily reruns whatever .features-gen/ already holds, which is last
# sprint's scenarios.

# the Gherkin suite against a server you started
e2e mode run=mode:
    cd example/e2e && BASE_URL="{{origin}}" SKGO_EXPECTED_MODE={{mode}} SKGO_E2E_RUN={{run}} SKGO_LOG="{{log}}" mise x -- pnpm test

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

# Releases have no local recipe. A Conventional Commit merged to main starts
# release.yml; only its browser-qualified SHA is published and tagged.
