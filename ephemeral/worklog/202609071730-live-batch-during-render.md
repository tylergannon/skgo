# Live and batch queries during render (#61)

## A `pending` snippet on a boundary opts its subtree out of server rendering

`/todos` wrapped `<TodoCount />` — a `query.live` — in a `<svelte:boundary>`
with both a `pending` and a `failed` snippet. With the live query answered
during the render the document still carried `counting…`, and the count only
appeared after hydration. Removing the `pending` snippet is what puts the
number in the markup; nothing on the Go side changes. Verified by putting the
snippet back, rebuilding and curling `/todos`.

Svelte's server compiler is the reason (`SvelteBoundary.js`: when a pending
snippet is present it renders that and nothing else), so this is true on kit's
own engine too. A page that wants a value *in the document* must not name a
pending state for it; `/stream` keeps its pending snippets, and should, because
a deferred load value genuinely is not there yet.

## `vp dev` binds `[::1]` only

`-proxy http://127.0.0.1:5199` gets `connection refused` and the page is a 502
that says so. Vite listens on IPv6 loopback; `-proxy http://localhost:5199`
works. `just serve` has no proxy arm, so the dev-mode server is started by hand:

    (cd example/web && mise x -- vp dev --port 5199 --strictPort) &
    go run ./example/cmd -listen 127.0.0.1:8124 \
      -proxy http://localhost:5199 -origin http://127.0.0.1:8124

## `go test ./...` can fail once under concurrent worktrees

`internal/newapp` scaffolds a module into the shared `GOMODCACHE` under a
timestamped pseudo-version; with two missions running `go test` at the same
time it failed with `open .../skgo@v0.0.0-scaffoldtest.../cmd/skgo/main.go: no
such file or directory` and passed on the next run. Not a flake in the code
under test — a flake in sharing one module cache.
