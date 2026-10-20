# Development host without goja

finding: Importing a package costs link time even when the binary never calls it. The dev host linked goja's 11 packages because the app's shared handler reaches `skgo.NewSSR` through `NativeDevelopment.NewSSR`, and `internal/ssr`'s package initializers are linked anyway. Dead-code elimination cannot remove them. Only leaving the package out of the build does.

implementation: `hotreload.Toolchain.command` appends `-tags=skgo_development` to `GOFLAGS` for every command the development profile runs: host discovery, the host build and patch loads. Under that tag, `internal/ssr/engine_development.go` replaces the goja engine with a stand-in that refuses to render. Codegen runs the stock Go with `-trimpath` and never shared cache entries with the host, so it does not take the tag.

trap: `checkFlags` already refused `-tags` in the environment's `GOFLAGS`. A `-tags` value set with `go env -w GOFLAGS=...` used to reach the host build; it is now overridden, because the profile's own `-tags` comes last.

measurement: In the stub experiment, the host link on the ledger fixture dropped from 241 ms to 165 ms (medians of 18 builds), and go-api-field from 1344 ms to 1173 ms (medians of 15 samples, same run). The rest of the 171 ms is outside the link and was not attributed.

trap: Nothing the native toolchain builds can render through goja any more. `TestGeneratedNativeHandlerContract` used a production composition with a hand-written SSR bundle. It now composes through `NativeDevelopment`: page data is read from `__data.json`, and form actions go through `/_skgo/dev/begin`, `action` and `end`, as Kit calls them in a dev host. Its goja-only consumer, a render-time fetch, was deleted. A `NativeDevelopment` event fetch of a page or endpoint goes to Kit's socket, so plain requests in that fixture bypass the callback handler to keep its in-process nested fetch.
