// Package templates loads trusted development plugins and resolves add-on recipes.
package templates

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tylergannon/skgo/templateapi"
)

type Entry struct {
	Path       string
	Plugin     templateapi.Plugin
	Descriptor templateapi.Descriptor
}

type Registry struct {
	Entries     []Entry
	Diagnostics []string
}

// Discover scans only immediate .so files. Failed opens have unknown exports and
// do not participate in name resolution. Initialization runs trusted plugin code.
func Discover(config, home, cwd, version string, builtins ...templateapi.Plugin) (*Registry, error) {
	paths, diagnostics, err := Files(config, home, cwd, "")
	if err != nil {
		return nil, err
	}
	r, err := DiscoverFiles(paths, version, builtins...)
	if r != nil {
		r.Diagnostics = append(diagnostics, r.Diagnostics...)
	}
	return r, err
}

// Files enumerates manual directories followed by the current managed directory.
func Files(config, home, cwd, managed string) (paths, diagnostics []string, err error) {
	explicit := config != ""
	dirs := []string{filepath.Join(home, ".skgo", "plugins")}
	if explicit {
		dirs = strings.Split(config, ",")
	}
	if managed != "" {
		dirs = append(dirs, managed)
	}
	seenDirs, seenFiles := map[string]bool{}, map[string]bool{}
	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			return nil, nil, fmt.Errorf("SKGO_PLUGIN_DIRS contains an empty directory")
		}
		if strings.HasPrefix(dir, "~/") {
			dir = filepath.Join(home, dir[2:])
		}
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(cwd, dir)
		}
		canonical, err := filepath.EvalSymlinks(dir)
		if os.IsNotExist(err) && (!explicit || dir == managed) {
			continue
		}
		if err != nil {
			return nil, nil, fmt.Errorf("plugin directory %s: %w", dir, err)
		}
		if seenDirs[canonical] {
			continue
		}
		seenDirs[canonical] = true
		files, err := os.ReadDir(canonical)
		if err != nil {
			return nil, nil, fmt.Errorf("plugin directory %s: %w", canonical, err)
		}
		for _, f := range files {
			if f.IsDir() || filepath.Ext(f.Name()) != ".so" {
				continue
			}
			name, err := filepath.EvalSymlinks(filepath.Join(canonical, f.Name()))
			if err != nil {
				diagnostics = append(diagnostics, fmt.Sprintf("plugin %s: %v", f.Name(), err))
				continue
			}
			if seenFiles[name] {
				continue
			}
			seenFiles[name] = true
			paths = append(paths, name)
		}
	}
	return paths, diagnostics, nil
}

// DiscoverFiles loads an explicit ordered file set in the current process.
func DiscoverFiles(paths []string, version string, builtins ...templateapi.Plugin) (*Registry, error) {
	r := &Registry{}
	for _, p := range builtins {
		d := p.Describe()
		if err := validate(d); err != nil {
			return nil, fmt.Errorf("built-in plugin: %w", err)
		}
		r.Entries = append(r.Entries, Entry{Path: "built-in:" + d.ID, Plugin: p, Descriptor: d})
	}
	seenFiles := map[string]bool{}
	for _, name := range paths {
		canonical, err := filepath.EvalSymlinks(name)
		if err != nil {
			r.Diagnostics = append(r.Diagnostics, fmt.Sprintf("plugin %s: %v", name, err))
			continue
		}
		name = canonical
		if seenFiles[name] {
			continue
		}
		seenFiles[name] = true
		p, d, err := open(name)
		if err == nil {
			err = validate(d)
		}
		if err == nil && d.SkgoVersion != version {
			err = fmt.Errorf("requires skgo %s; executing host is %s", d.SkgoVersion, version)
		}
		if err != nil {
			var loaded []string
			for _, e := range r.Entries {
				loaded = append(loaded, e.Path)
			}
			r.Diagnostics = append(r.Diagnostics, fmt.Sprintf("plugin %s: %v; build for this skgo host; align shared dependencies or remove the conflicting manual file / current-host managed installation with skgo plugin remove (already loaded: %s)", name, err, strings.Join(loaded, ", ")))
			continue
		}
		r.Entries = append(r.Entries, Entry{Path: name, Plugin: p, Descriptor: d})
	}
	ids := map[string][]string{}
	for _, e := range r.Entries {
		ids[e.Descriptor.ID] = append(ids[e.Descriptor.ID], e.Path)
	}
	var invalid []string
	for id, paths := range ids {
		if len(paths) > 1 {
			invalid = append(invalid, id)
		}
	}
	sort.Strings(invalid)
	for _, id := range invalid {
		r.Diagnostics = append(r.Diagnostics, fmt.Sprintf("duplicate plugin ID %q: %s; remove a copy; use skgo plugin remove MODULE for a managed installation", id, strings.Join(ids[id], ", ")))
	}
	valid := r.Entries[:0]
	for _, e := range r.Entries {
		if len(ids[e.Descriptor.ID]) == 1 {
			valid = append(valid, e)
		}
	}
	r.Entries = valid
	return r, nil
}
