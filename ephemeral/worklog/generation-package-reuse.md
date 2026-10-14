# Generation package reuse

decision: The public handoff must share parsed and type-checked packages, including dependency source when available; an AST-only handoff does not avoid type-checking. User authorized coordinated SKGO #307 and Polytype #164 implementation and landing.
friction: Root SKGO checkout is detached and has existing deleted profiling artifacts; implementation uses an isolated worktree and will not reset the user's checkout.
doc_bug: Issue #307's old reproduction calls ambient pnpm; this investigation uses vp install from each project's pinned declarations.
friction: SKGO's nested test modules do not inherit a local Polytype replace directive. Run cross-module checks against a reachable Polytype commit/release pin; a local replacement is sufficient only for the focused package tests.
correction: Six fewer package loads on Gimbal did not establish an overall speedup: snapshot acquisition/capture must be counted separately, and capture alone consumed the observed generation-time saving. Prepared graph reuse and skipped loads are demonstrated capabilities, not a claim of faster end-to-end Gimbal generation.
friction: The manual-plugin compatibility test inherited the real user home and mistook unrelated managed-plugin warnings for fixture incompatibility. Its help subprocesses now use a temporary home; the existing regression passes without touching installed plugins.
