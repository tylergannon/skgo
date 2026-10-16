# Development-server repair review, round 01

Outcome: **material findings remain**.

Target: `/Users/tyler/.codex/worktrees/5f35/skgo`, baseline `1d9e23e1760266cc83367c00999105e28db19944`, repair commits through `5ebe046` plus the working-tree cancellation cleanup in `internal/devserver/session.go` visible during review. The user authorized implementing the prior development-server review findings and reviewing the changed code. This is an implementation review, not a claim of full development or production qualification.

## Authority and scope

Read the repository's complete `AGENTS.md`, `agent-protocol`, `adversarial-review`, the current `ephemeral/projects/dev-server/{GOAL,DESIGN,CONTINUE}.md`, the prior review at `/Users/tyler/.codex/worktrees/9b3a/skgo-project/ephemeral/reviews/20261015-skgo-development-server-review.md`, and the repair worklogs. Followed `/Users/tyler/src/skgo-token-cache/index/README.md` and `index/dev-native-kit-uds.md` to the current authority; verified the installed Kit package is `3.0.0` and read `ephemeral/sveltekit-current/SKILL.md` and the installed load/header implementations.

The selected architecture remains Kit-owned development SSR and HMR, Go application behavior, one public coordinator and private per-session Unix sockets, with compatible native Go updates and rebuild fallback. Automatic direct SSR subfetch response-cookie adoption remains the explicitly accepted temporary limitation. No instruction narrowed which defects could be considered. The read-only boundary and restriction on starting shared processes were respected; only this review artifact was written.

## Finding

### 1. issue — concurrent fetch continuations close a response body while Go is still reading it

Evidence: `dev_fetch.go:158` constructs the continuation result with `Body: r.Body`. After sending that result to its individual reply channel at lines 178–186, the same HTTP handler immediately selects another continuation at lines 188–213. It writes that next instruction and returns, which allows `net/http` to close the earlier incoming request body even though the Go hook still owns the `http.Response` whose body is that reader.

The per-continuation reply channels repair response identity, but they do not extend the uploaded response body's lifetime. If a `HandleFetch` implementation calls its supplied continuation concurrently and reads the returned bodies, one continuation can make the prior HTTP handler return before the other consumer finishes its read. The load then fails with HTTP 500 instead of receiving both valid results. This violates the request-aware bridge requirement to preserve ordinary fetch behavior and the repair's own concurrent-continuation regression.

Reproduced twice using ambient Node `v26.11.1`, then independently with the project's managed Node `v24.22.0`. From `example/web`, run:

```sh
vp exec go -C ../.. test . -run '^TestNativeBridgeConcurrentFetchContinuationsKeepTheirReplies$' -count=1 -v
```

The real Go handler and actual adapter callback fixture reported:

```text
=== RUN   TestNativeBridgeConcurrentFetchContinuationsKeepTheirReplies
    dev_bridge_regression_test.go:294: native joined bridge: exit status 1
        Error: skgo: development fetch-response HTTP 500: http: invalid Read on closed Body
        Node.js v24.22.0
--- FAIL: TestNativeBridgeConcurrentFetchContinuationsKeepTheirReplies (0.07s)
FAIL
FAIL github.com/tylergannon/skgo 0.289s
```

The fixture starts `/first` and `/second` in separate Go goroutines, checks each returned body against its own literal, and requires the selected second response to reach JS. No derived expectation or mocked Go handler is involved. Keep each continuation body's ownership valid until its Go consumer releases it; preserving a reply channel alone is insufficient. Preserve progressive response and cancellation behavior while repairing this boundary.

## Evidence inspected and executed

Inspected the complete baseline diff and relevant surrounding code across supervisor publication/recovery, adapter readiness/configuration hashing, captured compiler/workspace selection, native host construction and type-aware carry injection, the explicit development constructor/ordinary filesystem seam, generated app/example constructors, parent-load transport, effect deltas, fetch metadata/body forwarding, request-owner cancellation and the added tests.

Fresh checks:

- `go test . -run 'TestNativeBridge|TestNativeFetch|TestNativeDevelopment' -count=1 -v`: failed only in the concurrent-continuation test above. Other selected tests passed, including large fetch bodies, URL/redirect metadata and clones, unread fetch cancellation, demand-driven parent loads and overlap, parent failure identity, concurrent header deltas, action effects with and without hooks, configured handler behavior and filesystem composition.
- `go test ./internal/adapter -run TestNativeFetchUsesSharedOwnerAcrossKitTracedEventCopies -count=1 -v`: passed.
- `go test -race . -run 'TestNativeBridge(ActionEffects|IndependentLoads|FetchBody|ParentFailure|ConcurrentLoad)|TestNativeFetchOwner|TestNativeDevelopmentComposes' -count=1 -v`: passed, `1.933s`, with no race report. This intentionally does not claim the known failing continuation test passed.
- The managed-Node reproduction above failed with the same body-lifetime error. The first ambient-node root run returned a failing test exit; later diagnostic displays filtered out the enormous data-URL stack but retained the raw error and test verdict.

The focused fixtures start only private disposable fixture listeners. No shared example app was started, and no example/generated artifact was modified by this review. Other owners are running native-host/session proof and repairing cancellation cleanup; those checks and full Go/development/production browser qualification are not claimed here. The historical qualification recorded in `DESIGN.md` is retained context, not a new pass against these repairs.

The earlier concrete repair targets have appropriate implementation changes and regression cases: configuration/environment identity now drives deliberate Go reload, captured workspace/environment settings reach native compilation, continuation payload bodies moved out of headers, effects are tracked at request/response-state ownership, independent Go loads can start before parent data is demanded, owner cancellation retires unread exchanges, metadata survives response reconstruction and cloning, and development setup no longer depends on hidden `fs.FS` identity. This assessment does not waive the reproduced failure above or substitute for the pending broader runtime proof.
