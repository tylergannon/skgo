#!/bin/sh
# Create a task worktree that is actually ready to work in.
#
# Usage: ephemeral/tractor/worktree.sh <branch-name>
#
# A fresh worktree is not a working copy of this project. `ephemeral/inspiration`
# is gitignored, so the pinned kit source — the specification every mirrored
# feature is written against — exists only in the root checkout. An agent told
# to read kit finds an empty directory and proceeds on priors, which is the
# quietest possible failure mode on a project whose first rule is that kit is
# the spec.
set -e

BRANCH="${1:?usage: worktree.sh <branch-name>}"
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
DEST="$ROOT/.claude/worktrees/$BRANCH"

git -C "$ROOT" worktree add -b "$BRANCH" "$DEST" main
# Still gitignored inside the worktree, so it cannot be committed by accident.
ln -s "$ROOT/ephemeral/inspiration" "$DEST/ephemeral/inspiration"

# Confirm rather than assume: `git worktree add -b` has silently landed an agent
# on main once already.
ON=$(git -C "$DEST" branch --show-current)
[ "$ON" = "$BRANCH" ] || { echo "worktree.sh: expected branch $BRANCH, got $ON" >&2; exit 1; }
[ -d "$DEST/ephemeral/inspiration/reference/kit" ] || { echo "worktree.sh: pinned kit source did not link through" >&2; exit 1; }

echo "$DEST  ($BRANCH, kit source linked)"
