package pluginbuild

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSourceCopyCarriesSDKInputsWithoutWorkspaceBuildProducts(t *testing.T) {
	source, dest := t.TempDir(), t.TempDir()
	inputs := []string{"go.mod", "sdk/core/build.zig", "sdk/core/src/root.zig", "sdk/swift/Package.swift", "sdk/swift/Sources/Client.swift", "scaffold/nested/go.mod"}
	products := []string{"sdk/core/zig-out/lib/native.a", "sdk/core/zig-pkg/dependency/source.zig", "sdk/core/.zig-cache/object.o", "sdk/swift/.build/libNative.a", "sdk/swift/.swiftpm/workspace-state.json", "example/internal/skgo/_links/generated.go"}
	for _, name := range append(inputs, products...) {
		path := filepath.Join(source, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := copySource(source, dest); err != nil {
		t.Fatal(err)
	}
	for _, name := range inputs {
		bytes, err := os.ReadFile(filepath.Join(dest, filepath.FromSlash(name)))
		if err != nil || string(bytes) != name {
			t.Errorf("source %s not preserved: %q %v", name, bytes, err)
		}
	}
	for _, name := range products {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(name))); !os.IsNotExist(err) {
			t.Errorf("workspace product %s reached source build: %v", name, err)
		}
	}
}
