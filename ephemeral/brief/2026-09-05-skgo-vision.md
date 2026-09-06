# skgo vision and build order — Tyler Gannon, 2026-09-05

Transcribed verbatim from the session brief. The first block is the session
instruction; everything after the divider is the product/build brief.

---

Write down the following, VERBATIM.  Once you've done so, carefully consider it.  I want you to do the whole initial research and planning.  Find out where within the junkyard and other ephemeral directories, we have valid guidance that and correct examples of how to do things.

Do not do any development.

use the /df-semantic-index skill but build the semantic index of the materials from ./ephemeral/inspiration, and put the index into somewhere in ./ephemeral as well.  

Do the indexing with respect to actual or potential solutions to the project as described here.  That means you can selectively avoid indexing topics or content that is obviously not going to be helpful in shipping this software.


-------

okay so here's the deal.  let's get this written down.



I honestly think that you guys really figured out some good stuff you just spent so much time stewing on the plan and on proof machinery that you never actually got anywhere.



So we need to resurrect good findings from there but then actually build this properly, in the right order.



## First, my thoughts on *what this is.*

skgo (pronounced "skaygo") is a SvelteKit backend written in Go.  Developers can
build a server in Go and serve a SvelteKit server from it, including file-based
routing, remote functions, and PageServerLoad / LayoutServerLoad.  At present,
in order to avoid needing to run a javascript runtime on the server, there is
no support for SSR.

`skgo` offers the same automagical type system that SvelteKit provides.  
You export a function from some `*.remote.go` file and `skgo` generates
a type-safe remote function stub that can be called from the client, as
well as a Go endpoint handler that will be served as the actual backend
implementation of the remote function.  Client code can call the remote
function in exactly the same way as SvelteKit remote functions are used.
Similar case with `PageServerLoad` / `LayoutServerLoad` --> you declare
and implement it inside of `layout.server.go` or `page.server.go`, and
`skgo` generates the appropriate endpoint handler for you so that routes
and layouts will be served in a transparent way.

So it's not a _SvelteKit Adapter_ in the true sense.  Rather it's a wrapper
frontend that replaces ALL actual server endpoints with identical Go
implementations.

## How to build it.

We made a mistake on the first time building it, by asking the agents to
begin by building a SvelteKit application that runs in Node, and *then*
to build the Go version and expect it to work identically.  The mistake
there was that the agent got lost forever trying to figure out how to
build a PROOF system.  We'll end up with a proof system but it'll evolve.





So here's what I want you to build first.  Start with the right shape:

```
skgo/
   go.mod (github.com/tylergannon/skgo)
   example/
     go.mod (github.com/tylergannon/skgo/example)
     cmd/
       main.go
     businesslogic/
       stuff.go
     web/
       package.json
       src/
         lib/
           shared.remote.go
           shared.remote.ts  (generated stubs)
         routes/
           go.mod (github.com/tylergannon/skgo/example/web/routes)
           data.remote.go
           layout.server.go
           page.server.go
           +page.server.ts   (generated stubs)
           +layout.server.ts (generated stubs)
           data.remote.ts    (generated stubs)
```

We're gonna eventually want a generator that will set up this shape with some basic
remote functions and stuff, because the release version of skgo will be able to
generate a new `skgo` project from scratch that will immediately give developers:

1. Latest version of SvelteKit (currently 3.0.0-next.25), with tsgo, rsvelte for
   the hygiene toolchain and the native Svelte compiler for build.
2. Latest version of `vp` (viteplus) with rust/go tools wherever possible.  Note
   that linting and formatting of `.svelte` files are not yet supported by oxlint/oxfmt.
3. Working dev setup that pairs `vp dev` (viteplus dev server) with an [Air](github.com/air-verse/air)
   server for hot rebuild/restart of the development Go server.  Go server provides the
   frontend both in dev mode and in production mode.
4. Server is written basically to have endpoints written in Go:
   - Remote Function and PageServerLoad/LayoutServerLoad implementation
   - Other endpoints as defined by the server
   - fallback route serves built SvelteKit files in production build, proxies
     them to the `vp dev` server in dev mode.
5. The CLI allows you to provide the URL for a server to proxy to, to support `vp dev`
   without needing to build a separate entrypoint.
6. Configured with `go:generate` tags sufficient to generate the server stubs.

But immediately we can just manually put this thing together without a generator,
just initially build it manually so that we can fan out and work on the project generator
at the same time as we develop the code generation that wires up endpoints and emits
javascript stubs.

So please note -- the project generator and the code generation should be built in parallel so basically
the first thing we *do* should be a basic working server that's got the Go service and proxy working
with the vp stack and no code generation.  Once that's done we can make a couple worktrees and fan out.


## Code Generation

In order to prove that code generation works, we must never write any working
endpoints in JavaScript.  We will 100% use generated JavaScript stubs instead,
and all of those should `throw` errors when called, therefore we will ALWAYS
know that we're getting a response from Go if we get a valid response.

You will **NOT** develop a deductive proof of correctness for the implementation
of the remote functions protocol.  You will only prove it works by implementing
the client side of the application and writing PlayWright tests against the application,
that use remote functions, and that pass.

