// Package skgo holds the generated implementation for the example application.
package skgo

//go:generate go tool skgo generate --web ../../web --locals-package github.com/tylergannon/skgo/example/internal/app --hook-package github.com/tylergannon/skgo/example/internal/serverhooks --swift-out ../../native/ios/Generated.swift --swift-remote src/lib/auth.remote.ts#whoami --swift-remote src/lib/auth.remote.ts#signIn
