package gen

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Watching owns acquisition, not compilation. Assert the bytes and membership
// the real event loop supplies, without regenerating after each file event.
func TestRetainedWatcherDirectoryCompilerInputs(t *testing.T) {
	t.Parallel()
	root, cfg := retainedFixture(t)
	root, _ = filepath.EvalSymlinks(root)
	cfg.Web, _ = sourcePath(cfg.Web)
	p, err := OpenProject(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	errs, err := p.Watch()
	if err != nil {
		t.Fatal(err)
	}
	wait := func(predicate func(map[string][]byte) bool) {
		t.Helper()
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		tick := time.NewTicker(time.Millisecond)
		defer tick.Stop()
		for {
			p.mu.Lock()
			ok := predicate(p.current.files)
			p.mu.Unlock()
			if ok {
				return
			}
			select {
			case e := <-errs:
				if e != nil {
					t.Fatal(e)
				}
			case <-tick.C:
			case <-timer.C:
				t.Fatal("required event was not acquired")
			}
		}
	}
	pending := filepath.Join(root, "pending")
	extra := "package extra\nconst Receipt = 541\n"
	cacheWrite(t, filepath.Join(pending, "answer.go"), extra)
	added := filepath.Join(cfg.Web, "src/extra")
	if err = os.Rename(pending, added); err != nil {
		t.Fatal(err)
	}
	wait(func(files map[string][]byte) bool { return string(files[filepath.Join(added, "answer.go")]) == extra })
	moved := filepath.Join(cfg.Web, "src/moved")
	if err = os.Rename(added, moved); err != nil {
		t.Fatal(err)
	}
	wait(func(files map[string][]byte) bool {
		_, old := files[filepath.Join(added, "answer.go")]
		return !old && string(files[filepath.Join(moved, "answer.go")]) == extra
	})
	if err = os.RemoveAll(moved); err != nil {
		t.Fatal(err)
	}
	wait(func(files map[string][]byte) bool {
		_, old := files[filepath.Join(moved, "answer.go")]
		_, original := files[filepath.Join(cfg.Web, "src/data/data.remote.go")]
		return !old && original
	})
	asset := filepath.Join(root, "wire/assets/two.txt")
	cacheWrite(t, asset, "new embed receipt 729")
	wait(func(files map[string][]byte) bool { return string(files[asset]) == "new embed receipt 729" })
}

func TestRetainedAdmissionUsesCurrentRevision(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cfg := Config{Web: filepath.Join(root, "web"), Out: filepath.Join(root, "generated"), Logf: func(string, ...any) {}}
	cacheWrite(t, filepath.Join(root, "go.mod"), "module admission.test\n\ngo 1.27\n")
	cacheWrite(t, filepath.Join(cfg.Web, "src/authored.txt"), "receipt 729")
	old, current := &projectSnapshot{revision: 1}, &projectSnapshot{revision: 2}
	p := &Project{cfg: cfg, current: current, rawEpoch: 7}
	stale := &ProjectBuild{Revision: 1, project: p, snapshot: old}
	calls := 0
	admit := func(*CompilerInputs) error { calls++; return nil }
	if err := stale.Commit(admit); !errors.Is(err, ErrObsolete) || calls != 0 {
		t.Fatalf("stale admission: %v calls=%d", err, calls)
	}
	if stale.Diagnose(errors.New("obsolete error")) {
		t.Fatal("obsolete diagnostic admitted")
	}
	stub := filepath.Join(cfg.Web, "src/data.remote.ts")
	inputs := &CompilerInputs{view: &generationSourceView{}, links: &routeLinks{}}
	build := &ProjectBuild{Revision: 2, project: p, snapshot: current, Inputs: inputs, plan: generationPlan{Files: map[string]string{stub: tsHeader + "export const receipt = 541;\n"}}}
	if !build.Diagnose(errors.New("fixture current source:7")) || p.Diagnostic().Revision != 2 || !strings.Contains(p.Diagnostic().Message, "fixture current source:7") {
		t.Fatalf("current diagnostic: %+v", p.Diagnostic())
	}
	build.RequireEventEpoch(6)
	if err := build.Commit(admit); !errors.Is(err, ErrObsolete) || calls != 0 {
		t.Fatalf("raw event admitted: %v calls=%d", err, calls)
	}
	build.RequireEventEpoch(7)
	if err := build.Commit(admit); err != nil || calls != 1 {
		t.Fatalf("current admission: %v calls=%d", err, calls)
	}
	assertCacheFile(t, stub, "receipt = 541")
	if p.Diagnostic() != nil {
		t.Fatal("successful publication kept diagnostic")
	}
}
