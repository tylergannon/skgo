package gen

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/mod/modfile"
)

// TestMain explicitly disables workspace selection. Unrelated temporary Go
// build siblings must not be included just to watch ineffective go.work files.
func TestRetainedWatcherStartsDuringTemporaryBuildChurn(t *testing.T) {
	root, cfg := retainedFixture(t)
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	p, err := OpenProject(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if p.sources.Environment["GOWORK"] != "off" {
		t.Fatal("fixture must disable workspace selection")
	}
	for _, control := range p.sources.Controls {
		if filepath.Base(control) == "go.work" || filepath.Base(control) == "go.work.sum" {
			t.Fatalf("disabled workspace control selected: %s", control)
		}
	}
	const directories = 128
	paths := make([]string, directories)
	for n := range paths {
		paths[n] = filepath.Join(root, fmt.Sprintf("go-build%d", n))
		if err := os.Mkdir(paths[n], 0700); err != nil {
			t.Fatal(err)
		}
	}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for {
				for n := worker; n < len(paths); n += 8 {
					select {
					case <-stop:
						return
					default:
					}
					_ = os.Remove(paths[n])
					_ = os.Mkdir(paths[n], 0700)
				}
			}
		}(worker)
	}
	defer func() { close(stop); wg.Wait() }()
	errs, err := p.Watch()
	if err != nil {
		t.Fatalf("selected scope lost during unrelated build churn: %v", err)
	}
	watched := p.watch.fs.WatchList()
	if slices.Contains(watched, root) {
		t.Fatal("disabled workspace ancestor is watched")
	}
	if !slices.Contains(watched, filepath.Join(root, "app")) {
		t.Fatal("actual module root is not watched")
	}
	// This ancestor file cannot affect an explicit GOWORK=off build.
	work := filepath.Join(root, "go.work")
	cacheWrite(t, work, "go 1.27.1\nuse ./app\n")
	remote, err := sourcePath(filepath.Join(cfg.Web, "src/data/data.remote.go"))
	if err != nil {
		t.Fatal(err)
	}
	cacheWrite(t, remote, strings.Replace(retainedRemote, "body-one", "during-churn", 1))
	waitRetainedFiles(t, p, errs, func(files map[string][]byte) bool { return strings.Contains(string(files[remote]), "during-churn") })
	b := prepareRetained(t, p)
	if err := b.Commit(nil); err != nil {
		t.Fatal(err)
	}
	runRetained(t, b, "asset-one:1:during-churn")
	p.mu.Lock()
	_, captured := p.current.files[work]
	p.mu.Unlock()
	if captured {
		t.Fatal("disabled workspace file entered compiler capture")
	}
}

func waitRetainedFiles(t *testing.T, p *Project, errs <-chan error, predicate func(map[string][]byte) bool) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		p.mu.Lock()
		ready := predicate(p.current.files)
		p.mu.Unlock()
		if ready {
			return
		}
		select {
		case err := <-errs:
			if err != nil {
				t.Fatal(err)
			}
		case <-p.Changes():
		case <-timer.C:
			t.Fatal("selected file event was not acquired")
		}
	}
}

