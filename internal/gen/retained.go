package gen

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tylergannon/polytype/goload"
	"golang.org/x/tools/go/packages"
)

// ErrObsolete means the selected source was superseded before admission.
var ErrObsolete = errors.New("skgo: source revision is obsolete")

// Revision identifies captured bytes, independent of file modification times.
type Revision uint64

// Project retains source handles and analysis for one app. Observe is fed by
// file events, never HTTP requests. Prepare stages work; Commit admits it only
// while its revision is current. Close joins watching and releases analysis.
// Compiler leases must be closed by their consumers.
type Project struct {
	mu             sync.Mutex
	cfg            Config
	sources        cacheSources
	current        *projectSnapshot
	closed         bool
	changes        chan Revision
	diagnostic     *ProjectDiagnostic
	stats          ProjectStats
	analysisMu     sync.Mutex
	analysisRoot   string
	analysisConfig Config
	analysisFiles  map[string][]byte
	session        goload.Session
	analysis       *goload.View
	plans          map[[32]byte]generationPlan
	pending        map[[32]byte]*generationFuture
	watch          *projectWatcher
	rawEpoch       uint64
	rawFull        uint64
	rawPaths       map[string]uint64
	rawChanges     chan struct{}
	rawControls    []string
	rawNodeCaches  []string
}

type projectSnapshot struct {
	revision        Revision
	environmentHash string
	selectionErr    error
	files           map[string][]byte // immutable byte handles shared with its predecessor
	parsed          map[string]*sourceAnalysis
	graph           *sourceGraph
	key             [32]byte
	metadata        *kitRouteMetadata
	ctx             context.Context
	cancel          context.CancelFunc
}

// ProjectStats describes retained acquisition and shared generation work.
// FileReads counts source acquisitions, not compiler/toolchain reads.
type ProjectStats struct {
	FileReads, FileParses, Generations, GenerationHits uint64
	Files, Analyses, Plans, Pending                    int
	Analysis                                           goload.Stats
}

// ProjectDiagnostic remains attached to the authored source and its revision.
type ProjectDiagnostic struct {
	Revision Revision
	Message  string
}

// OpenProject captures initial membership once. Watch optionally connects the
// owner's Observe method to ordered recursive filesystem events.
func OpenProject(cfg Config) (*Project, error) {
	if len(cfg.Generators) > 0 {
		return nil, fmt.Errorf("skgo: plugin generation requires a fresh generate invocation")
	}
	cfg, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	cfg.Web, err = sourcePath(cfg.Web)
	if err != nil {
		return nil, err
	}
	cfg.Out, err = sourcePath(cfg.Out)
	if err != nil {
		return nil, err
	}
	if cfg.kitMetadata != nil {
		cfg.kitMetadata, err = canonicalRouteMetadata(cfg.kitMetadata)
		if err != nil {
			return nil, err
		}
	}
	sources, err := acquireCacheSources(cfg)
	if err != nil {
		return nil, err
	}
	p := &Project{cfg: cfg, sources: sources, changes: make(chan Revision, 1), plans: map[[32]byte]generationPlan{}, pending: map[[32]byte]*generationFuture{}}
	p.session.SetMaxUniverses(4)
	files, _, err := sources.captureWithRead(cfg, func(path string) ([]byte, error) {
		p.stats.FileReads++
		return readProjectFile(context.Background(), path)
	})
	if err != nil {
		return nil, err
	}
	p.current = p.cloneSnapshot(nil, files, p.cfg.kitMetadata)
	return p, nil
}

// Changes coalesces desired revisions. Consumers always Prepare the current
// revision after waking; the channel does not own a build/session scheduler.
func (p *Project) Changes() <-chan Revision { return p.changes }
func (p *Project) Revision() Revision       { p.mu.Lock(); defer p.mu.Unlock(); return p.current.revision }
func (p *Project) Diagnostic() *ProjectDiagnostic {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.diagnostic == nil {
		return nil
	}
	d := *p.diagnostic
	return &d
}
func (p *Project) Stats() ProjectStats {
	p.mu.Lock()
	s := p.stats
	s.Files = len(p.current.files)
	s.Analyses = len(p.current.parsed)
	s.Plans = len(p.plans)
	s.Pending = len(p.pending)
	p.mu.Unlock()
	s.Analysis = p.session.Stats()
	return s
}

