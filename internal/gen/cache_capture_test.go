package gen

import (
	"os"
	"path/filepath"
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
