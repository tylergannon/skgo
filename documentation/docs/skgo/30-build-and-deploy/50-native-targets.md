---
title: Native targets
summary: "macOS and iOS today: released Swift clients, an application shell to try, and what is not finished."
proof:
  - TestGeneratedSwiftCallsTheGeneratedGoHandlers
  - TestApplicationNativeContributions
---

SKGo's aim is one application deployed to the web, macOS and iOS: the Kit UI in a WebView, services and domain types in Go, generated bindings for Svelte and Swift. Kit's own suggestion for a [mobile app](https://svelte.dev/docs/kit/project-types#Mobile-app) is an SPA inside a native shell. SKGo's shell keeps Go loads, actions and remote functions working inside the app.

## Status

| Piece                                               | Status                                                      |
| --------------------------------------------------- | ----------------------------------------------------------- |
| Web server binary                                   | Released                                                    |
| Swift query and command clients generated from Go   | Released                                                    |
| Embedded Go host and SwiftUI shell (`native-app`)   | Available to try; not a finished application target         |
| App lifecycle and WebView coordination              | Unfinished                                                  |
| Signing, packaging and distribution                 | Unfinished                                                  |
| A hosted-Go shell sharing one signed-in session     | Unfinished                                                  |

A successful `skgo native build`, an Xcode project or an archive shows that the code compiles. It does not show that an app installs, survives backgrounding or reaches a user, and none of those is claimed here.

## Bundled and hosted Go

Two arrangements are possible.

- **Bundled Go.** The app links your Go server into its own process as a c-archive and serves the Kit app on a loopback origin. A `WKWebView` shows that origin, and Swift code calls the same Go handlers over HTTP. This is what the `native-app` add-on builds.
- **Hosted Go.** The app contains no Go. The WebView and the Swift clients talk to your deployed server over HTTPS. The generated Swift clients work against any SKGo server today; a shell that shares one session between the WebView and the clients is not written.

## Swift bindings

`skgo generate` emits typed Swift for the remote functions you select. Name each as `<module>#<export>`, where the module is the generated `.remote.ts` path relative to the Vite root:

```sh
go tool skgo generate --web web --out internal/skgo \
  --locals-package example.com/myapp/internal/app \
  --swift-out native/Generated.swift \
  --swift-remote 'src/lib/data.remote.ts#read' \
  --swift-remote 'src/lib/data.remote.ts#save'
```

Only ordinary queries and commands can be selected; live queries, batch queries and forms cannot. The generated `NativeAPI` wraps the `RemoteClient` from the `SKGoNative` Swift package:

```swift
/// file: native/Sources/ApplicationView.swift
let client = RemoteClient(origin: "https://app.example.com")
let api = NativeAPI(core: client)
```

The client speaks Kit's remote-function protocol, the one the browser uses, so selection changes no authorization. It refuses HTTP redirects and never replays a command; a Kit redirect arrives as `RemoteError.redirect`. Retained queries cache their results and a command can request a query's refresh, the way `.updates()` does in Svelte. Call `resetSession()` after the user signs in or out.

Selecting a function changes its number handling for every caller, the browser included. Integers must fit JavaScript's safe range: an unsafe argument is a 400 and an unsafe result a 500, as other encoding failures are. Make exact identifiers strings before selecting an existing function.

## Adding a native app

The built-in `native-app` add-on adds the bundled-Go shell to an existing project:

```sh
go tool skgo add --set bundle-id=dev.example.myapp native-app
```

It writes `native/host/main.go`, `native/Sources/` and `native/skgo-native.json`. The host is a `package main` with a c-archive boundary: it builds your `NewHandler` through `nativehost.New` and exports `SKGoHostStart`, `SKGoHostRecover` and `SKGoHostStop` for Swift. This is the one place cgo appears; the web binary stays cgo-free.

```go
var host = nativehost.New(func(origin string) (http.Handler, error) {
	dist, err := fs.Sub(web.Build, "build")
	if err != nil {
		return nil, err
	}
	return app.NewHandler(dist, origin)
})
```

`native/Sources/` holds the SwiftUI shell: `Application.swift`, `ApplicationWebView.swift` around a `WKWebView`, `NativeHost.swift`, and `ApplicationView.swift`, which is yours to replace. `NativeHost` recovers the loopback listener when the app becomes active again, keeping Go's state and origin, and offers `onShutdown`, `onBackgroundChange` and `onAvailabilityChange` callbacks. WebView readiness and everything else specific to your app are yours.

`native/skgo-native.json` is a `nativeapp.Config`: the bundle id, `swiftRemotes` to generate `native/Generated.swift` from, deployment targets, extra Swift packages, sources, resources, `Info.plist` properties and named presets. [`skgo/nativeapp`](skgo-nativeapp) has the fields; [`skgo add`](/docs/cli/add) has the command.

## Building

```sh
go tool skgo native build --platform macos
go tool skgo native build --platform simulator
go tool skgo native build --platform iphone --preset beta
```

Builds run on macOS with Xcode, for arm64. The command prepares the frontend the way [`skgo build`](/docs/cli/build) does, generates the Swift when `swiftRemotes` is set, builds the Go host as a c-archive, runs Zig and XcodeGen through `mise` with the versions in SKGo's `native/mise.toml`, and compiles a Debug app with `xcodebuild`. It prints the products directory under `native/build/<platform>[-<preset>]` and does not install or launch anything. `--prepare-only` stops before Xcode, for tooling that owns that step; see [`skgo native build`](/docs/cli/native-build).

## Signing

Builds are unsigned unless `SKGO_DEVELOPMENT_TEAM` holds your Apple team id, in which case automatic signing runs with that team; `SKGO_DEVICE_ID` picks a destination. SKGo has no packaging, notarization or store step. Plan your own distribution, and test it on a clean device.
