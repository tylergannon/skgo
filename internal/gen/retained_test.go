package gen

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const retainedRemote = `package data
import("context";"example.com/wire";"github.com/tylergannon/skgo")
type Reply struct { Text string ` + "`json:\"text\"`" + ` }
func Answer(context.Context)(Reply,error){return Reply{Text:wire.Label()+":body-one"},nil}
var _=skgo.Query(Answer)
`
const retainedWire = `package wire
import("embed";"io/fs";"fmt")
//go:embed assets
var assets embed.FS
func Label() string {data,_:=assets.ReadFile("assets/one.txt");names,_:=fs.Glob(assets,"assets/*");return fmt.Sprintf("%s:%d",data,len(names))}
`

func retainedFixture(t *testing.T) (string, Config) {
	t.Helper()
	root, cfg := foreignFixture(t, retainedRemote, map[string]string{"wire/wire.go": retainedWire, "wire/assets/one.txt": "asset-one"})
	return root, cfg
}
func prepareRetained(t *testing.T, p *Project) *ProjectBuild {
	t.Helper()
	b, err := p.Prepare(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Cancel(); b.Inputs.Close() })
	return b
}
func assertRetainedFile(t *testing.T, b *ProjectBuild, authored, want string) {
	t.Helper()
	private := b.Inputs.PrivatePath(authored)
	if got := string(b.Inputs.Files()[private]); got != want {
		t.Fatalf("revision %d captured %s = %q, want %q", b.Revision, authored, got, want)
	}
	// Assert the file the compiler consumes, not just the retained handle.
	data, err := os.ReadFile(b.Inputs.CompiledPath(authored))
	if err != nil || string(data) != want {
		t.Fatalf("revision %d compiler input %s = %q, %v; want %q", b.Revision, authored, data, err, want)
	}
}
func observeRetained(t *testing.T, p *Project, path string) {
	t.Helper()
	if _, changed, err := p.Observe(context.Background(), path); err != nil || !changed {
		t.Fatalf("observe %s: changed=%t, %v", path, changed, err)
	}
}
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

func TestRetainedSharedGenerationWaiterCancellation(t *testing.T) {
	t.Parallel()
	f := newGenerationFuture()
	entered, release := make(chan struct{}, 1), make(chan struct{})
	var mu sync.Mutex
	computations := 0
	compute := func(ctx context.Context) (generationPlan, error) {
		mu.Lock()
		computations++
		mu.Unlock()
		select {
		case entered <- struct{}{}:
		default:
		}
		select {
		case <-ctx.Done():
			return generationPlan{}, ctx.Err()
		case <-release:
			return generationPlan{Files: map[string]string{"fixture": "shared"}}, nil
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	second := make(chan error, 1)
	go func() { _, err := f.get(ctx, compute); first <- err }()
	<-entered
	go func() {
		value, err := f.get(context.Background(), compute)
		if err == nil && value.Files["fixture"] != "shared" {
			err = errors.New("wrong shared plan")
		}
		second <- err
	}()
	// Wait on the owner state, not time, before canceling one of two waiters.
	deadline := time.Now().Add(time.Second)
	for {
		f.mu.Lock()
		ready := f.run != nil && f.run.waiters == 2
		f.mu.Unlock()
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("second generation waiter did not join")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	close(release)
	if err := <-second; err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if computations != 1 {
		t.Fatalf("equivalent generation computed %d times", computations)
	}
}
