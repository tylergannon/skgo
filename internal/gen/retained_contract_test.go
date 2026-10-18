package gen

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Directory transitions must reach generation and compilation through actual
// filesystem events, without direct Observe calls.
func TestRetainedWatcherDirectoryCompilerInputs(t *testing.T) {
	t.Parallel()
	root, cfg := retainedFixture(t)
	root, _ = filepath.EvalSymlinks(root)
	cfg.Web, _ = sourcePath(cfg.Web)
	cfg.Out, _ = sourcePath(cfg.Out)
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
				t.Fatal("required directory event was not acquired")
			}
		}
	}
	build := func() *ProjectBuild {
		t.Helper()
		b := prepareRetained(t, p)
		if err := b.Commit(nil); err != nil {
			t.Fatal(err)
		}
		return b
	}
	build()
	pending := filepath.Join(root, "pending")
	extra := strings.Replace(retainedRemote, "package data", "package extra", 1)
	cacheWrite(t, filepath.Join(pending, "answer.remote.go"), extra)
	added := filepath.Join(cfg.Web, "src/extra")
	if err := os.Rename(pending, added); err != nil {
		t.Fatal(err)
	}
	wait(func(files map[string][]byte) bool {
		return strings.Contains(string(files[filepath.Join(added, "answer.remote.go")]), "package extra")
	})
	assertRetainedFile(t, build(), filepath.Join(added, "answer.remote.go"), extra)
	generated, err := os.ReadFile(filepath.Join(added, "answer.remote.ts"))
	if err != nil || !strings.Contains(string(generated), "Answer") {
		t.Fatalf("created producer fixture missing: %s %v", generated, err)
	}
	moved := filepath.Join(cfg.Web, "src/moved")
	if err := os.Rename(added, moved); err != nil {
		t.Fatal(err)
	}
	wait(func(files map[string][]byte) bool {
		_, old := files[filepath.Join(added, "answer.remote.go")]
		return !old && strings.Contains(string(files[filepath.Join(moved, "answer.remote.go")]), "package extra")
	})
	renamed := build()
	assertRetainedFile(t, renamed, filepath.Join(moved, "answer.remote.go"), extra)
	if _, exists := renamed.Inputs.Files()[renamed.Inputs.PrivatePath(filepath.Join(added, "answer.remote.go"))]; exists {
		t.Fatal("compiler inputs retained the old directory after rename")
	}
	generated, err = os.ReadFile(filepath.Join(moved, "answer.remote.ts"))
	if err != nil || !strings.Contains(string(generated), "Answer") {
		t.Fatalf("renamed producer fixture missing: %s %v", generated, err)
	}
	if err := os.RemoveAll(moved); err != nil {
		t.Fatal(err)
	}
	wait(func(files map[string][]byte) bool {
		_, old := files[filepath.Join(moved, "answer.remote.go")]
		return !old
	})
	removed := build()
	if _, exists := removed.Inputs.Files()[removed.Inputs.PrivatePath(filepath.Join(moved, "answer.remote.go"))]; exists {
		t.Fatal("compiler inputs retained the removed directory")
	}
	bindings, err := os.ReadFile(filepath.Join(cfg.Out, generatedGoFile))
	if err != nil || !strings.Contains(string(bindings), `"src/data/data.remote.ts"`) || strings.Contains(string(bindings), "src/moved/answer.remote.ts") {
		t.Fatalf("removed producer remains in generated registry: %s %v", bindings, err)
	}
	// New embed matches must arrive through the replacement module's watcher.
	asset := filepath.Join(root, "wire/assets/two.txt")
	cacheWrite(t, asset, "second")
	wait(func(files map[string][]byte) bool { return string(files[asset]) == "second" })
	assertRetainedFile(t, build(), asset, "second")
	// Check the actual event loop settles after all generated writes, then an
	// equal-content external write must leave the source revision unchanged.
	time.Sleep(100 * time.Millisecond)
	revision := p.Revision()
	cacheWrite(t, asset, "second")
	time.Sleep(100 * time.Millisecond)
	if p.Revision() != revision {
		t.Fatalf("equal-content/generated noise advanced source: %d -> %d", revision, p.Revision())
	}
}

func TestRetainedCompilerDiagnosticsAndRepair(t *testing.T) {
	t.Parallel()
	_, cfg := retainedFixture(t)
	p, err := OpenProject(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	first := prepareRetained(t, p)
	if err := first.Commit(nil); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cfg.Web, "src/data/data.remote.go")
	path, _ = sourcePath(path)
	observeRetainedWrite(t, p, path, strings.Replace(retainedRemote, "wire.Label()", "missingFixtureSymbol()", 1))
	broken := prepareRetained(t, p)
	diagnostic := fmt.Errorf("%s:4:45: undefined: missingFixtureSymbol", broken.Inputs.CompiledPath(path))
	if !broken.Diagnose(diagnostic) {
		t.Fatal("current compiler diagnostic suppressed")
	}
	diag := p.Diagnostic()
	if diag == nil || diag.Revision != 2 || !strings.Contains(diag.Message, "web/src/data/data.remote.go:4:") || !strings.Contains(diag.Message, "undefined: missingFixtureSymbol") {
		t.Fatalf("compiler diagnostic lacks authored fixture location/revision: %+v", diag)
	}
	observeRetainedWrite(t, p, path, strings.Replace(retainedRemote, "body-one", "repaired", 1))
	repaired := prepareRetained(t, p)
	if err := repaired.Commit(nil); err != nil {
		t.Fatal(err)
	}
	if broken.Diagnose(errors.New("obsolete compiler error")) {
		t.Fatal("obsolete compiler diagnostic published")
	}
	if p.Diagnostic() != nil {
		t.Fatal("obsolete error displaced healthy revision")
	}
	assertRetainedFile(t, repaired, path, strings.Replace(retainedRemote, "body-one", "repaired", 1))
}

func TestRetainedPublicationReleaseOrders(t *testing.T) {
	t.Parallel()
	for _, currentFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "old-first", true: "current-first"}[currentFirst], func(t *testing.T) {
			_, cfg := retainedFixture(t)
			p, err := OpenProject(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			old := prepareRetained(t, p)
			path := filepath.Join(cfg.Web, "src/data/data.remote.go")
			source := strings.Replace(retainedRemote, `json:"text"`, `json:"current"`, 1)
			observeRetainedWrite(t, p, path, strings.Replace(source, "body-one", "body-current", 1))
			current := prepareRetained(t, p)
			commitOld := func() {
				t.Helper()
				if err := old.Commit(func(*CompilerInputs) error { t.Fatal("obsolete compiler inputs admitted"); return nil }); !errors.Is(err, ErrObsolete) {
					t.Fatalf("obsolete outputs admitted: %v", err)
				}
			}
			if !currentFirst {
				assertRetainedFile(t, old, path, retainedRemote)
				commitOld()
			}
			if err := current.Commit(nil); err != nil {
				t.Fatal(err)
			}
			if currentFirst {
				assertRetainedFile(t, current, path, strings.Replace(source, "body-one", "body-current", 1))
				commitOld()
				assertRetainedFile(t, old, path, retainedRemote)
			} else {
				assertRetainedFile(t, current, path, strings.Replace(source, "body-one", "body-current", 1))
			}
			raw, err := os.ReadFile(filepath.Join(cfg.Web, "src/data/types.ts"))
			if err != nil || !strings.Contains(string(raw), `"current": string`) {
				t.Fatalf("current fixture output lost: %s %v", raw, err)
			}
		})
	}
}
