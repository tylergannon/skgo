// Package sdk bundles the C# client and the shared native protocol library.
package sdk

import (
	"embed"
	"io/fs"

	nativeclient "github.com/tylergannon/skgo/plugins/native-client"
)

//go:embed csharp/README.md csharp/SKGoNative/*.cs csharp/SKGoNative/*.csproj csharp/RemoteCLI/*.cs csharp/RemoteCLI/*.csproj mise.toml
var resources embed.FS

func Resources() fs.FS {
	return nativeclient.Bundle(map[string]fs.FS{".": resources, "core": nativeclient.Core()})
}
