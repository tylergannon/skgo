---
title: Performance
summary: The renderer pool, precompressed assets, streaming, batching and what to measure.
proof:
  - example/e2e/features/batch.feature
  - example/e2e/features/stream.feature
  - TestAPrecompressedImmutableAssetKeepsItsCacheControl
  - TestStaticHandlerETagAnswers304
  - TestEachEncodingHasItsOwnETag
---

Most of Kit's [performance guide](https://svelte.dev/docs/kit/performance) is about the client and applies unchanged: code splitting, preloading, images, fonts and packages. This page is about what changes when one Go binary serves the application.

## Diagnosing issues

Measure the binary, not the development server. Under `skgo dev` Kit renders in Node through Vite, which says nothing about how the embedded renderer behaves. Run `just build`, start `bin/<app>`, and use your browser's network and performance tools or [PageSpeed Insights](https://pagespeed.web.dev/) against it.

The server is an ordinary Go process, so Go's profiler works. Serve `net/http/pprof` on a private listener, never on the public address:

```go
import _ "net/http/pprof"

go func() { log.Println(http.ListenAndServe("127.0.0.1:6060", nil)) }()
```

`go tool pprof http://127.0.0.1:6060/debug/pprof/profile` then shows where CPU time goes, in your Go code and in the JavaScript engine alike. With Kit's `tracing.server` option on, each request also emits Kit's spans from Go; see [Observability](observability).

## The renderer pool

Each page render takes one runtime from a pool of embedded JavaScript engines. `SSROptions.Runtimes`, set where `server.go` calls `NewSSR`, bounds how many pages render at once; zero, the default, means one per CPU. A request that arrives when every runtime is busy waits for one.

Every runtime evaluates the server bundle and keeps its own copy of the application's module state, so memory grows with the pool while the compiled program is shared. It is a memory-for-throughput dial and nothing else: lower it on a small machine, raise it where renders queue while the CPU is idle.

## Static assets

The adapter's `precompress` option is `true` by default, so `vp build` writes `.br` and `.gz` siblings of every client file. The Go server answers with Brotli when the request accepts it, then gzip, then the original, and it reads `Accept-Encoding` properly: `br;q=0` gets gzip. Each representation has its own `ETag` and answers a conditional request with `304`, and files under `/_app/immutable/` carry `Cache-Control: public, max-age=31536000, immutable`.

Only files from the build are compressed. Pages, `__data.json` and remote responses that Go produces per request go out uncompressed; compress them at a proxy or CDN, or wrap the handler in compression middleware of your own.

## Fonts

A document preloads its JavaScript and CSS and not its fonts. To preload a font, answer `Preload` from the `handle` hook's `resolve`, as Kit's `resolve(event, { preload })` does:

```go
func preloadFonts(ctx context.Context, event params.RequestEvent, resolve params.Resolve) (*http.Response, error) {
	return resolve(ctx, event, skgo.ResolveOptions{
		Preload: func(input skgo.PreloadInput) bool {
			return input.Type == "js" || input.Type == "css" ||
				(input.Type == "font" && strings.HasSuffix(input.Path, ".woff2"))
		},
	})
}
```

`PreloadInput.Type` is `js`, `css`, `asset` or `font`. Stylesheets are linked whatever the function answers.

## Navigation

Kit's client preloads code and data when a link is hovered or tapped; [link options](https://svelte.dev/docs/kit/link-options) control it. Preloading data runs your Go loads through `__data.json`, so a load has to be cheap enough to run on hover, or the link should say `data-sveltekit-preload-data="tap"`.

### Non-essential data

Return slow data that the page can draw without as a `skgo.Deferred` started with `skgo.Async`. The document goes out at once and the value arrives on the same response when the goroutine finishes; see [Streaming with promises](load#Streaming-with-promises).

### Preventing waterfalls

The loads of a page and its layouts run concurrently, each on its own goroutine, and a load waits for its parent only when it calls `skgo.Parent`. Call it after starting the work that does not need it, or not at all.

Inside one load, independent calls run one after another unless you start them together: goroutines, `errgroup` or `skgo.Async` all work. Between a page and its remote functions, make a query the page calls once per row into a batch query, so that many calls become one request and one Go call:

```go
/// file: src/routes/batch/quotes.remote.go
/// include: example/web/src/routes/batch/quotes.remote.go#batch
```

A fetch of one of your own routes, from a Go load or from a component during a render, does not use the network: it is dispatched in-process to the same handler. Calling the Go function directly is still cheaper than fetching a route that wraps it.

## Hosting

The binary serves everything from one process: no Node, no separate static file server, every load and remote call in the same address space. Place it near your database as well as near your visitors.

A platform that scales to zero adds a cold start to the first request after an idle period: the process starts, and the first renders build their runtimes, each of which evaluates the server bundle. When you measure, wake the server with one request and time the ones after it. The generated `cmd/main.go` serves plain HTTP/1.1, so terminate TLS and speak HTTP/2 at a proxy in front of it, as Kit's [hosting advice](https://svelte.dev/docs/kit/performance#Hosting) asks; see [Deploying](deploying).
