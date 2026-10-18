package gen

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/mod/modfile"
)

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

func TestRetainedWorkspaceSelectionEvents(t *testing.T) {
	for _, mode := range []string{"explicit", "auto"} {
		t.Run(mode, func(t *testing.T) { testRetainedWorkspaceSelectionEvents(t, mode) })
	}
}

func testRetainedWorkspaceSelectionEvents(t *testing.T, mode string) {
	root, cfg := retainedFixture(t)
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(root, "go.work")
	cacheWrite(t, work, "go 1.27.2\nuse ./app\n")
	requested := work
	if mode == "auto" {
		requested = "auto"
	}
	t.Setenv("GOWORK", requested)
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
	const revised = "go 1.27.2\nuse (\n ./app\n ./wire\n)\n"
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
		assertRetainedFile(t, b, filepath.Join(cfg.Web, "src/data/data.remote.go"), retainedRemote)
	}
	verify()
	const nearerSource = "go 1.27.2\nuse (\n .\n ../wire\n)\n"
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
