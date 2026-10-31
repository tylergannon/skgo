// Package nativeclient supplies the shared Zig protocol/cache and C ABI sources
// bundled into independently installed shell plugins. It is not a host plugin.
package nativeclient

import (
	"embed"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"
)

//go:embed sdk/core/build.zig sdk/core/build.zig.zon sdk/core/README.md sdk/core/src sdk/core/include
var resources embed.FS

func Core() fs.FS    { f, _ := fs.Sub(resources, "sdk/core"); return f }
func Headers() fs.FS { f, _ := fs.Sub(resources, "sdk/core/include"); return f }

// Bundle mounts immutable source trees without copying shared sources into each
// plugin's repository. More specific mounts override their parent trees.
func Bundle(mounts map[string]fs.FS) fs.FS { return bundle(mounts) }

type bundle map[string]fs.FS

func (b bundle) source(name string) (fs.FS, string) {
	best := ""
	var source fs.FS
	for prefix, f := range b {
		if prefix == "." || name == prefix || strings.HasPrefix(name, prefix+"/") {
			if source == nil || len(prefix) > len(best) {
				best, source = prefix, f
			}
		}
	}
	if best != "." {
		name = strings.TrimPrefix(strings.TrimPrefix(name, best), "/")
	}
	if name == "" {
		name = "."
	}
	return source, name
}
func (b bundle) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	if f, sub := b.source(name); f != nil {
		file, err := f.Open(sub)
		if err == nil {
			info, err := file.Stat()
			if err != nil {
				file.Close()
				return nil, err
			}
			if !info.IsDir() {
				return file, nil
			}
			file.Close()
			return &bundleDir{bundle: b, name: name, info: info}, nil
		}
		if !isAncestor(b, name) {
			return nil, err
		}
	}
	if isAncestor(b, name) {
		return &bundleDir{bundle: b, name: name, info: dirInfo(path.Base(name))}, nil
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}
func isAncestor(b bundle, name string) bool {
	for prefix := range b {
		if name == "." || strings.HasPrefix(prefix, name+"/") {
			return true
		}
	}
	return false
}
func (b bundle) ReadDir(name string) ([]fs.DirEntry, error) {
	entries := map[string]fs.DirEntry{}
	if f, sub := b.source(name); f != nil {
		items, err := fs.ReadDir(f, sub)
		if err != nil && !isAncestor(b, name) {
			return nil, err
		}
		for _, e := range items {
			entries[e.Name()] = e
		}
	}
	for prefix := range b {
		child := prefix
		if name != "." {
			child = strings.TrimPrefix(prefix, name+"/")
			if child == prefix {
				continue
			}
		}
		if child == "." || child == name {
			continue
		}
		child, _, _ = strings.Cut(child, "/")
		entries[child] = fs.FileInfoToDirEntry(dirInfo(child))
	}
	result := make([]fs.DirEntry, 0, len(entries))
	for _, e := range entries {
		result = append(result, e)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name() < result[j].Name() })
	return result, nil
}

type dirInfo string

func (d dirInfo) Name() string     { return string(d) }
func (dirInfo) Size() int64        { return 0 }
func (dirInfo) Mode() fs.FileMode  { return fs.ModeDir | 0555 }
func (dirInfo) ModTime() time.Time { return time.Time{} }
func (dirInfo) IsDir() bool        { return true }
func (dirInfo) Sys() any           { return nil }

type bundleDir struct {
	bundle bundle
	name   string
	info   fs.FileInfo
	offset int
}

func (d *bundleDir) Stat() (fs.FileInfo, error) { return d.info, nil }
func (*bundleDir) Close() error                 { return nil }
func (*bundleDir) Read([]byte) (int, error)     { return 0, fs.ErrInvalid }
func (d *bundleDir) ReadDir(n int) ([]fs.DirEntry, error) {
	entries, err := d.bundle.ReadDir(d.name)
	if err != nil {
		return nil, err
	}
	if d.offset >= len(entries) && n > 0 {
		return nil, io.EOF
	}
	end := len(entries)
	if n > 0 && d.offset+n < end {
		end = d.offset + n
	}
	items := entries[d.offset:end]
	d.offset = end
	return items, nil
}
