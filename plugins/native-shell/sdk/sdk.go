// Package sdk contains only Apple's Swift resources. Protocol and ABI sources
// come from the shared native-client dependency in every exported bundle.
package sdk

import (
	"embed"
	"io/fs"

	nativeclient "github.com/tylergannon/skgo/plugins/native-client"
)

//go:embed swift/Package.swift swift/Sources swift/Tests swift/README.md mise.toml mise.lock
var resources embed.FS

func Resources() fs.FS {
	return nativeclient.Bundle(map[string]fs.FS{
		".": resources, "core": nativeclient.Core(),
		"swift/Sources/CSKGo/include": nativeclient.Headers(),
	})
}
