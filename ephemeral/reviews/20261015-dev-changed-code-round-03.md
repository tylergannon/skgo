# Development-server repairs: independent complete review, round 03

Outcome: **no findings**.

Target: branch `codex/dev-server-review-fixes`, commit `380f943`, complete diff from `1d9e23e1760266cc83367c00999105e28db19944`. This is a review of the entire repair set, not only the latest fix. Authorities are the user's request to implement the prior development-server review findings, repository AGENTS instructions, agent-protocol, current `ephemeral/projects/dev-server/{GOAL,DESIGN,PLAN,CONTINUE}.md`, and the original review at `/Users/tyler/.codex/worktrees/9b3a/skgo-project/ephemeral/reviews/20261015-skgo-development-server-review.md`. The installed Kit 3.0.0 mapping and current token-cache architecture route read in round 01 remain applicable. No scope narrowing was supplied or applied.

## Findings

No material defects or genuine nitpicks remain from this independent review of the changed code and available evidence.

The configuration repair publishes a Vite instance epoch even for an in-process restart and separately identifies Go constructor inputs by manifest/environment content. Guarded publication reloads the Go application configuration when those inputs change and updates the public remote prefix after successful publication. The added real-session assertions cover base, appDir, trusted origins, static environment values, retained Go PID and a subsequent compatible edit.

Captured CompilerInputs now reach package analysis, native compilation and the launched host. Workspace mode keeps its captured members/replacements and adds the private loader without applying forbidden -modfile flags. Environment selection changes require replacement. The native fixture combines sibling workspace resolution, a workspace-only replacement, frozen runtime environment, aliased SKGO FetchConfig, an unrelated same-named type and an embedded resource. This directly exercises the original boundary failures. Explicit NewDevelopmentHandler construction removes the filesystem identity carrier; ordinary fs.FS wrapping/subdirectories remain composable, while nil development factories retain production construction.

The request bridge now sends continuation request payloads in response bodies, emits response-header effects once across hooks/actions/concurrent loads, requests Kit parent data only when Go calls Parent, preserves fetch URL/redirect/type metadata, and retires exchanges at their logical owner's end. Tests exercise actual Go services and the actual JS bridge with independent literals. The installed Kit rules that motivated these changes remain their source authority.

Both body-lifetime findings from the earlier review rounds are resolved. A continuation upload retains its HTTP handler until Go releases the body, while separate next-step negotiation permits additional continuations. The strengthened regression deliberately delays first-body consumption until a second continuation returns. Explicit Close now releases the wrapper without calling the draining server request-body Close; its 204 acknowledgment cancels that specific JS producer. The regression requires producer cancellation before consuming the substituted final response, so normal fetch completion cannot conceal the former hang. Progressive streaming remains separately exercised by a producer that cannot finish until its first forwarded bytes are received.

The shutdown repair also avoids replacing the frontend pointer with nil when cancellation races recovery and stops unnecessary recovery work after session cancellation.

## Evidence inspected and verification limits

- Fresh at `380f943`: `go test . -run 'TestNative(Bridge|Fetch|Development)' -count=10 -timeout=90s` passed in 6.127 s, using the managed Node v24.22.0 resolved by VitePlus. This includes concurrent continuations, progressive response delivery, explicit Close producer cancellation, unread-owner cleanup, metadata, large request payloads, parent demand and header effects.
- Fresh at the same commit: `go test -race . -run 'TestNative(Bridge|Fetch|Development)' -count=1 -timeout=90s` passed in 1.912 s.
- Inspected parent-run `/tmp/skgo-dev-repairs-final-build.log`: production Vite build and goja lowering completed successfully, ending with adapter output and `done`.
- Inspected the earlier parent-run actual-session log at `814801d`: `TestRealSessionNativeRecoveryAndIsolation` passed in 25.92 s, package 26.243 s. The latest commit changes continuation producer cancellation, not configuration/session publication.
- The new `/tmp/skgo-dev-repairs-final-session.log` was still running when this review completed. Full Go and development/production browser qualification are parent-owned follow-through. This review does not claim those unfinished executions passed.

Only the requested review artifact was written. Private root fixtures did not mutate shared applications or generated example artifacts. No PR, merge, deployment, release, new save-to-visible benchmark or Linux qualification is implied by this outcome. The accepted automatic direct-SSR-subfetch cookie limitation and previously disclosed qualification boundaries remain unchanged. Passing review and focused tests support the repair; the parent's current real-session and broad behavioral checks remain separate evidence.
