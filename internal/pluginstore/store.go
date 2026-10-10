// Package pluginstore locates installations for the exact executing binary.
package pluginstore

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ProbeFiles is private child-process state, never plugin metadata.
const ProbeFiles = "_SKGO_PLUGIN_PROBE_FILES"

type Source struct {
	Module  string `json:"module"`
	Version string `json:"version"`
}

func Digest(executable string) (string, error) {
	f, e := os.Open(executable)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
func Hosts(home string) string     { return filepath.Join(home, ".skgo", "plugins", "hosts") }
func Dir(home, host string) string { return filepath.Join(Hosts(home), host) }
func Slot(home, host, module string) string {
	return filepath.Join(Dir(home, host), fmt.Sprintf("%x.so", sha256.Sum256([]byte(module))))
}
func Reference(slot string) string { return strings.TrimSuffix(slot, ".so") + ".json" }

// Hints never opens other hosts' binaries and does not choose a source version.
func Hints(home, host string) ([]string, error) {
	dirs, err := os.ReadDir(Hosts(home))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	current := map[string]bool{}
	entries, err := os.ReadDir(Dir(home, host))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".so" {
			current[e.Name()] = true
		}
	}
	refs := map[string]bool{}
	var paths, bad []string
	for _, dir := range dirs {
		if !dir.IsDir() || dir.Name() == host {
			continue
		}
		path := filepath.Join(Hosts(home), dir.Name())
		files, e := os.ReadDir(path)
		if e != nil {
			return nil, e
		}
		hasPlugin := false
		for _, f := range files {
			if f.IsDir() || filepath.Ext(f.Name()) != ".so" {
				continue
			}
			if !hasPlugin {
				paths = append(paths, path)
				hasPlugin = true
			}
			var s Source
			b, e := os.ReadFile(Reference(filepath.Join(path, f.Name())))
			if e == nil {
				e = json.Unmarshal(b, &s)
			}
			if e != nil || s.Module == "" || s.Version == "" || filepath.Base(Slot(home, dir.Name(), s.Module)) != f.Name() {
				bad = append(bad, "retained plugin has missing/malformed source reference: "+path+"/"+f.Name())
				continue
			}
			if !current[f.Name()] {
				refs[s.Module+"@"+s.Version] = true
			}
		}
	}
	if len(paths) == 0 {
		return nil, nil
	}
	out := []string{fmt.Sprintf("%d retained installation directories belong to other skgo executables; not loaded. Deliberate cleanup: %s", len(paths), strings.Join(paths, ", "))}
	var sorted []string
	for ref := range refs {
		sorted = append(sorted, ref)
	}
	sort.Strings(sorted)
	for _, ref := range sorted {
		out = append(out, "retained source (not a restore recommendation): skgo plugin install "+ref+"; use go tool skgo plugin install for a project host")
	}
	return append(out, bad...), nil
}

// RemovalHints maps current-host opaque slots back to their source module. Callers
// show these alongside discovery and qualification diagnostics so the removal
// command does not require remembering which module produced a digest-named file.
func RemovalHints(dir string) ([]string, error) {
	files, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var hints []string
	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".so" {
			continue
		}
		path := filepath.Join(dir, f.Name())
		b, err := os.ReadFile(Reference(path))
		var source Source
		if err == nil {
			err = json.Unmarshal(b, &source)
		}
		expected := fmt.Sprintf("%x.so", sha256.Sum256([]byte(source.Module)))
		if err != nil || source.Module == "" || source.Version == "" || f.Name() != expected {
			hints = append(hints, "managed plugin "+path+" has a missing/malformed source reference; remove that exact file deliberately")
			continue
		}
		hints = append(hints, fmt.Sprintf("managed plugin %s: %s@%s; current-host removal: skgo plugin remove %s (use go tool skgo plugin remove %s for this project-tool host)", path, source.Module, source.Version, source.Module, source.Module))
	}
	return hints, nil
}