// Observe acquires fresh bytes for the supplied paths. Directory changes expand
// to known descendants and discover new members, including embed matches and
// local replacements. Equal-content events keep the same revision and handles.
func (p *Project) Observe(ctx context.Context, paths ...string) (Revision, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return 0, false, errors.New("skgo: project is closed")
	}
	if err := ctx.Err(); err != nil {
		return p.current.revision, false, err
	}
	before := p.current
	dirty := map[string]bool{}
	rediscover := false
	selectionErr := before.selectionErr
	for _, path := range paths {
		path, err := filepath.Abs(path)
		if err != nil {
			return before.revision, false, err
		}
		path, err = sourcePath(path)
		if err != nil {
			return before.revision, false, err
		}
		dirty[path] = true
		for _, control := range p.sources.Controls {
			if path == control {
				rediscover = true
			}
		}
		_, known := before.files[path]
		info, err := os.Stat(path)
		if err != nil && !os.IsNotExist(err) {
			return before.revision, false, err
		}
		if !known || (info != nil && info.IsDir()) || os.IsNotExist(err) {
			rediscover = true
		}
		if filepath.Ext(path) == ".mod" || filepath.Ext(path) == ".sum" || filepath.Ext(path) == ".work" || filepath.Base(path) == "go.work.sum" {
			rediscover = true
		}
		for name := range before.files {
			if strings.HasPrefix(name, path+string(filepath.Separator)) {
				dirty[name] = true
				rediscover = true
			}
		}
	}
	files := maps.Clone(before.files)
	read := func(path string) ([]byte, error) {
		if data, ok := before.files[path]; ok && !dirty[path] {
			return data, nil
		}
		p.stats.FileReads++
		return readProjectFile(ctx, path)
	}
	if rediscover {
		// Control changes can alter local replacements and workspace membership.
		controls := false
		for path := range dirty {
			if filepath.Base(path) == "go.mod" || filepath.Base(path) == "go.work" {
				controls = true
			}
			for _, control := range p.sources.Controls {
				if path == control {
					controls = true
				}
			}
		}
		if controls {
			sources, err := acquireCacheSources(p.cfg)
			selectionErr = err
			if err == nil {
				p.sources = sources
			}
		}
		var err error
		files, _, err = p.sources.captureWithRead(p.cfg, read)
		if err != nil && selectionErr != nil {
			// An invalid module/workspace file cannot redefine membership yet,
			// but its captured bytes still become the desired revision.
			files = maps.Clone(before.files)
			for path := range dirty {
				data, e := read(path)
				if e != nil && !os.IsNotExist(e) {
					return before.revision, false, e
				}
				files[path] = data
			}
		} else if err != nil {
			return before.revision, false, err
		}
	} else {
		for path := range dirty {
			data, err := read(path)
			if os.IsNotExist(err) {
				delete(files, path)
				continue
			}
			if err != nil {
				return before.revision, false, err
			}
			if cacheOwned(p.cfg, path, data) {
				continue
			}
			files[path] = data
			// Embed directive/import changes may alter captured resource membership.
			if strings.HasSuffix(path, ".go") && (bytes.Contains(data, []byte("//go:embed")) || bytes.Contains(before.files[path], []byte("//go:embed"))) {
				files, _, err = p.sources.captureWithRead(p.cfg, func(name string) ([]byte, error) {
					if name == path {
						return data, nil
					}
					return read(name)
				})
				if err != nil {
					return before.revision, false, err
				}
			}
		}
	}
	if sameCapturedFiles(before.files, files) && before.environmentHash == p.sources.EnvironmentHash {
		return before.revision, false, nil
	}
	p.current = p.cloneSnapshot(before, files, before.metadata)
	p.current.selectionErr = selectionErr
	if before.key != p.current.key {
		before.cancel()
	} // preserve equivalent pending generation, invalidate changed keys
	p.diagnostic = nil
	select {
	case <-p.changes:
	default:
	}
	p.changes <- p.current.revision
	return p.current.revision, true, nil
}

