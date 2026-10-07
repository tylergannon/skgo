# Prerender resolve options

decision: Installed Kit 3.0.0 calls preload after loads/rendering, the header filter during universal-load reads and serialization, and awaits the HTML transform. Retain callbacks on the suspended logical request and execute at those phases; nil options stay local to Kit.

decision: The build service has no renderer from which to discover SSROptions defaults. PrerenderServiceOptions.FilterSerializedResponseHeaders carries the shared default; adapter prerenderPackage selects an authored Go service entry using the same filter as production. Ordinary apps retain the generated main.

friction: Kit's crawler can expose an already-saved fetched endpoint body as an asset, without the original response headers. Reusing one endpoint path across independently prerendered header fixtures tested this cache behavior instead of the live callback. Use distinct literal paths and a dynamic endpoint for served parity.

correction: A rejected universal-load header read renders an error page, which still invokes the selected transform. A transform must not assume a successful load ran for an error response.

decision: On owner failure, send full error replies and notify outstanding predicate cells before closing the channel or waiting for process cleanup. A terminal worker relay never resets its cell or dispatches another request after failure, so a late reply cannot become a later answer.

friction: Go's slash-separated -run patterns match each subtest name independently; timeout$ also matched worker-timeout and repeated a 30-second build. Use an anchored subtest component, /^timeout$, when selecting one failure mode.
