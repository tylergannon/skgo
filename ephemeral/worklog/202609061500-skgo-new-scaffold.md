# `skgo new`: the first honest consumer

trap (cost the most, and it was mine to make): the machine's `go env` carries
`GOPRIVATE=github.com/pagerguild/*,github.com/tylergannon/*`, which sets
`GONOPROXY` and `GONOSUMDB` with it. Every module under `github.com/tylergannon/*`
is therefore fetched straight from git and **no proxy is consulted at all**. The
scaffold test publishes this checkout into a throwaway `file://` proxy so the
new project can require it by version; with `GONOPROXY` in force that proxy was
silently ignored and the failure read `unknown revision v0.0.0-scaffoldtest` —
which looks like a bad proxy layout, not like a bypassed one. Any test that
serves skgo from a local proxy has to set `GOPRIVATE=`, `GONOPROXY=none`,
`GONOSUMDB=none` and `GOSUMDB=off` explicitly. The setting itself is right for
Tyler and will be right for other consumers; it is only the local-proxy trick
it defeats.

trap: `(./bin/myapp > log 2>&1 &)` detaches, and `pkill -f 'trial/myapp/bin/myapp'`
did not reach it. The rebuilt binary then failed to bind, the old process kept
answering, and the screenshots were of the *previous* build — green run,
stale evidence. The only thing that caught it was reading the numbers in the
screenshot: `greetings before: 1` on a process that had supposedly just started.
Kill by port (`lsof -ti:PORT | xargs kill -9`) and check the bind succeeded
before trusting anything the server says.

fact worth keeping: `github.com/tylergannon/skgo` has no tagged releases, but
`@latest` resolves to a pseudo-version of `main` on GitHub. So a generated
project can require skgo today. `skgo new` takes its version from
`debug.ReadBuildInfo` when it was itself built from the module cache, and asks
the proxy for `@latest` when it was built from a checkout (where the build info
says `(devel)`, which no go.mod can require).

fact, checked rather than assumed: a `//go:embed all:build` whose directory does
not exist yet does **not** break `go mod tidy` or `go generate ./...`. Only a
build or a vet fails. That is what lets the build gesture be one gesture:
`go mod tidy` → `vp install` → `go generate ./...` → `vp build` → `go build`,
with the embed satisfied only at the last step.

fact: foreign wire types resolve correctly out of a read-only module cache. A
scaffolded project whose remote function returns `skgo.ManifestParam` gets a
declaration package under `generated/wiretypes/github.com/tylergannon/skgo/`,
polytype projects it to `web/src/lib/skgo/github.com/tylergannon/skgo/types.ts`,
and `go build ./...` is clean. Nothing needed fixing; the path had never been
exercised anywhere but a writable checkout, and now it has been.

correction to a plan item: build plan §312 says the app's origin is fixed at
build time and "Go must listen at that origin in every environment or every POST
403s". The runtime check is narrower than that — `Remotes.ServeHTTP` compares
the request's `Origin` header against the configured origin, so what has to
match is the origin the *browser* uses, not the address Go listens on (behind a
proxy those differ legitimately). The practical trap is the same, and the
template closes it two ways: `ORIGIN` is written once, in `mise.toml`, and read
by both the frontend build and the binary's link step; and a mismatch now
answers with both origins and the file to change instead of a bare 403.