// ObserveRoutes admits new Kit metadata into the same desired revision stream
// as file events. Its caller owns frontend matcher validation.
func (p *Project) ObserveRoutes(metadata *RouteMetadata) (Revision, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return 0, false, errors.New("skgo: project is closed")
	}
	var err error
	metadata, err = canonicalRouteMetadata(metadata)
	if err != nil {
		return p.current.revision, false, err
	}
	raw, _ := json.Marshal(metadata)
	old, _ := json.Marshal(p.current.metadata)
	if bytes.Equal(raw, old) {
		return p.current.revision, false, nil
	}
	before := p.current
	p.current = p.cloneSnapshot(before, before.files, cloneRouteMetadata(metadata))
	if before.key != p.current.key {
		before.cancel()
	}
	p.diagnostic = nil
	select {
	case <-p.changes:
	default:
	}
	p.changes <- p.current.revision
	return p.current.revision, true, nil
}

func sameCapturedFiles(a, b map[string][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for path, data := range a {
		v, ok := b[path]
		if !ok || (data == nil) != (v == nil) || !bytes.Equal(data, v) {
			return false
		}
	}
	return true
}

// Vite readModifiedFile's bounded retry is adapted to Go-owned acquisition.
// A transient empty read is retried; genuinely empty content is admitted after
// the bound. No mtime/size fingerprint is allowed to substitute for fresh bytes.
func readProjectFile(ctx context.Context, path string) ([]byte, error) {
	return readProjectFileWith(ctx, path, os.ReadFile)
}
func readProjectFileWith(ctx context.Context, path string, read func(string) ([]byte, error)) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		data, err := read(path)
		if (err == nil && len(data) > 0) || attempt == 3 || (!os.IsNotExist(err) && err != nil) {
			return data, err
		}
		timer := time.NewTimer(2 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

// CompilerInputs is a lease on the exact selected private source view. It
// contains authored and generated Go, resources, rewritten local modules and
// frozen selection environment. Stock Go owns compilation and artifact caching.
// Files returns a copy so consumers cannot mutate retained handles.
type CompilerInputs struct {
	Revision Revision
	// EnvironmentIdentity identifies captured Go source-selection settings,
	// independently of the disposable materialization paths in Env.
	EnvironmentIdentity string
	Dir, Web, Out       string
	Env                 []string
	view                *generationSourceView
	links               *routeLinks
	files               map[string][]byte
	close               func()
	once                sync.Once
}

func (i *CompilerInputs) Files() map[string][]byte {
	out := make(map[string][]byte, len(i.files))
	for p, b := range i.files {
		out[p] = bytes.Clone(b)
	}
	return out
}
func (i *CompilerInputs) AuthoredPath(path string) string {
	return i.view.mapPath(i.links.authoredPath(path), false)
}
func (i *CompilerInputs) PrivatePath(path string) string {
	canonical, err := sourcePath(path)
	if err != nil {
		return path
	}
	return i.view.mapPath(canonical, true)
}

// CompiledPath also maps route colocation to its generated Go import address.
func (i *CompilerInputs) CompiledPath(path string) string {
	mapped := i.PrivatePath(path)
	if link := i.links.byDir[filepath.Dir(mapped)]; link != nil {
		return filepath.Join(link.linkDir, filepath.Base(mapped))
	}
	return mapped
}
func (i *CompilerInputs) AuthoredText(text string) string {
	for _, link := range i.links.links {
		text = strings.ReplaceAll(text, link.linkDir+string(filepath.Separator), link.dir+string(filepath.Separator))
		rel, _ := filepath.Rel(i.Dir, link.linkDir)
		text = strings.ReplaceAll(text, filepath.ToSlash(rel)+"/", i.view.mapPath(link.dir, false)+"/")
	}
	return i.view.originalText(text)
}
func (i *CompilerInputs) Close() {
	if i != nil {
		i.once.Do(i.close)
	}
}

// ProjectBuild owns prepared output and compiler inputs until Commit or Cancel.
// Prepare never publishes generated files or diagnostics into the authored tree;
// Publish and Commit do.
type ProjectBuild struct {
	Revision   Revision
	Inputs     *CompilerInputs
	project    *Project
	snapshot   *projectSnapshot
	plan       generationPlan
	mu         sync.Mutex
	closed     bool
	eventEpoch *uint64
}

// Prepare shares generation by consumed declaration key. Each consumer gets its
// own compiler view, including fresh function bodies even on a generation hit.
func (p *Project) Prepare(ctx context.Context) (*ProjectBuild, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, errors.New("skgo: project is closed")
	}
	s := p.current
	if s.selectionErr != nil {
		p.mu.Unlock()
		return nil, &RevisionError{Revision: s.revision, Err: s.selectionErr}
	}
	sources := p.sources
	f := p.pending[s.key]
	if f == nil {
		f = newGenerationFuture()
		p.pending[s.key] = f
	}
	p.mu.Unlock()
	waitCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	defer cancel()
	plan, err := f.get(waitCtx, func(work context.Context) (generationPlan, error) {
		p.analysisMu.Lock()
		defer p.analysisMu.Unlock()
		p.mu.Lock()
		cached, hit := p.plans[s.key]
		if hit {
			p.stats.GenerationHits++
		}
		p.mu.Unlock()
		if hit {
			return cached, nil
		}
		if err := work.Err(); err != nil {
			return generationPlan{}, err
		}
		plan, err := p.generate(work, s, sources)
		if err == nil {
			p.mu.Lock()
			p.stats.Generations++
			// One retained successful key is sufficient: outstanding consumers own
			// their plans. Obsolete work cannot evict the current plan.
			if p.current.key == s.key {
				clear(p.plans)
				p.plans[s.key] = plan
			}
			p.mu.Unlock()
		}
		return plan, err
	})
	p.mu.Lock()
	if f.finished() && p.pending[s.key] == f {
		delete(p.pending, s.key)
	}
	current := p.current == s && !p.closed
	p.mu.Unlock()
	if !current {
		return nil, ErrObsolete
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, &RevisionError{Revision: s.revision, Err: err}
	}
	cfg, cleanup, err := sources.materialize(p.cfg, s.files)
	if err != nil {
		return nil, &RevisionError{Revision: s.revision, Err: err}
	}
	fail := func(err error) (*ProjectBuild, error) { cleanup(); return nil, err }
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	if err := publishGenerationPlan(cfg, rebaseGenerationPlan(plan, cfg.sourceView)); err != nil {
		return fail(err)
	}
	dir, module, err := moduleOf(cfg.Out)
	if err != nil {
		return fail(err)
	}
	inputs := &CompilerInputs{Revision: s.revision, EnvironmentIdentity: s.environmentHash, Dir: dir, Web: cfg.Web, Out: cfg.Out, Env: append([]string(nil), cfg.sourceView.env...), view: cfg.sourceView, files: map[string][]byte{}, close: cleanup}

	inputs.links, err = newRouteLinks(cfg, dir, module)
	if err != nil {
		inputs.Close()
		return nil, err
	}
	// Include the route links and rewritten build controls that Go consumes, as
	// well as authored files and resources. A route link is a relative
	// directory symlink inside the same private tree; its Go files are recorded
	// at the address Go compiles them by. This walk is over the selected
	// immutable private tree; it never reacquires mutable project bytes.
	for _, root := range cfg.sourceView.roots {
		err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				if fi, e := os.Stat(path); e != nil || fi.IsDir() {
					if inputs.links.byLink[path] == nil || e != nil {
						return nil
					}
					names, e := goFileNames(path)
					for _, name := range names {
						if e == nil {
							inputs.files[filepath.Join(path, name)], e = os.ReadFile(filepath.Join(path, name))
						}
					}
					return e
				}
			}
			data, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			inputs.files[path] = data
			return nil
		})
		if err != nil {
			inputs.Close()
			return nil, err
		}
	}

	p.mu.Lock()
	current = p.current == s && !p.closed
	p.mu.Unlock()
	if !current {
		inputs.Close()
		return nil, ErrObsolete
	}
	return &ProjectBuild{Revision: s.revision, Inputs: inputs, project: p, snapshot: s, plan: plan}, nil
}

