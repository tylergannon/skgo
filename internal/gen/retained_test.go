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

	"golang.org/x/tools/go/packages"
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
	if err := first.Commit(nil); err != nil {
		t.Fatal(err)
	}
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
	// Old leases must retain captured Go, replacement-module and embed bytes
	// after authored files change.
	cacheWrite(t, filepath.Join(root, "wire/assets/one.txt"), "asset-two")
	observeRetained(t, p, filepath.Join(root, "wire/assets/one.txt"))
	third := prepareRetained(t, p)
	assertRetainedFile(t, first, path, retainedRemote)
	assertRetainedFile(t, first, filepath.Join(root, "wire/assets/one.txt"), "asset-one")
	assertRetainedFile(t, second, path, changed)
	assertRetainedFile(t, second, filepath.Join(root, "wire/assets/one.txt"), "asset-one")
	assertRetainedFile(t, third, path, changed)
	assertRetainedFile(t, third, filepath.Join(root, "wire/assets/one.txt"), "asset-two")
	if err := second.Commit(func(*CompilerInputs) error { t.Fatal("obsolete compiler input admitted"); return nil }); !errors.Is(err, ErrObsolete) {
		t.Fatalf("old commit: %v", err)
	}
	if second.Diagnose(errors.New("obsolete error")) {
		t.Fatal("obsolete diagnostic published")
	}
	if err := third.Commit(nil); err != nil {
		t.Fatal(err)
	}
	rev := p.Revision()
	if _, yes, err := p.Observe(context.Background(), path); err != nil || yes || p.Revision() != rev {
		t.Fatalf("equal bytes triggered work: %v %t", err, yes)
	}
	// A local dependency body and a new embed match change actual compilation,
	// while leaving generation's consumed declaration keys unchanged.
	cacheWrite(t, filepath.Join(root, "wire/wire.go"), strings.Replace(retainedWire, "%s:%d", "dependency-%s:%d", 1))
	observeRetained(t, p, filepath.Join(root, "wire/wire.go"))
	cacheWrite(t, filepath.Join(root, "wire/assets/two.txt"), "new-match")
	observeRetained(t, p, filepath.Join(root, "wire/assets"))
	fourth := prepareRetained(t, p)
	assertRetainedFile(t, fourth, filepath.Join(root, "wire/wire.go"), strings.Replace(retainedWire, "%s:%d", "dependency-%s:%d", 1))
	assertRetainedFile(t, fourth, filepath.Join(root, "wire/assets/two.txt"), "new-match")
	if p.Stats().Generations != 1 {
		t.Fatalf("compiler-only edits regenerated declarations: %+v", p.Stats())
	}
	// Atomic replacement of an existing source is content-based too.
	cacheWrite(t, path+".tmp", strings.Replace(changed, "body-two", "body-end", 1))
	if err := os.Rename(path+".tmp", path); err != nil {
		t.Fatal(err)
	}
	observeRetained(t, p, path)
	fifth := prepareRetained(t, p)
	assertRetainedFile(t, fifth, path, strings.Replace(changed, "body-two", "body-end", 1))
}

