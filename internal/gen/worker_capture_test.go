package gen

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWorkerCaptureReusesOnlyCurrentFileVersions(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "source.go")
	write := func(path, data string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(file, "first")
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	retains := !captureChangeTime(info).IsZero()
	var previous map[string]capturedFile
	reads := 0
	capture := func(path, want string, hook func()) {
		t.Helper()
		r := captureReader{previous: previous, next: map[string]capturedFile{}, readFile: func(path string) ([]byte, error) {
			reads++
			data, err := os.ReadFile(path)
			if hook != nil {
				hook()
			}
			return data, err
		}}
		data, err := r.read(path)
		if err != nil || string(data) != want {
			t.Fatalf("capture=%q error=%v; want %q", data, err, want)
		}
		previous = r.next
	}
	capture(file, "first", nil)
	capture(file, "first", nil)
	if retains && reads != 1 {
		t.Fatalf("unchanged input read %d times", reads)
	}
	write(file, "other")
	if err := os.Chtimes(file, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	capture(file, "other", nil)
	if err := os.Rename(file, filepath.Join(root, "old")); err != nil {
		t.Fatal(err)
	}
	write(file, "third")
	if err := os.Chtimes(file, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	capture(file, "third", nil)
	write(file, "start")
	capture(file, "start", func() { write(file, "after") })
	if _, ok := previous[file]; ok {
		t.Fatal("read racing a write was retained")
	}
	capture(file, "after", nil)

	// A capture containing a different selection releases the previous bytes.
	other := filepath.Join(root, "selected.go")
	write(other, "selected")
	capture(other, "selected", nil)
	if _, ok := previous[file]; ok {
		t.Fatal("unselected file retained")
	}
	if err := os.Remove(other); err != nil {
		t.Fatal(err)
	}
	r := captureReader{previous: previous, next: map[string]capturedFile{}, readFile: os.ReadFile}
	if _, err := r.read(other); !os.IsNotExist(err) {
		t.Fatalf("deleted input error=%v", err)
	}
	if len(r.next) != 0 {
		t.Fatal("deleted input retained")
	}
}

func TestWorkerSourceCaptureRetainsBytesWithoutLosingMembership(t *testing.T) {
	root := t.TempDir()
	cfg := Config{Web: filepath.Join(root, "web"), Out: filepath.Join(root, "internal", "skgo")}
	cacheWrite(t, filepath.Join(root, "go.mod"), "module capture.test\n\ngo 1.27.2\n")
	if err := os.MkdirAll(cfg.Out, 0700); err != nil {
		t.Fatal(err)
	}
	cfg.worker = new(Worker)
	defer cfg.worker.Close()
	source := cacheSources{Modules: []cacheModule{{Dir: root}}}
	file := filepath.Join(cfg.Web, "src", "capture.txt")
	cacheWrite(t, file, "first")
	_, key, err := source.capture(cfg)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	cacheWrite(t, file, "other")
	if err := os.Chtimes(file, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	files, edited, err := source.capture(cfg)
	if err != nil || string(files[file]) != "other" || edited == key {
		t.Fatalf("edit capture=%q key changed=%v error=%v", files[file], edited != key, err)
	}
	added := filepath.Join(cfg.Web, "src", "added.txt")
	cacheWrite(t, added, "added")
	files, next, err := source.capture(cfg)
	if err != nil || string(files[added]) != "added" || next == edited {
		t.Fatal("new source membership missed")
	}
	if err := os.Remove(added); err != nil {
		t.Fatal(err)
	}
	files, next, err = source.capture(cfg)
	if err != nil || files[added] != nil || next != edited {
		t.Fatal("removed source membership missed")
	}
	if _, ok := cfg.worker.capturedFiles[added]; ok {
		t.Fatal("removed source retained by worker")
	}
	if err := cfg.worker.Close(); err != nil {
		t.Fatal(err)
	}
	if len(cfg.worker.capturedFiles) != 0 {
		t.Fatal("closed worker retained source")
	}
}

func BenchmarkSourceCapture(b *testing.B) {
	root := b.TempDir()
	cfg := Config{Web: filepath.Join(root, "web"), Out: filepath.Join(root, "internal", "skgo")}
	for _, dir := range []string{filepath.Join(root, "pkg"), filepath.Join(cfg.Web, "src"), cfg.Out} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			b.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module capture.test\n\ngo 1.27.2\n"), 0600); err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 768; i++ {
		data := fmt.Sprintf("package pkg\n// %s\nvar Value%d = %d\n", strings.Repeat("x", 8192), i, i)
		if err := os.WriteFile(filepath.Join(root, "pkg", fmt.Sprintf("file%d.go", i)), []byte(data), 0600); err != nil {
			b.Fatal(err)
		}
	}
	source := cacheSources{Modules: []cacheModule{{Dir: root}}}
	for _, mode := range []string{"ordinary", "worker-cold", "worker-warm"} {
		b.Run(mode, func(b *testing.B) {
			config := cfg
			if mode != "ordinary" {
				config.worker = new(Worker)
				defer config.worker.Close()
			}
			if _, _, err := source.capture(config); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			for b.Loop() {
				if mode == "worker-cold" {
					config.worker.capturedFiles = nil
				}
				if _, _, err := source.capture(config); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestWorkerCapturePreservedMtimeEditsMatchAuthoritativeGeneration(t *testing.T) {
	root, cfg := foreignFixture(t, cacheRemote, nil)
	cfg.CacheDir = "off"
	path := filepath.Join(root, "wire/wire.go")
	cacheWrite(t, path, "package wire\ntype Thing struct { Name string `json:\"name\"` }\n")
	var w Worker
	defer w.Close()
	if err := w.Run(cfg); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	for i, tag := range []string{"code", "kind"} {
		target := path
		if i == 1 {
			target = path + ".replacement"
		}
		cacheWrite(t, target, "package wire\ntype Thing struct { Name string `json:\""+tag+"\"` }\n")
		if err := os.Chtimes(target, info.ModTime(), info.ModTime()); err != nil {
			t.Fatal(err)
		}
		if target != path {
			if err := os.Rename(target, path); err != nil {
				t.Fatal(err)
			}
		}
		if err := w.Run(cfg); err != nil {
			t.Fatal(err)
		}
		assertCacheFile(t, filepath.Join(cfg.Web, "src/lib/skgo/example.com/wire/types.ts"), "\""+tag+"\": string")
		got := generatedTree(t, filepath.Join(root, "app"))
		if err := Run(cfg); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, generatedTree(t, filepath.Join(root, "app"))) {
			t.Fatal("retained capture differs from authoritative generation")
		}
	}
}
