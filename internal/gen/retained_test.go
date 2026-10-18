package gen

import (
	"context"
	"errors"
	"os"
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
