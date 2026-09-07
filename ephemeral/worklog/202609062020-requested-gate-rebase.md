# rebasing the gate onto #36 — corrections and traps

## Port 8080 is contested, and the server lies about owning it

Another session's worktree had `go run ./example/cmd -listen 127.0.0.1:8080`
running. My own server logged

    skgo-example: pid 92... listening on http://127.0.0.1:8080 in prod mode
    skgo-example: listen tcp 127.0.0.1:8080: bind: address already in use

— in that order, because `example/cmd/main.go` prints the line *before*
`ListenAndServe`. So the "listening" line is not evidence of anything, and the
suite then ran green-ish against a stranger's binary: ten scenarios red across
`loads`, `transport`, `routes` and `gate`, all of which read as application
bugs and none of which were. `just ports` showed the owner as `skgo-exam`
rather than `cmd`, which was the only tell.

Do not fight for 8080. Pick a port, bake it into the build, and use it
end to end:

    ORIGIN=http://127.0.0.1:8181 just build
    go run ./example/cmd -listen 127.0.0.1:8181
    ORIGIN=http://127.0.0.1:8181 just e2e prod

and for the proxied path, a vite of your own with the same origin, because kit
fixes `paths.origin` at build time on both sides:

    cd example/web && ORIGIN=http://127.0.0.1:8181 mise x -- \
      node_modules/.bin/vp dev --host 127.0.0.1 --port 5273 --strictPort
    go run ./example/cmd -listen 127.0.0.1:8181 -proxy http://127.0.0.1:5273

Then read the server's log and check `lsof -nP -iTCP:<port> -sTCP:LISTEN`
against the pid you started. Not the port — the pid.

## `requested(fn, limit)` has no limit to take when the query takes no argument

Kit's client calls a no-argument query with `undefined`, whose payload is the
empty string, and it collects the keys it wants refreshed into a `Set`
(`categorize_updates`). So the whole key space for such a query is one key,
`<id>/`, and `requested(getTodos, 5)` and `requested(getTodos, 1)` cannot
differ. `limit` there is not meaningless, it is always one — and a parameter
that can only hold one value is worse than no parameter, so
`RefreshRequestedNoArg` and `ReconnectRequestedNoArg` do not take one.

There is deliberately no `RequestedNoArg`. `Requested` exists so a handler can
choose *between* instances by their arguments; with one instance and no
argument its entire content is a boolean the handler already expresses by
putting the call under an `if`.

## An argument sent to a no-argument query is refused, not ignored

Kit's `create_validator` with no schema is `arg !== undefined → error(400)`, so
a payload posted under a no-argument query's id is a 400 on its own key rather
than a run with the argument dropped. `refuseAnyArgument` in `requested.go` is
that check. A duplicate of the legitimate instance is a different case: it
collides on the one key, the acceptance is registered after the loop that
records refusals, and the value wins — which is also what kit does.

## `EXPECTED_MODE` for a mutation run: only when the feature does not check it

The earlier worklog says use `prod`/`dev` only, because `the document response
came from skgo in the expected mode` compares `X-Skgo-Mode` against it.
`gate.feature` has no such step, so running *only* it under
`EXPECTED_MODE=gate-removed` is safe and keeps the deliberate-break frames out
of `ephemeral/screenshots/gate/prod/`. Running anything else that way is not.
