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
	// The app's formatter marks each file it formats, and waits for the test to
	// release it so Prepare's independence of it is observable.
	control := t.TempDir()
	release, broken := filepath.Join(control, "release"), filepath.Join(control, "broken")
	cacheWrite(t, release, "")
	vp := filepath.Join(cfg.Web, "node_modules/.bin/vp")
	cacheWrite(t, vp, "#!/bin/sh\nfor i in $(seq 100); do [ -e "+release+" ] && break; sleep 0.1; done\n[ -e "+release+" ] || exit 3\n[ -e "+broken+" ] && exit 4\nshift 3\nfor f; do printf '// formatted\\n' >> \"$f\"; done\n")
	if err := os.Chmod(vp, 0o755); err != nil {
		t.Fatal(err)
	}
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
	// Go selects an embedded file by name, so new bytes in one keep discovery.
	asset := filepath.Join(root, "wire/assets/one.txt")
	cacheWrite(t, asset, "asset-two")
	observeRetained(t, p, asset)
	discovered := p.Stats().Analysis.MetadataLoads
	if err := os.Remove(release); err != nil {
		t.Fatal(err)
	}
	// Prepare hands the compiler its inputs while the formatter waits; Commit
	// publishes what the formatter wrote.
	third := prepareRetained(t, p)
	cacheWrite(t, release, "")
	if err := third.Commit(nil); err != nil {
		t.Fatal(err)
	}
	if p.Stats().Generations != 2 {
		t.Fatalf("declaration plan not refreshed: %+v", p.Stats())
	}
	if got := p.Stats().Analysis.MetadataLoads - discovered; got != 0 {
		t.Fatalf("declaration and embedded-file edit ran Go discovery %d times, want 0", got)
	}
	projected, err := os.ReadFile(filepath.Join(cfg.Web, "src/data/types.ts"))
	if err != nil || !bytes.Contains(projected, []byte(`"receipt": string`)) || bytes.Contains(projected, []byte(`"text": string`)) || !bytes.HasSuffix(projected, []byte("// formatted\n")) {
		t.Fatalf("retained declaration: %s %v", projected, err)
	}
	// A failed format is not replayed: once the formatter works, the same
	// declarations generate and publish again.
	changed = strings.Replace(changed, `json:"receipt"`, `json:"answer"`, 1)
	cacheWrite(t, path, changed)
	observeRetained(t, p, path)
	cacheWrite(t, broken, "")
	if err := prepareRetained(t, p).Commit(nil); err == nil || !strings.Contains(err.Error(), "vp fmt") {
		t.Fatalf("broken formatter published: %v", err)
	}
	if err := os.Remove(broken); err != nil {
		t.Fatal(err)
	}
	if err := prepareRetained(t, p).Commit(nil); err != nil {
		t.Fatal(err)
	}
	if p.Stats().Generations != 4 {
		t.Fatalf("failed format replayed: %+v", p.Stats())
	}
	assertCacheFile(t, filepath.Join(cfg.Web, "src/data/types.ts"), `"answer": string`)

}
