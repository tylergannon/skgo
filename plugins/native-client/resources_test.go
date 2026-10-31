package nativeclient

import (
	"io/fs"
	"testing"
	"testing/fstest"
)

func TestBundleIncludesCoreAndNestedABIWithoutCopyingShellSources(t *testing.T) {
	shell := fstest.MapFS{"swift/Package.swift": {Data: []byte("package")}, "swift/Sources/CSKGo/bridge.c": {Data: []byte("bridge")}}
	bundle := Bundle(map[string]fs.FS{".": shell, "core": Core(), "swift/Sources/CSKGo/include": Headers()})
	if err := fstest.TestFS(bundle, "core/src/abi.zig", "core/src/cache.zig", "core/include/skgo.h", "swift/Package.swift", "swift/Sources/CSKGo/include/skgo.h"); err != nil {
		t.Fatal(err)
	}
}
