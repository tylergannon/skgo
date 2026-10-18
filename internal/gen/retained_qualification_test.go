//go:build skgo_qualification

package gen

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRetainedProjectSelectedCompilerInputs(t *testing.T) {
	t.Parallel()
	root, cfg := retainedFixture(t)
	root, _ = filepath.EvalSymlinks(root)
	p, err := OpenProject(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	first := prepareRetained(t, p)
	path := filepath.Join(cfg.Web, "src/data/data.remote.go")
	oldInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(retainedRemote, "body-one", "body-two", 1)
	cacheWrite(t, path, changed)
	if err := os.Chtimes(path, oldInfo.ModTime(), oldInfo.ModTime()); err != nil {
		t.Fatal(err)
	}
	before := p.Stats()
	observeRetained(t, p, path)
	after := p.Stats()
	if after.FileReads-before.FileReads != 1 || after.FileParses-before.FileParses != 1 {
		t.Fatalf("routine acquisition not one changed file: before=%+v after=%+v", before, after)
	}
	second := prepareRetained(t, p)
	if p.Stats().Generations != 1 || p.Stats().GenerationHits != 1 {
		t.Fatalf("body-only generation not reused: %+v", p.Stats())
	}
	if !bytes.Contains(second.Inputs.Files()[second.Inputs.PrivatePath(path)], []byte("body-two")) {
		t.Fatal("compiler did not receive supplied new body")
	}
	assertRetainedFile(t, first, path, retainedRemote)
	assertRetainedFile(t, second, path, changed)
	assertRetainedFile(t, first, filepath.Join(root, "wire/assets/one.txt"), "asset-one")
	if err := first.Commit(func(*CompilerInputs) error { t.Fatal("obsolete compiler input admitted"); return nil }); !errors.Is(err, ErrObsolete) {
		t.Fatalf("obsolete commit: %v", err)
	}
	if err := second.Commit(nil); err != nil {
		t.Fatal(err)
	}
	assertCacheFile(t, filepath.Join(cfg.Web, "src/data/types.ts"), `"text": string`)
	// A consumed declaration edit must invalidate the retained plan, while a
	// function-body edit above deliberately reused it.
	changed = strings.Replace(changed, `json:"text"`, `json:"receipt"`, 1)
	cacheWrite(t, path, changed)
	observeRetained(t, p, path)
	third := prepareRetained(t, p)
	if err := third.Commit(nil); err != nil {
		t.Fatal(err)
	}
	if p.Stats().Generations != 2 {
		t.Fatalf("declaration plan not refreshed: %+v", p.Stats())
	}
	projected, err := os.ReadFile(filepath.Join(cfg.Web, "src/data/types.ts"))
	if err != nil || !bytes.Contains(projected, []byte(`"receipt": string`)) || bytes.Contains(projected, []byte(`"text": string`)) {
		t.Fatalf("retained declaration: %s %v", projected, err)
	}

}
