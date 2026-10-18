# Group 2: Graceful production shutdown

## #361 Generated cmd/main.go does not shut down gracefully on SIGTERM

https://github.com/tylergannon/skgo/issues/361

At v0.36.0 (`3b382953`, which is also current `main`), the `cmd/main.go` that `skgo new` generates has no graceful shutdown.

**Template:** [internal/newapp/gofiles/cmd/main.go.tmpl:17-34](https://github.com/tylergannon/skgo/blob/3b382953fb97f77aecd1f5122d76dfbe586fec89/internal/newapp/gofiles/cmd/main.go.tmpl#L17-L34):

```go
log.Printf("{{.App}}: listening on http://%s", *listen)
if err := http.ListenAndServe(*listen, handler); err != nil {
	log.Fatal(err)
}
```

There is no signal handling, no `http.Server`, and no `Shutdown`.

**Effect:** Go's default action on SIGTERM ends the process immediately. Fly.io, Kubernetes, systemd and `docker stop` all send SIGTERM before a deploy or a scale-down, and the generated server then:

- drops in-flight requests, including streamed documents with pending `Deferred` values, live-query streams and form posts mid-action;
- never runs deferred cleanup in `main`. v0.36.0 asks applications to configure an OpenTelemetry provider before serving ([tracing.go:19-22](https://github.com/tylergannon/skgo/blob/3b382953fb97f77aecd1f5122d76dfbe586fec89/tracing.go#L19-L22)), and a provider's `Shutdown` (which flushes batched spans) is exactly such a deferred call, so the last spans before every deploy are lost.

The skgo docs say nothing about shutdown or SIGTERM for the web server. The only shutdown text in the README is about native application lifecycle callbacks ([README.md:598](https://github.com/tylergannon/skgo/blob/3b382953fb97f77aecd1f5122d76dfbe586fec89/README.md#L598)).

**Expected:** the template uses `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)`, an `http.Server`, and `srv.Shutdown(ctx)` with a bounded timeout, so deferred cleanup runs after the listener drains. The README's deployment section should say that SIGTERM drains in-flight requests. If live queries hold connections open, say how long shutdown waits for them, or call `RegisterOnShutdown` to close them.

🤖 Generated with [Claude Code](https://claude.com/claude-code)

### Comment by tylergannon at 2026-10-18T16:57:24Z

**Triage — 2026-10-18**

Checked against current [`main` at `920b6aca`](https://github.com/tylergannon/skgo/tree/920b6aca738a65d6c046a67f4db2373596fafd03). Classification: `bounded-hard`.

Still applicable: internal/newapp/gofiles/cmd/main.go.tmpl calls http.ListenAndServe with no signal handling or shutdown. The example now has signal-aware composition, but that does not repair the generated application. Scope is bounded, but request draining, long-lived streams, timeout behavior and deferred cleanup require lifecycle judgment and a real generated-server contract.

<!-- skgo-issue-triage-20261018-920b6aca -->

## Initial orchestration assessment

Goal: applications produced by `skgo new` stop accepting work on SIGTERM/interrupt, drain in-flight requests within a bounded interval, then return through normal deferred cleanup. Document the chosen wait behavior for long-lived streams and timeout.

Proof: an actual generated application process receives SIGTERM while a request is held; that request completes before clean process exit, deferred cleanup is observed, and long-lived/timeout behavior is bounded. Ordinary Go tests must exercise the generated server rather than merely compare template text.

Potential design question: does the existing example, Go server lifecycle or cached precedent settle the timeout/long-lived-stream policy? Choose an existing established policy if present; only a materially new product choice requires HITL. The issue already specifies signal.NotifyContext, http.Server and bounded Shutdown.

## Token-cache annotation: executable

Research entered through the [cache index](/Users/tyler/src/skgo-token-cache/index/README.md), then its [source families](/Users/tyler/src/skgo-token-cache/index/source-families.md:34) and [Kit shutdown heading](/Users/tyler/src/skgo-token-cache/index/upstream-docs/headings/kit.md:198). The cache holds the Go 1.27.2 standard-library source and older Kit adapter documentation; these are lifecycle references, not evidence that today's generated application drains.

- Go already specifies this lifecycle. [Server.Shutdown](/Users/tyler/src/skgo-token-cache/repos/dev-experience-20261015/frameworks/go-1.27.2/src/net/http/server.go:3252) closes listeners and idle connections, waits for active requests, and returns the context error if its deadline expires. Its explicit warning says `ListenAndServe` returns `ErrServerClosed` immediately: the program must wait for shutdown before returning. Deadline expiry does **not** itself close active connections; [Server.Close](/Users/tyler/src/skgo-token-cache/repos/dev-experience-20261015/frameworks/go-1.27.2/src/net/http/server.go:3213) is the separate force-close operation.
- [RegisterOnShutdown](/Users/tyler/src/skgo-token-cache/repos/dev-experience-20261015/frameworks/go-1.27.2/src/net/http/server.go:3313) starts protocol-specific shutdown and must not wait for it. Hijacked connections are outside normal HTTP shutdown. This is not a reason to create a new SKGO lifecycle abstraction or prematurely cancel ordinary in-flight requests.
- Cached [adapter-node documentation](/Users/tyler/src/skgo-token-cache/repos/dev-experience-20261015/frameworks/kit-3.0.0-upstream/documentation/docs/25-build-and-deploy/40-adapter-node.md:203) likewise rejects new work, drains, then ends remaining active connections at its timeout; cleanup follows closure. This is Node adapter prior art, **not** a Kit wire/client requirement or a mandate to reproduce Node environment variables in a Go application. The Go program owns the production listener and process.
- The current repository already chooses **10 seconds** in [example production main](/Users/tyler/.codex/worktrees/e413/skgo/example/cmd/main.go:58), with an independent background context for draining after the signal context is canceled. The [deployment documentation](/Users/tyler/.codex/worktrees/e413/skgo/documentation/docs/skgo/30-build-and-deploy/40-deploying.md:104) points users to that exact example. Reusing this bounded wait settles the ordinary scaffold default without HITL. Long-lived HTTP streams may drain until that bound; the timeout/connection outcome must be explicit in docs and real process proof.
- Current docs also describe the defect in [server composition](/Users/tyler/.codex/worktrees/e413/skgo/documentation/docs/skgo/30-build-and-deploy/30-server-composition.md:78) and [differences](/Users/tyler/.codex/worktrees/e413/skgo/documentation/docs/skgo/60-appendix/40-differences.md:102). The implementation changes those statements; the repository's documentation authoring/render contract applies. This is correction of existing public behavior, not a request for additional documentation machinery.

Question resolved: the issue's bounded drain and deferred-cleanup goal plus the existing ten-second example policy are sufficient. **#361 is executable now; no HITL product decision is missing.** Builder judgment remains in handling serve failure and timeout without bypassing cleanup. Acceptance is the actual generated process receiving SIGTERM, completing a held request before exit, running deferred cleanup, and exhibiting bounded behavior for a stream that does not complete; assertions must not merely inspect template strings. No new framework API, lifecycle registry or browser suite is required.
