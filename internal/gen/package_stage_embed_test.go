package gen

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestStagedEmbedsLeaveUnmatchedTreesAlone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	const source = "package fixture\nimport \"embed\"\n//go:embed all:assets\nvar Files embed.FS\n"
	files := map[string]string{"go.mod": "module stage.test\n\ngo 1.27\n", "assets.go": source, "assets/public.txt": "public receipt 541", "assets/.hidden.txt": "hidden receipt 729", "node_modules/dependency.txt": "unrelated"}
	for path, contents := range files {
		cacheWrite(t, filepath.Join(dir, path), contents)
	}
	cfg := &packages.Config{Dir: dir, Env: append(os.Environ(), "GOWORK=off"), Overlay: map[string][]byte{filepath.Join(dir, "generated.go"): []byte("package fixture\nconst Receipt=613\n")}}
	cleanup, err := stagePackageOverlay(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	for _, name := range []string{"assets/public.txt", "assets/.hidden.txt"} {
		path := filepath.Join(cfg.Dir, name)
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("selected embed not materialized: %s %v", path, err)
		}
		assertCacheFile(t, path, files[name])
	}
	info, err := os.Lstat(filepath.Join(cfg.Dir, "node_modules"))
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("unmatched tree materialized: %v %v", info, err)
	}
	assertCacheFile(t, filepath.Join(cfg.Dir, "generated.go"), "Receipt=613")
	if _, err = os.Stat(filepath.Join(dir, "generated.go")); !os.IsNotExist(err) {
		t.Fatalf("overlay published into authored tree: %v", err)
	}
}