func TestRetainedProjectDeclarationsDirectoriesAndRepair(t *testing.T) {
	t.Parallel()
	root, cfg := retainedFixture(t)
	root, _ = filepath.EvalSymlinks(root)
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
	// Tags, struct declarations, constants and markers all remain consumed inputs.
	source := strings.Replace(retainedRemote, `json:"text"`, `json:"label"`, 1)
	for _, edit := range []struct{ source, want string }{
		{source, `"label": string`},
		{strings.Replace(source, "type Reply struct {", "const Label = \"named\"\ntype Reply struct { Count int;", 1), `"Count": number`},
	} {
		cacheWrite(t, path, edit.source)
		observeRetained(t, p, path)
		b := prepareRetained(t, p)
		if err := b.Commit(nil); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(cfg.Web, "src/data/types.ts"))
		if err != nil || !strings.Contains(string(data), edit.want) {
			t.Fatalf("projection lacks fixture %q: %s, %v", edit.want, data, err)
		}
	}
	cacheWrite(t, path, "package data\nfunc invalid(")
	observeRetained(t, p, path)
	_, err = p.Prepare(context.Background())
	var re *RevisionError
	if !errors.As(err, &re) || !strings.Contains(err.Error(), "data.remote.go:") || !strings.Contains(err.Error(), "revision") {
		t.Fatalf("authored revision diagnostic missing: %v", err)
	}
	if !p.Diagnose(err) || p.Diagnostic().Revision != p.Revision() {
		t.Fatalf("current diagnostic missing: %v", p.Diagnostic())
	}
	cacheWrite(t, path, retainedRemote)
	observeRetained(t, p, path)
	repaired := prepareRetained(t, p)
	if err := repaired.Commit(nil); err != nil {
		t.Fatal(err)
	}
	if p.Diagnostic() != nil {
		t.Fatal("repair left diagnostic")
	}
	// A populated directory moved into source must acquire all its members.
	outside := filepath.Join(root, "pending")
	cacheWrite(t, filepath.Join(outside, "answer.remote.go"), strings.ReplaceAll(retainedRemote, "package data", "package extra"))
	added := filepath.Join(cfg.Web, "src/extra")
	if err := os.Rename(outside, added); err != nil {
		t.Fatal(err)
	}
	observeRetained(t, p, added)
	next := prepareRetained(t, p)
	if err := next.Commit(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(added, "answer.remote.ts")); err != nil {
		t.Fatal("new producer missing", err)
	}
	renamed := filepath.Join(cfg.Web, "src/moved")
	if err := os.Rename(added, renamed); err != nil {
		t.Fatal(err)
	}
	if _, _, err := p.Observe(context.Background(), added, renamed); err != nil {
		t.Fatal(err)
	}
	next = prepareRetained(t, p)
	if err := next.Commit(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(renamed, "answer.remote.ts")); err != nil {
		t.Fatal("renamed producer missing", err)
	}
	if err := os.Remove(filepath.Join(renamed, "answer.remote.go")); err != nil {
		t.Fatal(err)
	}
	observeRetained(t, p, renamed)
	next = prepareRetained(t, p)
	if err := next.Commit(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(renamed, "answer.remote.ts")); !os.IsNotExist(err) {
		t.Fatal("removed producer retained owned artifact", err)
	}
	// The unchanged dependency's parse result survives all these transitions.
	wire := filepath.Join(root, "wire/wire.go")
	wire, _ = sourcePath(wire)
	a := p.current.parsed[wire]
	observeRetainedWrite(t, p, path, strings.Replace(retainedRemote, "body-one", "body-last", 1))
	if p.current.parsed[wire] != a {
		t.Fatal("unchanged analysis discarded")
	}
	if stats := p.Stats(); stats.Plans > 1 || stats.Pending != 0 {
		t.Fatalf("retained state unbounded: %+v", stats)
	}
}
func observeRetainedWrite(t *testing.T, p *Project, path, source string) {
	cacheWrite(t, path, source)
	observeRetained(t, p, path)
}

