# The goja bundle is a fourth environment of kit's build

Traps and corrections only; the design is `ephemeral/brief/2026-09-07-adapter-research-continuation.md`
and the spike it names, and both held up. What follows is what they did not say.

## A `just serve` you killed is still serving

The whole first parity run was worthless and looked perfect. `just serve` is
`go run ./example/cmd | tee`; killing the pid `just` printed kills the shell and
the `tee`, not the compiled binary `go run` exec'd, which keeps the port. The
next `just serve` logs "listening on ..." — that line prints *before* the bind —
then dies with `address already in use`, into a log nobody reads because the
suite is already running. Every request in that window was answered by the
*previous* build, so a refactor's output was compared against itself and came
out byte-identical.

What the comparison is worth depends entirely on which process answered it. Do
not use `just serve` for an A/B. Build the binary and run it yourself, so the
pid you hold is the pid that bound, and prove the bind with `lsof -nP
-iTCP:<port> -sTCP:LISTEN` rather than with a log line:

    go build -o "$bin" ./example/cmd
    "$bin" -listen 127.0.0.1:$port >"$log" 2>&1 & pid=$!

`ephemeral/index/README.md` already says "the 'listening on' line prints before
the bind". It does not say the process outlives the pid you killed, and that is
the half that costs you a whole run.

## Two builds of the same source differ in three places, and only three

Kit derives `version.name` from `Date.now()`, so an A/B of two builds differs by
that timestamp, by `__sveltekit_<djb2(version.name)>` (the boot global), and by
the ETag over the bytes that hold them. Normalising those three — plus
`accountSerial`, which increments per request — makes the comparison exact.
Every route came out identical, signed out and signed in, statuses included.
Anything *else* differing is a real difference.

One more thing does move, and it is not the build: `accountSerial` counts how
many times the account load has run *in that process*, so a second capture run
against a server that already answered one starts four higher and reads exactly
like a regression. Give each side a freshly started server and the same request
sequence.

## `vp dev` binds `[::1]`, not `127.0.0.1`

It prints `Local: http://localhost:5241/` and listens on IPv6 loopback only, so
`-proxy http://127.0.0.1:5241` is a 502 that reads like the dev server failing
to start. `-proxy 'http://[::1]:5241'` works.

## An alias returned from `resolveId` is not realpathed

Vite realpaths what its own resolver returns; an id a plugin hands back is taken
literally. Aliasing kit internals through the pnpm symlink
(`node_modules/@sveltejs/kit/src/...`) makes `devalue` unresolvable from
`transport.js`, because the lookup walks up beside the *link* rather than beside
kit. `realpathSync` the kit directory first — the same reason the esbuild build
did, which its comment said and I removed on the theory that vite handled it.

## `applyToEnvironment` is what keeps prerendering alive

Take it off the plugin — so `$app/server` and the `node:async_hooks` stub reach
kit's own `ssr` environment too — and the build fails at `Prerendering` with
Svelte's `async_local_storage_unavailable` on `/about`. That is the whole reason
the bundle is an environment rather than a pass over kit's output: the
substitution has to be scoped, and after kit's graph is linked there is nothing
left to scope it to.

## goja_nodejs's `URL.origin` leaves the port out, and cannot be corrected

`new URL('http://127.0.0.1:8080/').origin` is `http://127.0.0.1`. The accessor is
defined `configurable: false` on `URL.prototype` (`url/url.go`'s
`defineURLAccessorProp`), and the object the constructor returns is a Go host
object, which goja will not let an own accessor shadow either
(`TypeError: Host objects do not support accessor properties`). There is no way
to correct it from Go or from JavaScript, and upstream `master` still says the
same thing.

**A render path reached it within the day.** #78's `event.fetch` guards the call
into Go's own handler with `target.origin !== url.origin`, which was written
against the hand-rolled JavaScript URL this branch deletes — that one built
`origin` from `protocol + '//' + host`, port included. Carried across
unchanged, the guard would have treated a page on :8080 and a URL on :9999 as
the same site and dispatched a cross-origin fetch to this app's routes. The
cross-origin scenario would still have passed: it differs by hostname too.

So `same_origin` in `skgo-adapter/entry.js` compares protocol, hostname and port
itself. Anything else in the engine that comes to want an origin has to do the
same; `TestTheEngineOriginLeavesThePortOut` is where that is written down.

Worth generalising: every polyfill this branch replaced with a Go binding is a
place where a landed feature may have been written against the old one's
behaviour, not against the standard. The diff being carried is where to look.

## An `allowBuilds` line survives the dependency that explains it

Dropping `esbuild` from `package.json` does not stop pnpm installing it: it is
an optional transitive dependency of `@voidzero-dev/vite-plus-core`, so removing
`allowBuilds: esbuild: false` from `pnpm-workspace.yaml` makes `pnpm install`
exit 1 with `ERR_PNPM_IGNORED_BUILDS`, write its own placeholder back into the
file, and — in CI, where `just install` is the first step — skip every step
after it. The checks go red without a single test having run.

`pnpm why esbuild` in the app's tree, not `package.json`, is what says whether
the line can go.

## `pnpm test -- <name>` does not filter

The gesture in `ephemeral/index/README.md` runs the whole suite; the positional
argument does not reach playwright. `pnpm test -- --grep "<scenario title>"`
does. Watch the count, not the tail: a filtered-looking run that reports the same
total as an unfiltered one filtered nothing.

## The node-numbering trap, demonstrated

Mutating the node table to kit's own numbering — identical up to the first
prerendered node, one off after it — leaves `/` passing and fails every
other row of `engine-build.feature`. That is the shape the brief predicted, and
it is why a parity check that stops at the home page proves nothing.
