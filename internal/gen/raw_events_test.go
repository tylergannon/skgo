package gen

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestRawInputsSurviveCompilerFilteringAndObsoleteAcknowledgement(t *testing.T) {
	root, cfg := retainedFixture(t)
	root, canonicalErr := filepath.EvalSymlinks(root)
	if canonicalErr != nil {
		t.Fatal(canonicalErr)
	}
	p, err := OpenProject(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	p.RawChanges()
	before := p.Revision()
	missing := filepath.Join(root, "app", "deleted.json")
	p.RecordRaw(true, missing)
	first := p.RawSelection()
	if !first.Full || !slices.Contains(first.Paths, missing) {
		t.Fatalf("selection=%+v", first)
	}
	p.RecordRaw(false, filepath.Join(root, "app", "second.yaml"))
	if p.AcknowledgeRaw(first.Epoch) {
		t.Fatal("obsolete acknowledgement discarded changes")
	}
	if p.Revision() != before || len(p.RawSelection().Paths) != 2 {
		t.Fatal("raw inputs coupled to compiler revision")
	}
	build := prepareRetained(t, p)
	defer build.Inputs.Close()
	build.RequireEventEpoch(p.RawSelection().Epoch)
	p.RecordRaw(false, missing)
	if err := build.Commit(func(*CompilerInputs) error { t.Fatal("stale application admitted"); return nil }); !errors.Is(err, ErrObsolete) {
		t.Fatalf("commit=%v", err)
	}
	current := p.RawSelection()
	if !p.AcknowledgeRaw(current.Epoch) || len(p.RawSelection().Paths) != 0 || p.RawSelection().Full {
		t.Fatal("current pass not acknowledged")
	}
}
func TestWatcherDeliversNonGoDeletionAndDirectoryMembership(t *testing.T) {
	root, cfg := retainedFixture(t)
	root, canonicalErr := filepath.EvalSymlinks(root)
	if canonicalErr != nil {
		t.Fatal(canonicalErr)
	}
	p, err := OpenProject(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	p.RawChanges()
	if _, err = p.Watch(); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(root, "app", "producer.json")
	write := func() {
		if err := os.WriteFile(input, []byte("literal input"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	wait := func(path string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if slices.Contains(p.RawSelection().Paths, path) {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("missing raw %s: %+v", path, p.RawSelection())
	}
	write()
	wait(input)
	p.AcknowledgeRaw(p.RawSelection().Epoch)
	if err := os.Remove(input); err != nil {
		t.Fatal(err)
	}
	wait(input)
	if err := p.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestWatcherWithoutGeneratorDoesNotRetainRawHistory(t *testing.T) {
	root, cfg := retainedFixture(t)
	p, err := OpenProject(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if _, err = p.Watch(); err != nil {
		t.Fatal(err)
	}
	before := p.Revision()
	for n := 0; n < 64; n++ {
		path := filepath.Join(root, "app", fmt.Sprintf("temporary-%d.json", n))
		if err := os.WriteFile(path, []byte("temporary input"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	barrier := filepath.Join(root, "app", "barrier.go")
	if err := os.WriteFile(barrier, []byte("package app\nconst rawOptOutBarrier=17\n"), 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for p.Revision() == before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if p.Revision() == before {
		t.Fatal("watcher did not acquire barrier")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.rawPaths) != 0 || p.rawChanges != nil || p.rawEpoch != 0 {
		t.Fatalf("unselected raw history retained: paths=%d epoch=%d", len(p.rawPaths), p.rawEpoch)
	}
}

func TestRawSelectionExcludesOnlyPrivateMirroredLinks(t *testing.T) {
	_, cfg := retainedFixture(t)
	p, err := OpenProject(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	p.RawChanges()
	p.RecordRaw(false, filepath.Join(cfg.Out, linkRootName, "route", "skgo_gen.go"))
	if p.RawSelection().Epoch != 0 || len(p.RawSelection().Paths) != 0 {
		t.Fatal("private mirror invalidated application generation")
	}
	output := filepath.Join(cfg.Web, "src", "domain_gen.go")
	p.RecordRaw(false, output)
	selection := p.RawSelection()
	canonical, err := sourcePath(output)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Epoch != 1 || !slices.Contains(selection.Paths, canonical) {
		t.Fatalf("application-owned generated output hidden: %+v", selection)
	}
}

func TestRawSelectionExcludesOnlyKnownNodeCompileCaches(t *testing.T) {
	root, cfg := retainedFixture(t)
	configured := filepath.Join(root, "app", "explicit-node-cache")
	t.Setenv("NODE_COMPILE_CACHE", configured)
	p, err := OpenProject(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	p.RawChanges()
	for _, path := range []string{filepath.Join(root, "wire", ".cache", "node-compile", "entry"), filepath.Join(root, "app", ".cache", "node-compile", "entry"), filepath.Join(configured, "entry")} {
		p.RecordRaw(false, path)
	}
	if p.RawSelection().Epoch != 0 {
		t.Fatalf("tool cache invalidated generation: %+v", p.RawSelection())
	}
	// Dot directories and other cache-adjacent data can be real generator inputs.
	input := filepath.Join(root, "app", ".cache", "producer-input.json")
	p.RecordRaw(false, input)
	canonical, err := sourcePath(input)
	if err != nil {
		t.Fatal(err)
	}
	if selection := p.RawSelection(); selection.Epoch != 1 || !slices.Contains(selection.Paths, canonical) {
		t.Fatalf("real non-Go input hidden: %+v", selection)
	}
}