Passing playwright tests is what is important.  Code review will be carried out
but only for a moderate number of passes and fixes will only be made to actual
real bugs (actual crashes or incorrect behavior) that happen in real (non-hypothetical)
scenarios.  Edge case bugs discovered during code review, if not an actual failure in
a required scenario, should be filed in github and move on quickly.  The bug report
can be handled asynchronously with forward development.

Okay.  Back to what it's supposed to do.

We won't run a JavaScript runtime in production, so all endpoints have to be implemented
in Go.

So I should be able to implement an arbitrary function* in Go, export it from a `*.remote.go`
file, and run code generation on that file, which will generate the following for each declared
remote function:

- A wrapper in Go that converts the published function to an `http.Handler`
- A mapping of a URL path to the Go `http.Handler` so that the http.handler can be mounted
  as the replacement endpoint in the Go HTTP server.  This should be exactly the same
  path as the endpoint generated by SvelteKit.
- use `polytype` to generate JavaScript types for the remote functions parameters and return values
- A JavaScript `*.remote.js` file that contains stub definitions for the remote functions,
  referencing the generated JavaScript types so that you get type safe calling from within
  the JavaScript application.
- A mapping in Go, with a `symlink` to the package containing the Go remote function implementation,
  to account for the fact that SvelteKit routes use characters forbidden in Go package paths.
  So we need to generate a symlink to the package containing the Go remote function implementation.
  It'll have to independently symlink all the `go` files in the root route to avoid picking up the `go.mod` file
  that's there.

This is actually pretty easy to make.  Maybe we use typescript instead of javascript, to begin with.
But this is pretty simple stuff.

So you build the server so that the remote functions will only work once you've properly implemented
the remote functions codec, at least across all of the Go types supported by `polytype`.

I feel like it should be something like this:

```src/routes/users/[id]/data.remote.go
package users_show

import "github.com/tylergannon/skgo"
import "github.com/tylergannon/skgo/example/businesslogic"

// the following just needs to exist in one root directory, just shown for
// understanding.
//go:generate skgo ./...

func getUser(id businesslogic.UserID) (skgo.Promise<businesslogic.UserProfile>, error) {
	return db.GetUser(id)
}

// call the marker function to indicate which type of remote function it is
// This will be picked up by the AST traversal to generate the remote function implementation
var _ = skgo.QueryFunction(getUser)

```

Right?  So then when we run `go generate`, we'll you'll also see something like the following.
Note that this is absolute bullshit code, looks like Go but consider it pseudo-code and
incomplete at that.

```src/routes/users/[id]/_data.remote.go
package users_show

import "github.com/tylergannon/skgo"

func GetUser(w http.ResponseWriter, r *http.Request) {
	// do shit
	userId := extractUserIdFromRequest(r) // this should be handled by the generated json.Unmarshaler implementation from polytype.
	userPromise, err := getUser(userId) // call to the handwritten impl.
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	user, err := userPromise.Get()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.Marshal(user, w)
	w.Header().Set("Content-Type", "application/json")
}

```

okay then the way this will be actually plugged in, this is the generated application code
wiring which we should eventually have once the project generator is working.

```example/cmd/main.go
package main

import "github.com/tylergannon/skgo"

import (
	"github.com/tylergannon/skgo/example/generated"
)

func main() {
	serveMux := http.NewServeMux()
	generated.RegisterHandlers(serveMux)
	http.ListenAndServe(":8080", serveMux)
}
```

obviously you'll need parameters and wiring for the actual server address/port
and proxypass destination (for dev mode), but not much more than that.
we don't provide any library-provided param parsing because the actual application
is likely to require quite a bit more configuration, so it should provide CLI params
parsing.

The `./generated/` directory has got the bindings that wire the generated handlers to
the servemux, so it'll look something like this:

```example/generated/config.go
package generated

// needs to know the source root where svelte routes are defined because the
// endpoints from underneath the routes require remapped package paths via symlinks
//go:generate skgo gen-bindings --src-root ../ --svelte-routes ../app/src/routes
```

---

## Addendum (same day, later in the session)

one extra piece of advice for during the build and development:

I think it's okay for you to build a pure SvelteKit application if you need to observe it in order to reverse-engineer the endpoints and the remote function / server load function server protocol.

But really please consider that you might not have to do very much of that, if you properly just port the javascript protocols directly to Go, along with the unit tests wherever possible.  As much as possible we should strive to just basically copy the upstream behavior by DIRECT observation of the code, rather than reverse-engineering. 

That should get us really close, to a point where we can then stand up our example application and tinker with it until we have the code generation stuff right.

Another point is that I have left the "generated wiring" stuff very incomplete.  I hope you can propose a simple way to generate those bindings, but please don't just hand-wave over it. if you don't know how to do it then you need to cop to that.

---

## Addendum: polytype status

I'm working on getting a new rc.6 version of polytype.  you shouldn't have any trouble using that by the time you're done with your research and are ready to start programming.

## Addendum, 2026-09-06 — all server-side work is Go's

Clarifying the direction, because a summary of this brief had drifted into
"anything not written in Go stays plain kit."

Every server endpoint is Go's. Running remote functions or server loads in
TypeScript is not a supported mode and is not a fallback — the JavaScript is a
generated stub that throws, and that is the point.

Server-side rendering, when it comes, should be a runtime embedded in the Go
binary rather than a supervised Node sidecar. One process, one binary. That is
an aspiration and is unproven; nothing should be designed around it until the
CSR base case has earned it.