// RevisionError associates external compiler/generator text with captured bytes.
// Private source paths have already been mapped to authored paths.
type RevisionError struct {
	Revision Revision
	Err      error
}

func (e *RevisionError) Error() string {
	return fmt.Sprintf("skgo: revision %d: %v", e.Revision, e.Err)
}
func (e *RevisionError) Unwrap() error { return e.Err }

func rebaseGenerationPlan(plan generationPlan, view *generationSourceView) generationPlan {
	out := generationPlan{Files: map[string]string{}}
	// Generated imports contain module identities, never private root paths.
	for path, source := range plan.Files {
		out.Files[view.mapPath(path, true)] = source
	}
	for _, path := range plan.GoFiles {
		out.GoFiles = append(out.GoFiles, view.mapPath(path, true))
	}
	return out
}

// Commit serializes admission with Observe. The optional callback is where the
// compiler/host owner admits already prepared inputs; it must not reenter this
// Project. A failed callback leaves generated outputs unpublished.
func (b *ProjectBuild) Commit(admit func(*CompilerInputs) error) error {
	return b.commit(admit, true)
}

// Publish writes the generated outputs of a current build and keeps it open, so
// Kit can register new declarations before Commit admits the host built from
// them.
func (b *ProjectBuild) Publish() error {
	return b.commit(nil, false)
}

