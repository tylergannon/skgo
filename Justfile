# skgo recipes.
#
# Every recipe here DOES something. None of them decides whether the software
# works. There is deliberately no `just check` and no `just acceptance`: a
# single command that exits 0 becomes what agents build toward, and it cannot
# see the app. Whether skgo works is `just test` green in seconds, `just e2e`
# green on main, and a person who opened the box.

origin := env("ORIGIN", "http://127.0.0.1:8080")
port := env("SKGO_PORT", "8080")
devport := env("SKGO_DEV_PORT", "5173")

# Where `just serve` copies the server's log. The Gherkin suite reads it,
# because one claim in it is about a line only an operator ever sees: what the
# rendering engine wrote to the console. A browser never sees that, so a
# scenario about it has nowhere else to look.
log := env("SKGO_LOG", justfile_directory() / "example/e2e/server.log")

# Every recipe's child processes resolve Staticcheck from this tree first:
# the one on PATH was built for an older Go and cannot read this toolchain's
# export data, which makes `skgo check` report it incomplete.
export PATH := justfile_directory() / ".tools/bin" + ":" + env("PATH")

# Build and test processes load the same pinned frontend dependencies. Reuse
# Node's compiled code; each process still executes its own module instances.
export NODE_COMPILE_CACHE := env("NODE_COMPILE_CACHE", justfile_directory() / ".cache/node-compile")

_default:
    @just --list --unsorted

# node deps for the example app and its Gherkin suite
install:
    cd example/web && mise x -- pnpm install
    cd example/e2e && mise x -- pnpm install

# the link tree, the throwing stubs, and the wire types
generate:
    cd example/internal/skgo && go generate ./...

# example/web/dist.go embeds `all:build`; its tracked placeholder keeps a fresh
# checkout compilable before the frontend build has run.

# build the frontend (do this first in a fresh tree)
build:
    cd example/web && ORIGIN="{{origin}}" mise x -- vp build

# go vet, both modules
vet:
    go vet ./... ./example/...

# a Staticcheck that can read this Go toolchain's export data, kept in .tools/
tools:
    #!/usr/bin/env bash
    set -eu
    if [ "$(.tools/bin/staticcheck -version 2>/dev/null | cut -d' ' -f1-2)" != "staticcheck 2026.2.1" ]; then
        GOBIN="{{justfile_directory()}}/.tools/bin" go install honnef.co/go/tools/cmd/staticcheck@v0.8.1
    fi

# go test, both modules, in one invocation so their packages run side by side
test: tools
    cd example && mise x -- go -C .. test -json -count=1 ./... ./example/...

# the example server, against the built frontend
serve:
    #!/usr/bin/env bash
    set -o pipefail
    go run ./example/cmd -listen 127.0.0.1:{{port}} 2>&1 | tee "{{log}}"

# The example app in dev, launched once: `skgo dev` starts `vp dev`, builds and
# runs the Go application, and serves the public address itself. Go renders the
# document — pulling one module at a time out of the `goja` environment the
# adapter declares in the dev server — and forwards modules, assets and the HMR
# socket to vite, so the browser only ever talks to Go. Editing Go source,
# signatures or routes regenerates the bindings, rebuilds the application and
# replaces it behind the same address; a build error is the response to every
# request until it is fixed.
#
# `vp` must be the project-local binary: kit checks the SSR environment with
# `instanceof` against the project's own `vite`, and a mise-global copy of the
# same version fails that check.

# the example app in dev: vite, the generator and the Go application, kept current
dev:
    #!/usr/bin/env bash
    set -o pipefail
    go run ./cmd/skgo dev --root example --web web --cmd ./cmd \
        --listen 127.0.0.1:{{port}} --origin "{{origin}}" \
        --vite-port {{devport}} --vite "mise x -- node_modules/.bin/vp dev" 2>&1 | tee "{{log}}"

# Start the server yourself first: `just serve` for a production build, with
# `just dev` alongside it for the proxied path. `vp run test` invokes the e2e
# package script, which regenerates BDD tests before the Go runner starts
# Playwright in two ordered stages.

e2e_json_reporter := if env_var_or_default("PLAYWRIGHT_JSON_OUTPUT_FILE", "") != "" { " --add-reporter=json" } else if env_var_or_default("PLAYWRIGHT_JSON_OUTPUT_NAME", "") != "" { " --add-reporter=json" } else { "" }

# the Gherkin suite against a server you started
e2e mode run=mode:
    cd example/e2e && BASE_URL="{{origin}}" SKGO_EXPECTED_MODE={{mode}} SKGO_E2E_RUN={{run}} SKGO_LOG="{{log}}" mise x -- ../web/node_modules/.bin/vp run --no-cache test{{e2e_json_reporter}}

# `git worktree add -b` has silently landed an agent on main once, so the
# branch is confirmed rather than assumed. Each worktree installs its own
# pinned Kit source; follow ephemeral/sveltekit-current/SKILL.md.

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
