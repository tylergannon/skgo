# Semantic Index Configuration

(Placed under `ephemeral/` because this repo forbids writing `docs/` without permission.)

## Token Cache

The token cache is the full local materialization of the project context.
Tools like `rg`, `find`, `grep`, and `wc` operate on it directly.

**Remote source**: mixed — `git` clones of sveltejs/kit and other frontend sources; `tylergannon/devalue` at `0561b52038cc0fd6955365014f660655fc128820` (`v5.UpstreamVersion = 5.9.4`); the junkyard (earlier attempt, gitignored snapshot); polytype mirrored from the Go module cache
**Remote type**: `local` (gitignored; must be present in the root checkout)
**Local path**: `/Users/tyler/src/skgo/ephemeral/inspiration/`
**Sync command**:
```
# devalue: git -C /Users/tyler/src/skgo/ephemeral/inspiration/reference/devalue@5.9.4 fetch && git -C /Users/tyler/src/skgo/ephemeral/inspiration/reference/devalue@5.9.4 checkout --detach 0561b52038cc0fd6955365014f660655fc128820
# other git sources: fetch and checkout their pinned commits individually
# polytype: see ephemeral/inspiration/reference/polytype/README-LOCAL.md
```
**Token cache scope**: pinned upstream sources skgo must reproduce in Go, plus the junkyard's working pieces and recorded traps.

## Semantic Index

The semantic index is a routing tree built over the token cache.
Use it to retrieve relevant citations without scanning the full token cache.

**Status**: Available
**Local path**: `/Users/tyler/src/skgo/ephemeral/semantic-index/`
**Entrypoint**: `/Users/tyler/src/skgo/ephemeral/semantic-index/README.md`
**Access**: Read the entrypoint, then `TAXONOMY.md` (task routes), `themes.md`, `recipes.md`, then leaf citations.
Citations in leaf files resolve to paths under the token cache local path.