func (b *ProjectBuild) commit(admit func(*CompilerInputs) error, close bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return errors.New("skgo: build is closed")
	}
	p := b.project
	plan, formatErr := b.plan.formatted()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.current != b.snapshot || (b.eventEpoch != nil && p.rawEpoch != *b.eventEpoch) {
		return ErrObsolete
	}
	if formatErr != nil {
		// The formatter may succeed once the app's tools are back; the next
		// Prepare must generate again rather than replay this plan.
		if p.plans[b.snapshot.key].frontend == b.plan.frontend {
			delete(p.plans, b.snapshot.key)
		}
		return formatErr
	}
	if admit != nil {
		if err := admit(b.Inputs); err != nil {
			return err
		}
	}
	if err := publishGenerationPlan(p.cfg, plan); err != nil {
		return err
	}
	p.diagnostic = nil
	b.closed = close
	return nil
}

// Diagnose publishes only a current revision's authored diagnostic.
func (b *ProjectBuild) Diagnose(err error) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return false
	}
	p := b.project
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.current != b.snapshot {
		return false
	}
	p.diagnostic = &ProjectDiagnostic{Revision: b.Revision, Message: b.Inputs.AuthoredText(err.Error())}
	return true
}
func (p *Project) Diagnose(err error) bool {
	var re *RevisionError
	if !errors.As(err, &re) {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.current.revision != re.Revision {
		return false
	}
	p.diagnostic = &ProjectDiagnostic{Revision: re.Revision, Message: re.Err.Error()}
	return true
}

// Cancel releases this consumer's compiler lease. Commit does not release it:
// an admitted host may still need it until the compiler owner closes it.
func (b *ProjectBuild) Cancel() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.closed {
		b.closed = true
		b.Inputs.Close()
	}
}
func (b *ProjectBuild) Current() bool {
	p := b.project
	p.mu.Lock()
	defer p.mu.Unlock()
	return !p.closed && p.current == b.snapshot
}

