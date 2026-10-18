---
title: "skgo: server"
summary: SSR, the static handler, the build manifest, transport and the prerender service.
---

The pieces `skgo new` composes into `server.go` and `internal/serverhooks`.
`NewSSR` runs Kit's server-side rendering in a pool of goja runtimes over the
adapter's build output; `NewStaticHandler` serves the client assets and
prerendered pages and hands everything else to SSR; `ReadManifest` reads the
description of routes, pages and assets the adapter wrote. `Transport`
declares the domain types that cross between Go and the browser with their
own encoding. `RunPrerenderService` is the Go side of the build-time
prerender.

> PACKAGE: github.com/tylergannon/skgo
> SYMBOLS: SSR NewSSR SSROptions NewStaticHandler StaticOption ServedAsFile Manifest ManifestCSP ManifestClient ManifestEndpoint ManifestFont ManifestPage ManifestParam ManifestRoute ManifestSSR ManifestSSRNode CSPDirectiveValue Transport Transporter Transported RunPrerenderService PrerenderOptions PrerenderServiceOptions NativeDevelopment NewNativeDevelopment
