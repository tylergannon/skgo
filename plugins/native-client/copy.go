package nativeclient

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func SyncSDK(source fs.FS, dest string) error {
	// Synchronize SDK source without changing identical compiler inputs or
	// deleting compiler-owned caches. Remove source deleted by an SDK upgrade.
	files := map[string]bool{}
	err := fs.WalkDir(source, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := path
		if d.IsDir() {
			if d.Name() == ".build" || d.Name() == ".zig-cache" || d.Name() == "zig-out" || d.Name() == "zig-pkg" || d.Name() == "node_modules" || d.Name() == "bin" || d.Name() == "obj" {
				return filepath.SkipDir
			}
			target := filepath.Join(dest, rel)
			if info, err := os.Lstat(target); err == nil && info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("native SDK output contains a symlink: %s", target)
			}
			return os.MkdirAll(target, 0755)
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("native SDK contains a symlink: %s", rel)
		}
		b, err := fs.ReadFile(source, path)
		if err != nil {
			return err
		}
		files[rel] = true
		return writeSDKFile(filepath.Join(dest, rel), b, 0644)
	})
	if err != nil {
		return err
	}
	return filepath.WalkDir(dest, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".build", ".zig-cache", "zig-out", "zig-pkg", "node_modules", ".swiftpm", "bin", "obj":
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(dest, path)
		if err != nil {
			return err
		}
		if !files[rel] {
			return os.Remove(path)
		}
		return nil
	})
}

func writeSDKFile(path string, data []byte, mode fs.FileMode) error {
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("native output contains a symlink: %s", path)
	}
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) {
		return nil
	}
	return os.WriteFile(path, data, mode)
}

// CopySDK copies bundled native sources while retaining compiler caches and
// unchanged file timestamps. The destination is disposable SDK output.
func CopySDK(source fs.FS, dest, owner string) error {
	// Only a directory created for SDK output may be synchronized destructively.
	for part := filepath.Clean(dest); ; part = filepath.Dir(part) {
		if info, err := os.Lstat(part); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("SDK destination contains a symlink: %s", part)
		}
		if filepath.Dir(part) == part {
			break
		}
	}
	marker := "." + owner + "-sdk"
	entries, err := os.ReadDir(dest)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if len(entries) != 0 {
		info, err := os.Lstat(filepath.Join(dest, marker))
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("SDK destination is not disposable %s output: %s", owner, dest)
		}
		data, err := os.ReadFile(filepath.Join(dest, marker))
		if err != nil || string(data) != owner+" SDK output\n" {
			return fmt.Errorf("SDK destination ownership marker is invalid")
		}
	}
	if err := SyncSDK(source, dest); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dest, marker), []byte(owner+" SDK output\n"), 0644)
}
