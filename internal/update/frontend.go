package update

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tylergannon/skgo/internal/toolchain"
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
	after, err := frontendFiles(stage)
	if err != nil {
		return err
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
