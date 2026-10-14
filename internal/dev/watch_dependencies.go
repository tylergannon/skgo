package dev

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Refresh only at build boundaries. Go chooses the dependency modules and embed
// patterns; polling then checks their local filesystem inputs without running Go.
func (in *inputs) refresh(ctx context.Context, cfg Config) error {
	args := append([]string{"list", "-e", "-deps", "-json=Dir,EmbedPatterns,Module"}, cfg.BuildFlags...)
	args = append(args, cfg.Package)
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = cfg.Root
	raw, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("discover watched dependencies: %w", err)
	}
	type module struct {
		Dir, Version string
		Main         bool
		Replace      *module
	}
	roots, embeds := map[string]bool{}, map[string]bool{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	for {
		var p struct {
			Dir           string
			EmbedPatterns []string
			Module        *module
		}
		if err := decoder.Decode(&p); err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		m := p.Module
		if m == nil {
			continue
		}
		local := m.Main
		if m.Replace != nil {
			m = m.Replace
			local = m.Version == ""
		}
		if !local || m.Dir == "" {
			continue
		}
		if !withinDir(in.root, m.Dir) {
			roots[m.Dir] = true
		}
		for _, pattern := range p.EmbedPatterns {
			// A conservative directory prefix catches additions/deletions as well as
			// edits. Go alone decides which matches actually enter the application.
			parts := strings.Split(strings.TrimPrefix(pattern, "all:"), "/")
			var prefix []string
			for _, part := range parts {
				if strings.ContainsAny(part, "*?[") {
					break
				}
				prefix = append(prefix, part)
			}
			embeds[filepath.Join(append([]string{p.Dir}, prefix...)...)] = true
		}
	}
	workspace := exec.CommandContext(ctx, "go", "env", "GOWORK")
	workspace.Dir = cfg.Root
	if raw, err := workspace.Output(); err != nil {
		return err
	} else if path := strings.TrimSpace(string(raw)); filepath.IsAbs(path) {
		embeds[path] = true
		embeds[path+".sum"] = true
	}
	var localRoots, embedPaths []string
	for path := range roots {
		localRoots = append(localRoots, path)
	}
	for path := range embeds {
		if in.out != "" && withinDir(in.out, path) {
			continue
		}
		embedPaths = append(embedPaths, path)
	}
	sort.Strings(localRoots)
	sort.Strings(embedPaths)
	in.mu.Lock()
	in.localRoots, in.embedPaths = localRoots, embedPaths
	in.mu.Unlock()
	return nil
}

func (in *inputs) dependencyFingerprint(lines *[]string) error {
	in.mu.RLock()
	roots := append([]string(nil), in.localRoots...)
	embeds := append([]string(nil), in.embedPaths...)
	in.mu.RUnlock()
	for _, root := range roots {
		extra := inputs{root: root}
		sum, err := extra.fingerprint()
		if err != nil {
			return err
		}
		*lines = append(*lines, "module "+root+" "+sum)
	}
	for _, path := range embeds {
		err := filepath.WalkDir(path, func(name string, d fs.DirEntry, err error) error {
			if os.IsNotExist(err) {
				*lines = append(*lines, "missing embed "+name)
				return nil
			}
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			return in.stat(lines, "embed "+name, d)
		})
		if err != nil {
			return err
		}
	}
	return nil
}