func TestRetainedProjectStaleGeneration(t *testing.T) {
	t.Parallel()
	_, cfg := retainedFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	cfg.OnPackageLoad = func(*packages.Config, []string) { once.Do(func() { close(entered); <-release }) }
	p, err := OpenProject(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	done := make(chan error, 1)
	go func() {
		b, err := p.Prepare(context.Background())
		if b != nil {
			b.Cancel()
		}
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("generation never entered")
	}
	path := filepath.Join(cfg.Web, "src/data/data.remote.go")
	cacheWrite(t, path, strings.Replace(retainedRemote, " }\nfunc Answer", "; Count int }\nfunc Answer", 1))
	observeRetained(t, p, path)
	releaseOnce.Do(func() { close(release) })
	if err := <-done; !errors.Is(err, ErrObsolete) {
		t.Fatalf("stale generation accepted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.Web, "src/data/types.ts")); !os.IsNotExist(err) {
		t.Fatal("stale generation wrote output", err)
	}
	current := prepareRetained(t, p)
	if err := current.Commit(nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(cfg.Web, "src/data/types.ts"))
	if err != nil || !strings.Contains(string(data), `"Count": number`) {
		t.Fatalf("newer generation absent: %s, %v", data, err)
	}
}

func TestRetainedProjectFilesystemEvents(t *testing.T) {
	t.Parallel()
	root, cfg := retainedFixture(t)
	root, _ = filepath.EvalSymlinks(root)
	p, err := OpenProject(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	errs, err := p.Watch()
	if err != nil {
		t.Fatal(err)
	}
	wait := func(want string) {
		t.Helper()
		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()
		for {
			select {
			case err := <-errs:
				if err != nil {
					t.Fatal(err)
				}
			case <-p.Changes():
				p.mu.Lock()
				data := p.current.files[filepath.Join(root, "wire/assets/one.txt")]
				p.mu.Unlock()
				if string(data) == want {
					return
				}
			case <-timer.C:
				t.Fatal("file events did not acquire", want)
			}
		}
	}
	asset := filepath.Join(root, "wire/assets/one.txt")
	info, err := os.Stat(asset)
	if err != nil {
		t.Fatal(err)
	}
	cacheWrite(t, asset+".tmp", "atomic")
	if err := os.Rename(asset+".tmp", asset); err != nil {
		t.Fatal(err)
	}
	wait("atomic")
	cacheWrite(t, asset, "same-time")
	if err := os.Chtimes(asset, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	wait("same-time")
	b := prepareRetained(t, p)
	if err := b.Commit(nil); err != nil {
		t.Fatal(err)
	}
	// Observe the generator's own outputs directly: ownership suppresses noise.
	rev := p.Revision()
	if _, changed, err := p.Observe(context.Background(), filepath.Join(cfg.Web, "src/data/data.remote.ts"), filepath.Join(cfg.Out, generatedGoFile)); err != nil || changed || p.Revision() != rev {
		t.Fatalf("generated writes fed back: %d -> %d %t %v", rev, p.Revision(), changed, err)
	}
}

func TestRetainedProjectConsumedConstantAndMarker(t *testing.T) {
	t.Parallel()
	root, cfg := foreignFixture(t, cacheRemote, nil)
	p, err := OpenProject(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	first := prepareRetained(t, p)
	if err := first.Commit(nil); err != nil {
		t.Fatal(err)
	}
	constPath := filepath.Join(root, "wire/wire.go")
	raw, err := os.ReadFile(constPath)
	if err != nil {
		t.Fatal(err)
	}
	cacheWrite(t, constPath, strings.Replace(string(raw), `StatusOff Status = "off"`, `StatusOff Status = "disabled"`, 1))
	observeRetained(t, p, constPath)
	next := prepareRetained(t, p)
	if err := next.Commit(nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(cfg.Web, "src/lib/skgo/example.com/wire/types.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"disabled"`)) || bytes.Contains(data, []byte(`"off"`)) {
		t.Fatalf("consumed imported constant stale: %s", data)
	}
	remote := filepath.Join(cfg.Web, "src/data/data.remote.go")
	cacheWrite(t, remote, strings.Replace(cacheRemote, "skgo.Query(query)", "skgo.Prerender(query)", 1))
	observeRetained(t, p, remote)
	next = prepareRetained(t, p)
	if err := next.Commit(nil); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join(cfg.Web, "src/data/data.remote.ts"))
	if err != nil || !strings.Contains(string(data), "prerender") {
		t.Fatalf("marker edit not regenerated: %s %v", data, err)
	}
	if stats := p.Stats(); stats.Generations != 3 || stats.Analysis.PackageHits == 0 {
		t.Fatalf("unchanged analysis not retained across producers: %+v", stats)
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

func TestRetainedReadStabilizesEmptyAndAdmitsEmpty(t *testing.T) {
	t.Parallel()
	reads := 0
	data, err := readProjectFileWith(context.Background(), "fixture.go", func(string) ([]byte, error) {
		reads++
		if reads < 3 {
			return []byte{}, nil
		}
		return []byte("package fixture"), nil
	})
	if err != nil || string(data) != "package fixture" || reads != 3 {
		t.Fatalf("transient empty input: %q %v after %d reads", data, err, reads)
	}
	reads = 0
	data, err = readProjectFileWith(context.Background(), "empty.txt", func(string) ([]byte, error) { reads++; return []byte{}, nil })
	if err != nil || data == nil || len(data) != 0 || reads != 4 {
		t.Fatalf("genuinely empty input not admitted: %#v %v after %d reads", data, err, reads)
	}
}

func TestRetainedProjectEquivalentPendingGeneration(t *testing.T) {
	t.Parallel()
	_, cfg := retainedFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	cfg.OnPackageLoad = func(load *packages.Config, _ []string) {
		once.Do(func() {
			close(entered)
			select {
			case <-release:
			case <-load.Context.Done():
			}
		})
	}
	p, err := OpenProject(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	old := make(chan error, 1)
	go func() {
		b, err := p.Prepare(context.Background())
		if b != nil {
			b.Cancel()
		}
		old <- err
	}()
	<-entered
	path := filepath.Join(cfg.Web, "src/data/data.remote.go")
	cacheWrite(t, path, strings.Replace(retainedRemote, "body-one", "body-new", 1))
	observeRetained(t, p, path)
	current := make(chan *ProjectBuild, 1)
	failed := make(chan error, 1)
	go func() {
		b, err := p.Prepare(context.Background())
		if err != nil {
			failed <- err
		} else {
			current <- b
		}
	}()
	close(release)
	if err := <-old; !errors.Is(err, ErrObsolete) {
		t.Fatalf("old input escaped admission: %v", err)
	}
	select {
	case b := <-current:
		defer b.Inputs.Close()
		defer b.Cancel()
		if err := b.Commit(nil); err != nil {
			t.Fatal(err)
		}
		assertRetainedFile(t, b, path, strings.Replace(retainedRemote, "body-one", "body-new", 1))
	case err := <-failed:
		t.Fatal(err)
	case <-time.After(10 * time.Second):
		t.Fatal("equivalent generation did not finish")
	}
	if stats := p.Stats(); stats.Generations != 1 || stats.Pending != 0 {
		t.Fatalf("compatible pending work not shared/bounded: %+v", stats)
	}
}

func TestRetainedProjectModuleSelectionRepair(t *testing.T) {
	t.Parallel()
	root, cfg := retainedFixture(t)
	p, err := OpenProject(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	first := prepareRetained(t, p)
	if err := first.Commit(nil); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "app/go.mod")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cacheWrite(t, path, "module\n")
	observeRetained(t, p, path)
	_, err = p.Prepare(context.Background())
	if err == nil || !strings.Contains(err.Error(), "go.mod") || !p.Diagnose(err) {
		t.Fatalf("invalid module revision not diagnosed: %v", err)
	}
	if err := first.Commit(nil); err == nil {
		t.Fatal("old module inputs republished")
	}
	cacheWrite(t, path, string(original))
	observeRetained(t, p, path)
	repaired := prepareRetained(t, p)
	if err := repaired.Commit(nil); err != nil {
		t.Fatal(err)
	}
	assertRetainedFile(t, repaired, filepath.Join(cfg.Web, "src/data/data.remote.go"), retainedRemote)
}
