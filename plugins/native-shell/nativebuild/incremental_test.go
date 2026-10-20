package nativebuild

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSDKSyncPreservesCompilerCachesAndUnchangedInputs(t *testing.T) {
	source, dest := t.TempDir(), t.TempDir()
	write := func(root, name, content string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write(source, "core/build.zig", "source")
	write(source, "swift/Package.swift", "package")
	if err := copySDK(os.DirFS(source), dest); err != nil {
		t.Fatal(err)
	}
	write(dest, "core/.zig-cache/receipt", "cached zig")
	write(dest, "swift/.build/receipt", "cached swift")
	write(dest, "swift/removed.swift", "stale source")
	input := filepath.Join(dest, "swift/Package.swift")
	old := time.Unix(1000, 0)
	if err := os.Chtimes(input, old, old); err != nil {
		t.Fatal(err)
	}
	if err := copySDK(os.DirFS(source), dest); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(input)
	if !info.ModTime().Equal(old) {
		t.Fatal("identical Swift input was rewritten")
	}
	for _, name := range []string{"core/.zig-cache/receipt", "swift/.build/receipt"} {
		if _, err := os.Stat(filepath.Join(dest, name)); err != nil {
			t.Fatalf("compiler cache removed: %s", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dest, "swift/removed.swift")); !os.IsNotExist(err) {
		t.Fatal("stale SDK source retained")
	}
	write(source, "swift/Package.swift", "updated")
	if err := copySDK(os.DirFS(source), dest); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(input)
	if string(data) != "updated" {
		t.Fatal("changed SDK source was reused")
	}
}

func TestSDKExportRefusesAuthoredDestination(t *testing.T) {
	source := t.TempDir()
	dest := t.TempDir()
	dest, _ = filepath.EvalSymlinks(dest)
	if err := os.WriteFile(filepath.Join(source, "Package.swift"), []byte("SDK"), 0644); err != nil {
		t.Fatal(err)
	}
	authored := filepath.Join(dest, "application.go")
	if err := os.WriteFile(authored, []byte("application"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := CopySDK(os.DirFS(source), dest); err == nil {
		t.Fatal("accepted authored SDK destination")
	}
	if data, err := os.ReadFile(authored); err != nil || string(data) != "application" {
		t.Fatalf("authored output changed: %q %v", data, err)
	}
	outputRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(outputRoot, "SDK")
	if err := CopySDK(os.DirFS(source), output); err != nil {
		t.Fatal(err)
	}
	if err := CopySDK(os.DirFS(source), output); err != nil {
		t.Fatalf("repeat export: %v", err)
	}
}
