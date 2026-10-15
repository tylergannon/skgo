// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license in GOPLS_LICENSE.
//
// Semantic port of gopls/internal/filewatcher fsnotify_watcher.go v0.52.0
// (16517d9): NewFSNotifyWatcher/run/process/convertEvent/WatchDir. Directory
// creation expands pre-order, rename deletes the old scope, and one delivery
// loop preserves observed ordering. SKGO watches embed resources as well as Go.
package gen

import (
	"context"
	"errors"
	"github.com/fsnotify/fsnotify"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

type projectWatcher struct {
	project *Project
	fs      *fsnotify.Watcher
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	dirs    map[string]bool
	errors  chan error
	once    sync.Once
}

// Watch starts event-fed acquisition. It never starts generation or a compiler.
// Errors must be consumed by the session owner; uncertain scopes reconcile
// immediately, outside request handling, before another revision is announced.
func (p *Project) Watch() (<-chan error, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, errors.New("skgo: project is closed")
	}
	if p.watch != nil {
		return p.watch.errors, nil
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	w := &projectWatcher{project: p, fs: watcher, ctx: ctx, cancel: cancel, dirs: map[string]bool{}, errors: make(chan error, 8)}
	for _, module := range p.sources.Modules {
		if err := w.watchDir(module.Dir); err != nil {
			_ = watcher.Close()
			cancel()
			return nil, err
		}
	}
	// Selection controls outside modules need parent watches (not whole ancestor
	// trees). This detects a newly created go.work and preserved-mtime writes.
	for _, control := range p.sources.Controls {
		dir := filepath.Dir(control)
		if dir == filepath.Dir(dir) {
			continue
		} // kqueue cannot watch the volume root with dangling system symlinks.
		if !w.dirs[dir] {
			if err := watcher.Add(dir); err != nil && !os.IsNotExist(err) {
				_ = watcher.Close()
				cancel()
				return nil, err
			}
			w.dirs[dir] = true
		}
	}
	p.watch = w
	w.wg.Add(1)
	go w.run()
	return w.errors, nil
}
func (w *projectWatcher) watchDir(dir string) error {
	return filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		if path != dir && cacheToolingDirectory(entry.Name()) {
			return filepath.SkipDir
		}
		if !w.dirs[path] {
			if err := w.fs.Add(path); err != nil {
				return err
			}
			w.dirs[path] = true
		}
		return nil
	})
}
func (w *projectWatcher) report(err error) {
	select {
	case w.errors <- err:
	default:
	}
}
func (w *projectWatcher) run() {
	defer w.wg.Done()
	defer close(w.errors)
	for {
		select {
		case <-w.ctx.Done():
			return
		case err, ok := <-w.fs.Errors:
			if !ok {
				return
			}
			w.report(err)
			// Overflow means event completeness is unknown. Reconcile all selected
			// module scopes rather than trusting their retained byte handles.
			w.project.mu.Lock()
			var roots []string
			for _, module := range w.project.sources.Modules {
				roots = append(roots, module.Dir)
			}
			w.project.mu.Unlock()
			if _, _, e := w.project.Observe(w.ctx, roots...); e != nil && w.ctx.Err() == nil {
				w.report(e)
			}
		case event, ok := <-w.fs.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}
			w.project.mu.Lock()
			interested := false
			for _, module := range w.project.sources.Modules {
				if withinTree(module.Dir, event.Name) {
					interested = true
					break
				}
			}
			for _, control := range w.project.sources.Controls {
				if event.Name == control {
					interested = true
					break
				}
			}
			w.project.mu.Unlock()
			if !interested {
				continue
			}
			// Ownership, not merely a filename suffix, prevents output feedback loops.
			if data, err := os.ReadFile(event.Name); err == nil && cacheOwned(w.project.cfg, event.Name, data) {
				continue
			}
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					if err := w.watchDir(event.Name); err != nil {
						w.report(err)
					}
				}
			}
			if event.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
				for dir := range w.dirs {
					if dir == event.Name || withinTree(event.Name, dir) {
						_ = w.fs.Remove(dir)
						delete(w.dirs, dir)
					}
				}
			}
			if _, _, err := w.project.Observe(w.ctx, event.Name); err != nil && w.ctx.Err() == nil {
				w.report(err)
			}
			// A module/workspace event may introduce another local replacement.
			if filepath.Base(event.Name) == "go.mod" || filepath.Base(event.Name) == "go.work" {
				w.project.mu.Lock()
				modules := append([]cacheModule(nil), w.project.sources.Modules...)
				w.project.mu.Unlock()
				for _, module := range modules {
					if err := w.watchDir(module.Dir); err != nil {
						w.report(err)
					}
				}
			}
		}
	}
}
func (w *projectWatcher) close() { w.once.Do(func() { w.cancel(); _ = w.fs.Close(); w.wg.Wait() }) }
