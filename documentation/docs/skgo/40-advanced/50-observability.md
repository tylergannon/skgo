---
title: Observability
summary: Emit Kit's OpenTelemetry spans from the Go server and annotate them through Event.Tracing.
proof:
  - TestTracingFrameworkOperations
  - TestTracingErrorsAndDisabled
  - TestTracingIncomingAndConcurrentContext
  - TestTracingActionsAndSequencedHooks
  - TestTracingConfiguredPropagator
  - TestNativeTracingAnnotationBudget
  - TestNativeFetchUsesSharedOwnerAcrossKitTracedEventCopies
---

SKGo emits the same server-side [OpenTelemetry](https://opentelemetry.io) spans that SvelteKit does, and creates them in Go. You annotate them, and add your own child spans, from Go loads, actions, remote functions, `+server` routes and the `handle` hook. The Go OpenTelemetry SDK collects and exports them. Kit's `src/instrumentation.server.ts` has no place in an SKGo app.

## Enabling tracing

Turn tracing on in the same place as in Kit, with the `tracing.server` option of the SvelteKit plugin:

```ts
/// file: vite.config.ts
export default defineConfig({
	plugins: [
		sveltekit({
			adapter: skgo(),
			tracing: { server: true }
		})
	]
});
```

The adapter writes the setting into the build manifest, and the Go server reads it at startup. With tracing off, SKGo creates no spans. `Event.Tracing` then holds non-recording spans, so annotation code still runs safely and does nothing. Kit's [overhead note](https://svelte.dev/docs/kit/observability) applies here too.

## Configuring a provider

SKGo uses the global OpenTelemetry tracer provider and text-map propagator. Install both from `newHandler`, not from `main`: `skgo dev` builds the app from `NewDevelopmentHandler` and never runs `main`. A function like this does it; the options are the provider's, so the exporter stays your choice:

```go
/// file: tracing.go
/// include: example/tracing.go#tracing-setup
```

The packages are `go.opentelemetry.io/otel`, `go.opentelemetry.io/otel/sdk/trace` and `go.opentelemetry.io/otel/propagation`, plus an exporter such as `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp`, whose `otlptracehttp.New` result you pass as `sdktrace.WithBatcher(exporter)`. The root span reads an incoming `traceparent` header through the global propagator. If you leave the propagator unset, every request starts a new trace even when a proxy or client sent trace context. Call the provider's `Shutdown` when the server stops, so that batched spans are flushed.

## Spans SKGo emits

The span names and attributes follow Kit's, under the tracer name `sveltekit`:

| Span                                | Covers                                                                                                                         |
| ----------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| `sveltekit.handle.root`             | the whole request; attributes `http.route`, `http.method`, `http.url`, `sveltekit.is_data_request`, `sveltekit.is_sub_request` |
| `sveltekit.handle.sequenced.<name>` | each function in a `params.Sequence`, named by function or by index                                                            |
| `sveltekit.resolve`                 | the call to `resolve` from your `handle`; `http.route`, `http.response.status_code`, `http.response.body.size`                 |
| `sveltekit.load`                    | a Go server load; `sveltekit.load.node_id`, `sveltekit.load.node_type`, `sveltekit.load.environment`, `http.route`             |
| `sveltekit.form_action`             | a page form action; `sveltekit.form_action.name`                                                                               |
| `sveltekit.endpoint`                | a `+server` handler; `http.route`, `http.method`                                                                               |
| `sveltekit.remote.call`             | a remote function call; `sveltekit.remote.call.id`, `sveltekit.remote.call.type`, `sveltekit.remote.call.name`                 |
| `sveltekit.remote.form.post`        | a remote form submission; `sveltekit.remote.form.post.id`                                                                      |

Universal `load` functions that run during a server render are traced too. Kit's code runs inside the Go renderer, and Go creates and exports those spans on its behalf. When a span ends with a redirect, an action failure or an error, SKGo sets `<span>.result.type` to `redirect`, `failure`, `known_error` or `unknown_error`. An exception is recorded on the span only when the error is unknown or has a status of 500 or above, as in Kit.

A subrequest made with `event.Fetch`, or with `fetch` in a universal load during server rendering, carries the active span and baggage, so the subrequest's root span is a child in the same trace.

## Augmenting the built-in tracing

`skgo.Event` has a `Tracing` field, which mirrors Kit's `event.tracing`:

```go
type Tracing struct {
	Enabled bool
	Root    trace.Span
	Current trace.Span
}
```

`Root` is the `sveltekit.handle.root` span. `Current` is the span for whatever is running your code: the load, the action, the endpoint or the remote call. Set attributes on either one. To add a child span, start it from the request context, because that context carries `Current`:

```go
/// file: src/routes/traced/page.server.go
/// include: example/web/src/routes/traced/page.server.go#traced-load
```

Code that receives a `context.Context` gets the event with `skgo.EventFrom`. This covers remote functions, actions and `+server` handlers, where the context is `r.Context()`:

```go
/// file: src/routes/traced/traced.remote.go
/// include: example/web/src/routes/traced/traced.remote.go#traced-remote
```

## In development

Under [`skgo dev`](development-server), Kit runs in Node and creates its own framework spans: `handle`, page loads and rendering. Your Go code still sees `Event.Tracing`, and its `Root` and `Current` stand for the Node spans. Attributes, status and events you set on them in Go are relayed to Node with the response, up to 3 KB per request; anything beyond that is dropped. Child spans that you start in Go are created and exported by the Go provider.

To collect Kit's own spans in development, register a Node tracer provider before Vite starts, for example with a file loaded through Node's `--import`. Keep the file out of `src/`: the SKGo adapter does not declare support for `src/instrumentation.server.ts`, because production has no Node process to load it into.

## Not supported

- **`instrumentation.server`.** Configure OpenTelemetry in Go instead. The [bundling caveats](https://svelte.dev/docs/kit/observability#Bundling-caveats) in Kit's docs do not apply.
- **Browser spans.** Neither Kit nor SKGo traces the client. To join a browser trace, send `traceparent` and `tracestate` headers on your requests. The root span picks them up through the propagator.
