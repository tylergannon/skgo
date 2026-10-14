package dev

import (
	"path/filepath"
	"reflect"
	"slices"
	"strings"
)

// WatchInputs is source selection already discovered by a retained generator.
// Roots are mutable Go modules. Files are source/control inputs; frontend
// components remain Vite-owned. EmbedPatterns include additions to Go embeds.
type WatchInputs struct{ Roots, Files, EmbedPatterns []string }

func (s *Server) updateInputs() bool {
	if s.cfg.WatchInputs == nil {
		return false
	}
	selected := s.cfg.WatchInputs()
	var roots, files []string
	for _, root := range selected.Roots {
		root = s.absolute(root)
		if root != s.cfg.Root {
			roots = append(roots, root)
		}
	}
	for _, path := range selected.Files {
		if strings.HasSuffix(path, string(filepath.Separator)) {
			continue
		}
		path = s.absolute(path)
		// Mutable module source and frontend route membership are already watched.
		if strings.HasSuffix(path, ".go") {
			continue
		}
		if withinDir(filepath.Join(s.cfg.Web, "src"), path) {
			continue
		}
		if s.outputPath(path) {
			continue
		}
		files = append(files, path)
	}
	for _, pattern := range selected.EmbedPatterns {
		pattern = s.absolute(pattern)
		if !s.outputPath(pattern) {
			files = append(files, pattern)
		}
	}
	slices.Sort(roots)
	roots = slices.Compact(roots)
	slices.Sort(files)
	files = slices.Compact(files)
	s.inputMu.Lock()
	defer s.inputMu.Unlock()
	if reflect.DeepEqual(s.in.roots, roots) && reflect.DeepEqual(s.in.files, files) {
		return false
	}
	s.in.roots, s.in.files = roots, files
	return true
}

func (s *Server) absolute(path string) string {
	if !filepath.IsAbs(path) {
		return filepath.Join(s.cfg.Root, path)
	}
	return filepath.Clean(path)
}

func (s *Server) outputPath(path string) bool {
	return s.cfg.Out != "" && withinDir(s.cfg.Out, path) || withinDir(filepath.Join(s.cfg.Web, "build"), path)
}

func (s *Server) fingerprint() (string, error) {
	s.inputMu.RLock()
	defer s.inputMu.RUnlock()
	return s.in.fingerprint()
}
