package update

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/tylergannon/skgo/internal/toolchain"
	"gopkg.in/yaml.v3"
)

func frontendFiles(root string) (map[string][]byte, error) {
	result := map[string][]byte{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", ".git", ".svelte-kit", "build", ".cache", "coverage":
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("frontend source contains a symlink; resolve its ownership before automatic migration: %s", rel)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		result[filepath.ToSlash(rel)] = b
		return nil
	})
	return result, err
}
func copyFrontend(source, dest string) error {
	files, err := frontendFiles(source)
	if err != nil {
		return err
	}
	for name, b := range files {
		path := filepath.Join(dest, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(path, b, 0644); err != nil {
			return err
		}
	}
	return nil
}
func pinDependencies(root, adapter, addon, kit string) error {
	path := filepath.Join(root, "package.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var p map[string]json.RawMessage
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	sections := map[string]map[string]string{}
	for _, name := range []string{"dependencies", "devDependencies"} {
		v := map[string]string{}
		if raw := p[name]; raw != nil {
			if err := json.Unmarshal(raw, &v); err != nil {
				return err
			}
		}
		sections[name] = v
	}
	for name, version := range map[string]string{"vite-plus": toolchain.VitePlus, "@sveltejs/kit": kit, "@skgo/sveltekit-adapter": adapter} {
		section := "devDependencies"
		if _, ok := sections["dependencies"][name]; ok {
			section = "dependencies"
		}
		if name == "vite-plus" && strings.HasPrefix(sections[section][name], "catalog:") {
			continue // The existing-vp aligner updates the referenced catalog.
		}
		sections[section][name] = version
	}
	for name, v := range sections {
		if _, ok := v["@skgo/sv"]; ok {
			v["@skgo/sv"] = addon
		}
		p[name], err = json.Marshal(v)
		if err != nil {
			return err
		}
	}
	p["packageManager"], _ = json.Marshal("pnpm@" + toolchain.PNPM)
	engines := map[string]json.RawMessage{}
	if raw := p["devEngines"]; raw != nil {
		if err := json.Unmarshal(raw, &engines); err != nil {
			return err
		}
	}
	engines["packageManager"], _ = json.Marshal(map[string]string{"name": "pnpm", "version": toolchain.PNPM, "onFail": "error"})
	p["devEngines"], _ = json.Marshal(engines)
	b, err = json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0644)
}
func dependencyFile(name string) bool {
	switch name {
	case "package.json", "pnpm-lock.yaml", "pnpm-workspace.yaml":
		return true
	}
	return strings.HasPrefix(name, "patches/skgo-kit-") && strings.HasSuffix(name, ".patch")
}
func preservedFrontend(source, stage string) error {
	before, err := frontendFiles(source)
	if err != nil {
		return err
	}
	return preservedFiles(before, stage)
}

func preservedFiles(before map[string][]byte, stage string) error {
	after, err := frontendFiles(stage)
	if err != nil {
		return err
	}
	for _, name := range []string{"package.json", "pnpm-workspace.yaml"} {
		a, err := authoredConfig(name, before[name])
		if err != nil {
			return err
		}
		b, err := authoredConfig(name, after[name])
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(a, b) {
			return fmt.Errorf("VitePlus migration would change authored configuration in %s; application configuration is untouched", name)
		}
	}
	for name, b := range before {
		if !dependencyFile(name) && !bytes.Equal(b, after[name]) {
			return fmt.Errorf("VitePlus migration would change application source %s; migration is incomplete and the original file is untouched", name)
		}
	}
	for name := range after {
		if _, ok := before[name]; !ok && !dependencyFile(name) {
			return fmt.Errorf("VitePlus migration would add application source %s; review the migration before updating", name)
		}
	}
	return nil
}
func applyFrontend(root, stage string) error {
	files, err := frontendFiles(stage)
	if err != nil {
		return err
	}
	names := []string{}
	for name := range files {
		if dependencyFile(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if st, err := os.Lstat(path); err == nil && st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing dependency symlink %s", name)
		}
		f, err := os.CreateTemp(filepath.Dir(path), ".skgo-update-")
		if err != nil {
			return err
		}
		temp := f.Name()
		if _, err := f.Write(files[name]); err != nil {
			f.Close()
			os.Remove(temp)
			return err
		}
		if err := f.Close(); err != nil {
			os.Remove(temp)
			return err
		}
		if err := os.Chmod(temp, 0644); err != nil {
			os.Remove(temp)
			return err
		}
		if err := os.Rename(temp, path); err != nil {
			os.Remove(temp)
			return err
		}
	}
	return nil
}

// Remove only dependency selections owned by skgo/vp before comparing authored
// configuration. Scripts, runtime settings, unrelated overrides and packages
// must survive migration, even when they live in a dependency file.
func authoredConfig(name string, data []byte) (map[string]any, error) {
	p := map[string]any{}
	if len(data) > 0 {
		var err error
		if name == "package.json" {
			err = json.Unmarshal(data, &p)
		} else {
			err = yaml.Unmarshal(data, &p)
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
	}
	strip := func(parent map[string]any, key string, owned func(string, any) bool) {
		if m, ok := parent[key].(map[string]any); ok {
			for k, v := range m {
				if owned(k, v) {
					delete(m, k)
				}
			}
			if len(m) == 0 {
				delete(parent, key)
			}
		}
	}
	managed := func(k string, _ any) bool {
		switch k {
		case "vite-plus", "vite", "vitest", "@sveltejs/kit", "@skgo/sveltekit-adapter", "@skgo/sv":
			return true
		}
		return vitestSibling(k)
	}
	if name == "package.json" {
		delete(p, "packageManager")
		strip(p, "devEngines", func(k string, _ any) bool { return k == "packageManager" })
		strip(p, "dependencies", managed)
		strip(p, "devDependencies", managed)
		strip(p, "optionalDependencies", managed)
		strip(p, "peerDependencies", managed)
	} else {
		strip(p, "catalog", managed)
		if catalogs, ok := p["catalogs"].(map[string]any); ok {
			for name := range catalogs {
				strip(catalogs, name, managed)
			}
			if len(catalogs) == 0 {
				delete(p, "catalogs")
			}
		}
		strip(p, "overrides", managedOverride)
		strip(p, "patchedDependencies", func(k string, v any) bool {
			path, ok := v.(string)
			return ok && strings.HasPrefix(k, "@sveltejs/kit@") && strings.HasPrefix(path, "patches/skgo-kit-") && strings.HasSuffix(path, ".patch")
		})
	}
	return p, nil
}

func unchangedFrontend(root string, original map[string][]byte) error {
	current, err := frontendFiles(root)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(current, original) {
		return fmt.Errorf("frontend changed while update was staging; refusing to overwrite concurrent application edits")
	}
	return nil
}

func managedOverride(k string, _ any) bool {
	return k == "vite" || k == "vite@*" || k == "vitest" || k == "vitest@*"
}
