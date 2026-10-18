package gen

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Directory transitions must reach generation and compilation through actual
// filesystem events, without direct Observe calls.
func TestRetainedWatcherDirectoryCompilerInputs(t *testing.T) {
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
	build := func(want string) {
		t.Helper()
		b := prepareRetained(t, p)
		if err := b.Commit(nil); err != nil {
			t.Fatal(err)
		}
		runRetained(t, b, want)
	}
	build("asset-one:1:body-one")
	pending := filepath.Join(root, "pending")
	cacheWrite(t, filepath.Join(pending, "answer.remote.go"), strings.Replace(retainedRemote, "package data", "package extra", 1))
	added := filepath.Join(cfg.Web, "src/extra")
	if err := os.Rename(pending, added); err != nil {
		t.Fatal(err)
	}
	wait(func(files map[string][]byte) bool {
		return strings.Contains(string(files[filepath.Join(added, "answer.remote.go")]), "package extra")
	})
	build("asset-one:1:body-one")
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
	build("asset-one:1:body-one")
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
	build("asset-one:1:body-one")
	// New embed matches must arrive through the replacement module's watcher.
	asset := filepath.Join(root, "wire/assets/two.txt")
	cacheWrite(t, asset, "second")
	wait(func(files map[string][]byte) bool { return string(files[asset]) == "second" })
	build("asset-one:2:body-one")
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
	cmd := exec.Command("go", "run", "./probe")
	cmd.Dir = broken.Inputs.Dir
	cmd.Env = broken.Inputs.Env
	raw, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("compiler accepted invalid fixture")
	}
	if !broken.Diagnose(errors.New(string(raw))) {
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
	runRetained(t, repaired, "asset-one:1:repaired")
}

func TestRetainedPublicationReleaseOrders(t *testing.T) {
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
				runRetained(t, old, "asset-one:1:body-one")
				commitOld()
			}
			if err := current.Commit(nil); err != nil {
				t.Fatal(err)
			}
			if currentFirst {
				runRetained(t, current, "asset-one:1:body-current")
				commitOld()
				runRetained(t, old, "asset-one:1:body-one")
			} else {
				runRetained(t, current, "asset-one:1:body-current")
			}
			raw, err := os.ReadFile(filepath.Join(cfg.Web, "src/data/types.ts"))
			if err != nil || !strings.Contains(string(raw), `"current": string`) {
				t.Fatalf("current fixture output lost: %s %v", raw, err)
			}
		})
	}
}

func TestRetainedRepeatedEditsStayBounded(t *testing.T) {
	_, cfg := retainedFixture(t)
	p, err := OpenProject(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	path := filepath.Join(cfg.Web, "src/data/data.remote.go")
	for n := 0; n < 24; n++ {
		source := strings.Replace(retainedRemote, `json:"text"`, fmt.Sprintf(`json:"fixture%d"`, n), 1)
		observeRetainedWrite(t, p, path, source)
		b := prepareRetained(t, p)
		if err := b.Commit(nil); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(filepath.Join(cfg.Web, "src/data/types.ts"))
		if err != nil || !strings.Contains(string(raw), fmt.Sprintf(`"fixture%d": string`, n)) {
			t.Fatalf("current fixture %d not generated: %s %v", n, raw, err)
		}
		if n == 23 {
			runRetained(t, b, "asset-one:1:body-one")
		}
		b.Inputs.Close()
		stats := p.Stats()
		// The fixture supplies seven Go files: app locals, probe, remote,
		// wire, wire/other/wire, the skgo stand-in, and its qualified pins.
		if stats.Analyses != 7 || stats.Plans != 1 || stats.Pending != 0 {
			t.Fatalf("retained state after fixture %d: %+v", n, stats)
		}
	}
}
