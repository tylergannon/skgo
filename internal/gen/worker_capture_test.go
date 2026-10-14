package gen

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

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
