package gen

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// RawSelection is independent of compiler revisions and output ownership.
// Acknowledgement only removes paths from a successful, still-current pass.
type RawSelection struct {
	Epoch uint64
	Full  bool
	Paths []string
}

func (p *Project) RawChanges() <-chan struct{} {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.rawChanges == nil {
		p.rawChanges = make(chan struct{}, 1)
		p.rawPaths = map[string]uint64{}
		// NODE_COMPILE_CACHE is private Node tooling output. The repository's
		// conventional per-module root is handled below; also respect an explicit
		// configured root, including relative roots used by frontend/tool commands.
		if cache := os.Getenv("NODE_COMPILE_CACHE"); cache != "" {
			roots := []string{}
			if filepath.IsAbs(cache) {
				roots = append(roots, cache)
			} else {
				if absolute, err := filepath.Abs(cache); err == nil {
					roots = append(roots, absolute)
				}
				roots = append(roots, filepath.Join(p.cfg.Web, cache))
				for _, module := range p.sources.Modules {
					roots = append(roots, filepath.Join(module.Dir, cache))
				}
			}
			for _, root := range roots {
				if canonical, err := sourcePath(root); err == nil {
					p.rawNodeCaches = append(p.rawNodeCaches, canonical)
				}
			}
		}
	}
	return p.rawChanges
}
func (p *Project) RecordRaw(full bool, paths ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.rawChanges == nil {
		return
	}

	// The link tree is SKGO's private mirror, not an application producer
	// input. Its authored originals are watched separately. Publication can
	// rewrite a mirrored registration twice while reconciling imports; feeding
	// those internal copies back into generation cannot converge.
	selected := make([]string, 0, len(paths))
	for _, path := range paths {
		canonical, err := sourcePath(path)
		if err == nil && !p.rawToolingPath(canonical) {
			selected = append(selected, canonical)
		}
	}
	if !full && len(selected) == 0 {
		return
	}
	p.rawEpoch++
	if full {
		p.rawFull = p.rawEpoch
	}
	for _, path := range selected {
		p.rawPaths[path] = p.rawEpoch
	}

	select {
	case p.rawChanges <- struct{}{}:
	default:
	}
}
func (p *Project) RawSelection() RawSelection {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := RawSelection{Epoch: p.rawEpoch, Full: p.rawFull != 0}
	for path := range p.rawPaths {
		s.Paths = append(s.Paths, path)
	}
	sort.Strings(s.Paths)
	return s
}
func (p *Project) AcknowledgeRaw(epoch uint64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.rawEpoch != epoch {
		return false
	}
	clear(p.rawPaths)
	p.rawFull = 0
	return true
}

// Reconcile captures new and removed generated files immediately, before a
// compiler lease is prepared, without waiting for eventual watcher delivery.
func (p *Project) Reconcile(ctx context.Context) error {
	p.mu.Lock()
	var roots []string
	for _, module := range p.sources.Modules {
		roots = append(roots, module.Dir)
	}
	p.mu.Unlock()
	_, _, err := p.Observe(ctx, roots...)
	return err
}

// WatchRawFile adds an explicit tooling control before Watch starts. It does
// not broaden compiler source selection or recursively watch external trees.
func (p *Project) WatchRawFile(path string) error {
	canonical, err := sourcePath(path)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.watch != nil {
		return fmt.Errorf("raw controls must be selected before Watch")
	}
	p.rawControls = append(p.rawControls, canonical)
	return nil
}

// rawToolingPath names owned implementation output, not arbitrary generated
// files or dot directories. Authored generator inputs and owned application
// outputs remain in the freshness domain, including removals.
func (p *Project) rawToolingPath(path string) bool {
	for _, control := range p.rawControls {
		if path == control {
			return false
		}
	}
	if withinTree(filepath.Join(p.cfg.Out, linkRootName), path) {
		return true
	}
	for _, module := range p.sources.Modules {
		if withinTree(filepath.Join(module.Dir, ".cache", "node-compile"), path) {
			return true
		}
	}
	for _, root := range p.rawNodeCaches {
		if withinTree(root, path) {
			return true
		}
	}
	return false
}
