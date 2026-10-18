---
title: native-app
description: Add an embedded Go host and a SwiftUI shell for macOS and iOS.
division: cli
section: Add-ons
order: 3010
---

`native-app` is the add-on built into `skgo`. It wraps your application in a native one: a Go host that serves the application on a loopback origin inside the app, and a SwiftUI shell that shows it in a WebView on macOS and iOS. Native hosting, lifecycle, WebView coordination and distribution are still being built; [Native targets](/docs/skgo/native-targets) says what works today.

## Usage

```sh
go tool skgo add --set bundle-id=dev.example.notes native-app
```

Then build it with [`skgo native build`](/docs/cli/native-build):

```sh
go tool skgo native build --platform macos
```

## What you get

```tree
native/
├── .gitignore                 # ignores build/ and Generated.swift
├── Bridge.h
├── host/main.go               # cgo entry points that start, recover and stop the host
├── skgo-native.json           # native configuration
└── Sources/
    ├── Application.swift
    ├── ApplicationView.swift  # yours to edit
    ├── ApplicationWebView.swift
    └── NativeHost.swift
```

`native/host/main.go` builds the handler with the same `NewHandler` from your `server.go` that the standalone binary uses, over the embedded frontend build, and hands it to [`nativehost`](/docs/reference/skgo/nativehost). Three exported C functions start, recover and stop that host; the Swift shell calls them.

`ApplicationView.swift` holds the app's state, its content view and the macOS settings view. It is the one shell file the add-on treats as yours: once the configuration exists, running the add-on again leaves it and `skgo-native.json` alone.

Running the add-on again with the same `bundle-id` writes only what is missing. A different `bundle-id`, or any other shell file whose bytes differ from what the add-on would write, is a conflict, and nothing is written.

## Options

### bundle-id

The application's bundle identifier, such as `dev.example.notes`, without any preset suffix. Required. It must be reverse-DNS form with at least two dot-separated components.

## Configuration

`native/skgo-native.json` starts as:

```json
{
  "version": 1,
  "bundleId": "dev.example.notes"
}
```

The builder reads it with unknown fields rejected, and every path in it must stay inside the application. You can add:

| Field | Purpose |
| --- | --- |
| `swiftRemotes` | remote functions to generate Swift clients for, as `module#export`; the same selection as [`skgo generate --swift-remote`](/docs/cli/generate) |
| `deploymentTargets` | minimum OS versions keyed by platform: `macos`, `iphone` or `simulator` |
| `packages` | local Swift packages, each `{ "name", "path", "products" }`; the name `SKGoNative` is reserved |
| `sources`, `resources` | extra Swift sources and app resources |
| `info`, `iosInfo`, `macosInfo` | Info.plist values for both platforms, iOS only, or macOS only |
| `presets` | named build variants with `goTags`, `bundleSuffix`, `info` and `resources` |

A preset that omits `resources` inherits the application's; an empty list clears them. Select one with `skgo native build --preset NAME`, and the output goes to `native/build/<platform>-<preset>` instead of `native/build/<platform>`. The Go type behind the file is [`nativeapp.Config`](/docs/reference/skgo/nativeapp#Config).

Builds are unsigned unless `SKGO_DEVELOPMENT_TEAM` is set; `SKGO_DEVICE_ID` then chooses a signing destination. A successful build is not evidence that the app installs or runs on a device.
