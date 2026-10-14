package gen

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/tylergannon/polytype/goload"
)

// Worker owns serial generation and a stable private source view for a
// development session. Close releases its files and retained analysis.
// A Worker must not be copied after first use.
type Worker struct {
	mu           sync.Mutex
	root         string
	session      *goload.Session
	previous     map[string][]byte
	materialized map[string][]byte
	modules      []cacheModule
	view         *goload.View
	viewSession  *goload.Session
	sources      *cacheSources
}

// Run generates the current authored inputs, retaining compatible analysis for
// the next call. Persistent cache hits still replay without using the session.
func (w *Worker) Run(cfg Config) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.session == nil {
		w.session = new(goload.Session)
		w.session.SetMaxUniverses(3)
	}
	if cfg.Session == nil {
		cfg.Session = w.session
	}
	cfg.worker = w
	return Run(cfg)
}

// Close releases private source files and analysis. A subsequent Run starts a
// fresh development session.
func (w *Worker) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	var err error
	if w.view != nil {
		err = w.view.Close()
	}
	err = errors.Join(err, os.RemoveAll(w.root))
	w.view, w.viewSession, w.sources = nil, nil, nil
	w.root, w.session, w.previous, w.materialized, w.modules = "", nil, nil, nil, nil
	return err
}

func acquireGenerationSources(cfg Config) (cacheSources, map[string][]byte, string, error) {
	if w := cfg.worker; w != nil && w.sources != nil {
		sources := *w.sources
		environment, err := currentCacheEnvironment(cfg.Out)
		if err == nil && cacheEnvironmentHash(environment) == sources.EnvironmentHash {
			sources.Environment = environment
			files, digest, err := sources.capture(cfg)
			if err == nil && sameModuleControls(sources, w.previous, files) {
				return sources, files, digest, nil
			}
		}
	}
	sources, err := acquireCacheSources(cfg)
	if err != nil {
		return sources, nil, "", err
	}
	files, digest, err := sources.capture(cfg)
	return sources, files, digest, err
}

func sameModuleControls(sources cacheSources, before, after map[string][]byte) bool {
	controls := map[string]bool{}
	for _, path := range sources.Controls {
		controls[path] = true
	}
	for _, files := range []map[string][]byte{before, after} {
		for path := range files {
			switch filepath.Base(path) {
			case "go.mod", "go.sum", "go.work", "go.work.sum":
				controls[path] = true
			}
		}
	}
	for path := range controls {
		if !bytes.Equal(before[path], after[path]) {
			return false
		}
	}
	return true
}
