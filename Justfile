# skgo recipes.
#
# Every recipe here DOES something. None of them decides whether the software
# works. There is deliberately no `just check` and no `just acceptance`: a
# single command that exits 0 becomes what agents build toward, and it cannot
# see the app. Tests let an agent check quickly that a change broke nothing
# else; they are not proof of work, and they do not qualify a release.

origin := env("ORIGIN", "http://127.0.0.1:8080")
port := env("SKGO_PORT", "8080")

# Every recipe's child processes resolve Staticcheck from this tree first:
# the one on PATH was built for an older Go and cannot read this toolchain's
# export data, which makes `skgo check` report it incomplete.
export PATH := justfile_directory() / ".tools/bin" + ":" + env("PATH")

# Build and test processes load the same pinned frontend dependencies. Reuse
# Node's compiled code; each process still executes its own module instances.
export NODE_COMPILE_CACHE := env("NODE_COMPILE_CACHE", justfile_directory() / ".cache/node-compile")

_default:
    @just --list --unsorted

# node deps for the example app
install:
    cd example/web && vp install

# the link tree, the throwing stubs, and the wire types
generate:
    cd example/internal/skgo && go generate ./...

# example/web/dist.go embeds `all:build`; its tracked placeholder keeps a fresh
# checkout compilable before the frontend build has run.

# Generate and build the frontend, reusing current outputs automatically.
build:
    ORIGIN="{{origin}}" go run ./cmd/skgo build --root example

# Explicitly discard example build caches; normal builds always reuse them.
clean:
    rm -rf example/.skgo example/web/.svelte-kit
    find example/web/build -mindepth 1 ! -name .gitkeep -delete

# go vet, both modules
vet:
    go vet ./... ./example/...

# Checker tools that can read this Go toolchain's export data, kept in .tools/.
tools:
    #!/usr/bin/env bash
    set -eu
    if [ "$(.tools/bin/staticcheck -version 2>/dev/null | cut -d' ' -f1-2)" != "staticcheck 2026.2.2" ]; then
        GOBIN="{{justfile_directory()}}/.tools/bin" go install honnef.co/go/tools/cmd/staticcheck@v0.8.2
    fi
    GOBIN="{{justfile_directory()}}/.tools/bin" go install golang.org/x/tools/cmd/goimports@v0.52.0

# go test, both modules; vp resolves the frontend tools from the project pins.
# Compiler/process and built-example witnesses are selected explicitly by CI
# with skgo_qualification; ordinary contracts use Go's normal test cache.
test *args: tools
    cd example/web && vp exec go -C ../.. test {{args}} ./... ./example/...

# the example server, against the built frontend
serve:
    go run ./example/cmd -listen 127.0.0.1:{{port}}

# `git worktree add -b` has silently landed an agent on main once, so the
# branch is confirmed rather than assumed. Each worktree installs its own
# pinned Kit source; follow ephemeral/sveltekit-current/SKILL.md.

# a task worktree, on the branch it says it is on
worktree branch base="main":
    #!/bin/sh
    set -e
    root=$(git rev-parse --show-toplevel)
    dest="$root/.claude/worktrees/{{branch}}"
    git -C "$root" worktree add -b "{{branch}}" "$dest" "{{base}}"
    on=$(git -C "$dest" branch --show-current)
    [ "$on" = "{{branch}}" ] || { echo "expected branch {{branch}}, got $on" >&2; exit 1; }
    echo "$dest  ({{branch}})"

# what is already listening, before you bind a port
ports:
    lsof -nP -iTCP -sTCP:LISTEN

# Releases have no local recipe. A core change merged to main announces a
# candidate to skgo-project for qualification (candidate.yml); a manual
# release.yml dispatch naming a qualified candidate publishes the signed CLI
# and npm packages and updates Homebrew.