// Workspace tests run in isolated processes because TestMain and other package
// fixtures intentionally keep GOWORK=off. Each process uses stock Go discovery.
func TestRetainedWorkspaceSelectionEvents(t *testing.T) {
	mode := os.Getenv("SKGO_RETAINED_WORKSPACE_TEST")
	if mode == "" {
		for _, mode := range []string{"explicit", "auto"} {
			t.Run(mode, func(t *testing.T) {
				cmd := exec.Command(os.Args[0], "-test.run=^TestRetainedWorkspaceSelectionEvents$", "-test.count=1", "-test.v")
				cmd.Env = append(os.Environ(), "SKGO_RETAINED_WORKSPACE_TEST="+mode, "SKGO_CACHE_TEST_CONFIG=workspace-selection-test")
				out, err := cmd.CombinedOutput()
				if err != nil || strings.Contains(string(out), "SKIP") || !strings.Contains(string(out), "--- PASS: TestRetainedWorkspaceSelectionEvents") {
					t.Fatalf("real workspace events: %v\n%s", err, out)
				}
			})
		}
		return
	}
	root, cfg := retainedFixture(t)
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(root, "go.work")
	cacheWrite(t, work, "go 1.27.1\nuse ./app\n")
	requested := work
	if mode == "auto" {
		requested = "auto"
	}
	if err := os.Setenv("GOWORK", requested); err != nil {
		t.Fatal(err)
	}
	p, err := OpenProject(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	expected := []string{work, work + ".sum"}
	nearer := filepath.Join(root, "app/go.work")
	if mode == "auto" {
		expected = append(expected, nearer)
	}
	var actual []string
	for _, path := range p.sources.Controls {
		if filepath.Base(path) == "go.work" || filepath.Base(path) == "go.work.sum" {
			actual = append(actual, path)
		}
	}
	slices.Sort(actual)
	slices.Sort(expected)
	if !slices.Equal(actual, expected) {
		t.Fatalf("effective workspace controls: %v, want %v", actual, expected)
	}
	errs, err := p.Watch()
	if err != nil {
		t.Fatal(err)
	}
	const revised = "go 1.27.1\nuse (\n ./app\n ./wire\n)\n"
	cacheWrite(t, work, revised)
	waitRetainedFiles(t, p, errs, func(files map[string][]byte) bool { return string(files[work]) == revised })
	verify := func() {
		t.Helper()
		b := prepareRetained(t, p)
		if err := b.Commit(nil); err != nil {
			t.Fatal(err)
		}
		privateWork := ""
		for _, env := range b.Inputs.Env {
			if value, ok := strings.CutPrefix(env, "GOWORK="); ok {
				privateWork = value
			}
		}
		raw, err := os.ReadFile(privateWork)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := modfile.ParseWork(privateWork, raw, nil)
		if err != nil || len(parsed.Use) != 2 {
			t.Fatalf("compiler workspace lost supplied two modules: %s %v", raw, err)
		}
		runRetained(t, b, "asset-one:1:body-one")
	}
	verify()
	const nearerSource = "go 1.27.1\nuse (\n .\n ../wire\n)\n"
	revision := p.Revision()
	cacheWrite(t, nearer, nearerSource)
	if mode == "auto" {
		waitRetainedFiles(t, p, errs, func(files map[string][]byte) bool {
			return string(files[nearer]) == nearerSource && p.sources.Environment["GOWORK"] == nearer
		})
		verify()
	} else {
		time.Sleep(100 * time.Millisecond)
		if p.Revision() != revision {
			t.Fatal("nearer workspace changed an explicitly selected build")
		}
	}
}

func TestRetainedAutoWorkspaceDanglingAncestor(t *testing.T) {
	if os.Getenv("SKGO_RETAINED_AUTO_ANCESTOR_TEST") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestRetainedAutoWorkspaceDanglingAncestor$", "-test.count=1", "-test.v")
		cmd.Env = append(os.Environ(), "SKGO_RETAINED_AUTO_ANCESTOR_TEST=1", "SKGO_CACHE_TEST_CONFIG=auto-ancestor-test")
		out, err := cmd.CombinedOutput()
		if err != nil || strings.Contains(string(out), "SKIP") || !strings.Contains(string(out), "--- PASS: TestRetainedAutoWorkspaceDanglingAncestor") {
			t.Fatalf("actual automatic workspace watcher: %v\n%s", err, out)
		}
		return
	}
	scope := t.TempDir()
	scope, err := filepath.EvalSymlinks(scope)
	if err != nil {
		t.Fatal(err)
	}
	root, cfg := foreignFixtureIn(t, filepath.Join(scope, "project"), retainedRemote, map[string]string{"wire/wire.go": retainedWire, "wire/assets/one.txt": "asset-one", "app/probe/main.go": retainedProbe})
	if err := os.Setenv("GOWORK", "auto"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "absent-target"), filepath.Join(root, "dangling-sibling")); err != nil {
		t.Fatal(err)
	}
	p, err := OpenProject(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if got := p.sources.Environment["GOWORK"]; got != "" {
		t.Fatalf("fixture unexpectedly selected %s", got)
	}
	errs, err := p.Watch()
	if err != nil {
		t.Fatalf("actual Project cannot watch required ancestor with dangling sibling: %v", err)
	}
	// Go searches beyond the module's parent. This new workspace must actually
	// become the selected compiler workspace, with both supplied modules.
	work := filepath.Join(scope, "go.work")
	const source = "go 1.27.1\nuse (\n ./project/app\n ./project/wire\n)\n"
	cacheWrite(t, work, source)
	waitRetainedFiles(t, p, errs, func(files map[string][]byte) bool {
		return string(files[work]) == source && p.sources.Environment["GOWORK"] == work
	})
	b := prepareRetained(t, p)
	if err := b.Commit(nil); err != nil {
		t.Fatal(err)
	}
	runRetained(t, b, "asset-one:1:body-one")
	var privateWork string
	for _, env := range b.Inputs.Env {
		if path, ok := strings.CutPrefix(env, "GOWORK="); ok {
			privateWork = path
		}
	}
	raw, err := os.ReadFile(privateWork)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := modfile.ParseWork(privateWork, raw, nil)
	if err != nil || len(parsed.Use) != 2 {
		t.Fatalf("farther compiler workspace lost supplied modules: %s %v", raw, err)
	}
}

func TestRetainedAtomicThenPreservedMtimeCompilerInputs(t *testing.T) {
	root, cfg := retainedFixture(t)
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	p, err := OpenProject(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	errs, err := p.Watch()
	if err != nil {
		t.Fatal(err)
	}
	asset := filepath.Join(root, "wire/assets/one.txt")
	info, err := os.Stat(asset)
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 32; n++ {
		atomic := fmt.Sprintf("atomic-%d", n)
		next := fmt.Sprintf("same-time-%d", n)
		cacheWrite(t, asset+".tmp", atomic)
		if err := os.Rename(asset+".tmp", asset); err != nil {
			t.Fatal(err)
		}
		waitRetainedFiles(t, p, errs, func(files map[string][]byte) bool { return string(files[asset]) == atomic })
		cacheWrite(t, asset, next)
		if err := os.Chtimes(asset, info.ModTime(), info.ModTime()); err != nil {
			t.Fatal(err)
		}
		waitRetainedFiles(t, p, errs, func(files map[string][]byte) bool { return string(files[asset]) == next })
	}
	b := prepareRetained(t, p)
	if err := b.Commit(nil); err != nil {
		t.Fatal(err)
	}
	runRetained(t, b, "same-time-31:1:body-one")
}
