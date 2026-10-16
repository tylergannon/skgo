package gen

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceCaptureReadsOverlappingInputsOnceAndRefreshesExactBytes(t *testing.T) {
	root, cfg := foreignFixture(t, cacheRemote, nil)
	module := filepath.Join(root, "app")
	route := filepath.Join(cfg.Web, "src/routes/assets/embed.go")
	asset := filepath.Join(filepath.Dir(route), "data/one.txt")
	cacheWrite(t, route, "package assets\nimport \"embed\"\n//go:embed data/* data/one.txt\nvar Files embed.FS\n")
	cacheWrite(t, asset, "one")
	native := filepath.Join(cfg.Web, "src/data/fixture.h")
	cacheWrite(t, native, "native bytes")
	source := cacheSources{Modules: []cacheModule{{Dir: module}}, Controls: []string{filepath.Join(module, "go.mod")}}
	capture := func() (map[string][]byte, string, map[string]int) {
		t.Helper()
		reads := map[string]int{}
		files, key, err := source.captureWithRead(cfg, func(path string) ([]byte, error) { reads[path]++; return os.ReadFile(path) })
		if err != nil {
			t.Fatal(err)
		}
		for path, n := range reads {
			if n != 1 {
				t.Fatalf("%s read %d times in one capture", path, n)
			}
		}
		return files, key, reads
	}
	first, key, reads := capture()
	for path, want := range map[string]string{route: "package assets\nimport \"embed\"\n//go:embed data/* data/one.txt\nvar Files embed.FS\n", asset: "one", native: "native bytes"} {
		if string(first[path]) != want || reads[path] != 1 {
			t.Fatalf("capture %s=%q, reads=%d", path, first[path], reads[path])
		}
	}
	info, err := os.Stat(asset)
	if err != nil {
		t.Fatal(err)
	}
	cacheWrite(t, asset, "two")
	if err := os.Chtimes(asset, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	second, next, reads := capture()
	if string(second[asset]) != "two" || next == key || reads[asset] != 1 {
		t.Fatal("same-size, same-mtime edit was not captured")
	}
	added := filepath.Join(filepath.Dir(asset), "two.txt")
	cacheWrite(t, added, "added")
	third, next2, _ := capture()
	if string(third[added]) != "added" || next2 == next {
		t.Fatal("new embed match was not captured")
	}
	if err := os.Remove(added); err != nil {
		t.Fatal(err)
	}
	fourth, next3, _ := capture()
	if _, ok := fourth[added]; ok || next3 != next {
		t.Fatal("removed embed match was not removed from capture")
	}
}

// WalkDir buffers a directory's entries before visiting them. Mutating a later
// entry while reading an earlier file deterministically exercises disappearance
// between enumeration and traversal, without sleeps or a watcher scheduling bet.
func TestSourceCaptureDirectoryDisappearsDuringWalk(t *testing.T) {
	for _, walk := range []string{"module", "embed", "frontend"} {
		for _, operation := range []string{"delete", "rename"} {
			t.Run(walk+"/"+operation, func(t *testing.T) {
				root := t.TempDir()
				cfg := Config{Web: filepath.Join(root, "web"), Out: filepath.Join(root, "internal/skgo")}
				cacheWrite(t, filepath.Join(root, "go.mod"), "module example.com/capture\ngo 1.27\n")
				cacheWrite(t, filepath.Join(cfg.Web, "src/+page.svelte"), "<p>fixture</p>")
				dir, name := root, "a.go"
				if walk == "embed" {
					cacheWrite(t, filepath.Join(root, "embed.go"), "package fixture\nimport \"embed\"\n//go:embed assets\nvar Files embed.FS\n")
					dir, name = filepath.Join(root, "assets"), "a.txt"
				} else if walk == "frontend" {
					dir, name = filepath.Join(cfg.Web, "src"), "a.txt"
				}
				trigger := filepath.Join(dir, name)
				victim := filepath.Join(dir, "zdir")
				moved := filepath.Join(dir, "bdir")
				cacheWrite(t, trigger, "package fixture // before\n")
				cacheWrite(t, filepath.Join(victim, "data.go"), "package fixture // retained\n")
				source := cacheSources{Modules: []cacheModule{{Dir: root}}}
				mutated := false
				files, _, err := source.captureWithRead(cfg, func(path string) ([]byte, error) {
					data, err := os.ReadFile(path)
					if path == trigger && !mutated {
						mutated = true
						if operation == "rename" {
							if err := os.Rename(victim, moved); err != nil {
								t.Fatal(err)
							}
						} else if err := os.RemoveAll(victim); err != nil {
							t.Fatal(err)
						}
						cacheWrite(t, trigger, "package fixture // after\n")
					}
					return data, err
				})
				if err != nil {
					t.Fatal(err)
				}
				if !mutated || string(files[trigger]) != "package fixture // after\n" {
					t.Fatalf("partial pre-mutation capture survived: mutated=%t, trigger=%q", mutated, files[trigger])
				}
				for path := range files {
					if strings.HasPrefix(path, victim+string(filepath.Separator)) {
						t.Fatalf("deleted subtree retained in capture: %s", path)
					}
				}
				if operation == "rename" && string(files[filepath.Join(moved, "data.go")]) != "package fixture // retained\n" {
					t.Fatal("renamed source missing from replacement capture")
				}
				if string(files[filepath.Join(cfg.Web, "src/+page.svelte")]) != "<p>fixture</p>" {
					t.Fatal("surviving frontend fixture missing")
				}
			})
		}
	}
}

func TestSourceCapturePreservesErrors(t *testing.T) {
	root := t.TempDir()
	cfg := Config{Web: filepath.Join(root, "web"), Out: filepath.Join(root, "internal/skgo")}
	cacheWrite(t, filepath.Join(root, "go.mod"), "module example.com/capture\ngo 1.27\n")
	cacheWrite(t, filepath.Join(cfg.Web, "src/+page.svelte"), "<p>fixture</p>")
	source := cacheSources{Modules: []cacheModule{{Dir: root}}}
	reads := 0
	files, _, err := source.captureWithRead(cfg, func(string) ([]byte, error) {
		reads++
		return nil, fs.ErrPermission
	})
	if !errors.Is(err, fs.ErrPermission) || files != nil || reads != 1 {
		t.Fatalf("permission error was retried or suppressed: files=%v reads=%d err=%v", files, reads, err)
	}
	if err := os.RemoveAll(filepath.Join(cfg.Web, "src")); err != nil {
		t.Fatal(err)
	}
	files, _, err = source.capture(cfg)
	if !errors.Is(err, fs.ErrNotExist) || files != nil {
		t.Fatalf("permanently missing frontend root accepted: files=%v err=%v", files, err)
	}
}