func (p *Project) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.current.cancel()
	w := p.watch
	p.mu.Unlock()
	if w != nil {
		w.close()
	}
	p.analysisMu.Lock()
	defer p.analysisMu.Unlock()
	if p.analysis != nil {
		_ = p.analysis.Close()
	}
	if p.analysisRoot != "" {
		return os.RemoveAll(p.analysisRoot)
	}
	return nil
}

func (p *Project) generate(ctx context.Context, s *projectSnapshot, sources cacheSources) (generationPlan, error) {
	cfg := p.cfg
	cfg.kitMetadata = s.metadata
	cfg.RouteMetadata = nil
	cfg.Session = &p.session
	if p.analysisRoot == "" {
		root, err := os.MkdirTemp("", "skgo-retained-analysis-")
		if err != nil {
			return generationPlan{}, err
		}
		p.analysisRoot, err = filepath.EvalSymlinks(root)
		if err != nil {
			return generationPlan{}, err
		}
	}
	if p.analysisConfig.sourceView != nil {
		for path := range p.analysisFiles {
			if _, ok := s.files[path]; !ok {
				_ = os.Remove(p.analysisConfig.sourceView.mapPath(path, true))
			}
		}
	}
	cfg, _, err := sources.materializeIn(cfg, s.files, p.analysisRoot)
	if err != nil {
		return generationPlan{}, err
	}
	p.analysisConfig = cfg
	p.analysisFiles = s.files
	analysisInputs := make(map[string][]byte, len(s.files))
	for path, data := range s.files {
		target := cfg.sourceView.mapPath(path, true)
		if target != path && data != nil && !goReadsContent(target) {
			// Go selects these by name alone, so editing a component beside a
			// route's Go keeps the view's discovery.
			analysisInputs[target] = []byte{}
		} else if target != path && data != nil {
			actual, e := os.ReadFile(target)
			if e != nil {
				return generationPlan{}, e
			}
			analysisInputs[target] = actual
		}
	}
	if p.analysis == nil {
		p.analysis = p.session.PrepareView(analysisInputs)
	} else if err := p.analysis.Update(analysisInputs); err != nil {
		return generationPlan{}, err
	}
	cfg.analysisView = p.analysis
	result := &generationCache{deferFormat: true}
	cfg.cache = result
	// Go package loads are cancellable through the shared invocation context.
	original := cfg.OnPackageLoad
	cfg.OnPackageLoad = func(load *packages.Config, patterns []string) {
		load.Context = ctx
		if original != nil {
			original(load, patterns)
		}
	}
	if err := runUncached(cfg); err != nil {
		for path, a := range s.parsed {
			if a.err != nil && strings.Contains(err.Error(), filepath.Base(path)) {
				return generationPlan{}, a.err
			}
		}
		return generationPlan{}, fmt.Errorf("%s", cfg.sourceView.originalText(err.Error()))
	}
	return result.plan, nil
}

// goReadsContent reports whether Go's package selection reads a file's bytes:
// Go and the other sources go/build checks for constraints, and module and
// workspace files. Any other file, such as an embedded one, counts by name.
func goReadsContent(path string) bool {
	switch filepath.Base(path) {
	case "go.mod", "go.sum", "go.work", "go.work.sum", "modules.txt":
		return true
	}
	switch filepath.Ext(path) {
	case ".go", ".c", ".cc", ".cpp", ".cxx", ".m", ".h", ".hh", ".hpp", ".hxx", ".f", ".F", ".for", ".f90", ".s", ".S", ".sx", ".swig", ".swigcxx":
		return true
	}
	return false
}

// RequireEventEpoch additionally guards admission against raw generator inputs.
// Call before Commit; the event check and application admission share the lock.
func (b *ProjectBuild) RequireEventEpoch(epoch uint64) { b.eventEpoch = &epoch }
