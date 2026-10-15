# Native development request bridge

decision: Preserve the application's actual constructor, FetchConfig, registry configuration and Go hooks through an explicit NativeDevelopment fs.FS carrier. Rebuilding a fresh generated boundary loses app-owned options.
friction: A bidirectional HTTP response relay deadlocks under net/http's normal request-body drain before header flush -> enable full duplex for the private streamed /response endpoint.
decision: Logical request handles live in the original process map and are cryptographically random; replacement rejects unknown handles instead of replaying callbacks. Callback disconnect cancels its own request, never the dev service.
