// Package webbuild keeps the ordinary frontend build incremental.
package webbuild

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type Runner func(string, []string, string, ...string) ([]byte, error)

// Build checks inputs and built bytes before reusing a frontend. Generation runs
// only on a miss. Plugin generators run separately on an explicit official host.
func Build(root string, out io.Writer, run Runner) error {
	web := filepath.Join(root, "web")
	cache := filepath.Join(root, ".skgo", "build.json")
	key, err := fingerprint(root, run)
	if err != nil {
		return err
	}
	var saved struct{ Inputs, Outputs, Install, Source string }
	if data, err := os.ReadFile(cache); err == nil {
		_ = json.Unmarshal(data, &saved)
	}
	outputs, err := treeHash(filepath.Join(web, "build"), false)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	manifest, err := os.Stat(filepath.Join(web, "build/skgo.manifest.json"))
	manifestOK := err == nil && manifest.Mode().IsRegular() && manifest.Size() > 0
	importPath := filepath.Join(root, ".skgo", "import.json")
	// Release artifact restoration carries a producer receipt. Adopt it once,
	// only when its portable source and built bytes both match this checkout.
	var imported struct{ Inputs, Outputs, Install, Source string }
	if data, err := os.ReadFile(importPath); err == nil {
		if err := os.Remove(importPath); err != nil {
			return err
		}
		if json.Unmarshal(data, &imported) == nil && imported.Source == key && imported.Outputs == outputs && manifestOK {
			saved = imported
			saved.Inputs = environmentKey(key)
			data, err := json.Marshal(saved)
			if err != nil {
				return err
			}
			if err := os.WriteFile(cache, data, 0644); err != nil {
				return err
			}
		}
	}
	if manifestOK && saved.Inputs == environmentKey(key) && saved.Outputs == outputs {
		fmt.Fprintln(out, "Frontend is current")
		return nil
	}
	install, err := installHash(web)
	if err != nil {
		return err
	}
	_, modulesErr := os.Stat(filepath.Join(web, "node_modules/.bin/vp"))
	if saved.Install != install || modulesErr != nil {
		if _, err := run(web, nil, "vp", "install"); err != nil {
			return err
		}
	}
	// Invalidate before changing outputs, including when a later command fails.
	if err := os.Remove(cache); err != nil && !os.IsNotExist(err) {
		return err
	}
	if _, err := run(root, nil, "go", "generate", "./..."); err != nil {
		return err
	}
	if _, err := run(web, nil, filepath.Join(web, "node_modules/.bin/vp"), "fmt"); err != nil {
		return err
	}
	// Capture after generators/formatting, before compilation. An edit during
	// compilation must not mark the old result current for the edited source.
	compiledInputs, err := fingerprint(root, run)
	if err != nil {
		return err
	}
	keepPath := filepath.Join(web, "build/.gitkeep")
	keep, keepErr := os.ReadFile(keepPath)
	if _, err := run(web, nil, filepath.Join(web, "node_modules/.bin/vp"), "build"); err != nil {
		return err
	}
	if keepErr == nil {
		if err := os.WriteFile(keepPath, keep, 0644); err != nil {
			return err
		}
	}
	manifest, err = os.Stat(filepath.Join(web, "build/skgo.manifest.json"))
	if err != nil {
		return err
	}
	if !manifest.Mode().IsRegular() || manifest.Size() == 0 {
		return fmt.Errorf("frontend build produced no manifest")
	}
	// Generation and dependency installation may have changed their own inputs.
	saved.Source, err = fingerprint(root, run)
	if err != nil {
		return err
	}
	if saved.Source != compiledInputs {
		return fmt.Errorf("build inputs changed during frontend compilation; run the build again")
	}
	saved.Inputs = environmentKey(saved.Source)
	saved.Outputs, err = treeHash(filepath.Join(web, "build"), false)
	if err != nil {
		return err
	}
	saved.Install, err = installHash(web)
	if err != nil {
		return err
	}
	data, err := json.Marshal(saved)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cache), 0755); err != nil {
		return err
	}
	return os.WriteFile(cache, data, 0644)
}

func fingerprint(root string, run Runner) (string, error) {
	h := sha256.New()
	io.WriteString(h, "skgo-web-build-v2\x00")
	inputs, err := treeHash(root, true)
	if err != nil {
		return "", err
	}
	io.WriteString(h, inputs)
	// Module identities include tool updates; local replacements need content
	// checks too because their version does not change while editing.
	data, err := run(root, nil, "go", "list", "-m", "-json", "all")
	if err != nil {
		return "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	for {
		var m struct {
			Main                              bool
			Path, Version, Sum, GoModSum, Dir string
			Replace                           *struct{ Path, Dir, Version, Sum, GoModSum string }
		}
		if err := decoder.Decode(&m); err == io.EOF {
			break
		} else if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00", m.Path, m.Version, m.Sum, m.GoModSum)
		if m.Replace != nil {
			fmt.Fprintf(h, "%s\x00%s\x00%s\x00", m.Replace.Version, m.Replace.Sum, m.Replace.GoModSum)
		}
		if m.Main && m.Dir != "" && filepath.Clean(m.Dir) != filepath.Clean(root) {
			digest, err := treeHash(m.Dir, true)
			if err != nil {
				return "", err
			}
			io.WriteString(h, digest)
		}
		if m.Replace != nil && m.Replace.Version == "" && m.Replace.Dir != "" {
			digest, err := treeHash(m.Replace.Dir, true)
			if err != nil {
				return "", err
			}
			io.WriteString(h, digest)
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func installHash(web string) (string, error) {
	h := sha256.New()
	for _, name := range []string{"package.json", "pnpm-lock.yaml", "package-lock.json", "yarn.lock", "vp.config.ts", "pnpm-workspace.yaml", ".npmrc"} {
		data, err := os.ReadFile(filepath.Join(web, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		io.WriteString(h, name+"\x00")
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func treeHash(root string, inputs bool) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if inputs && (d.Name() == ".git" || d.Name() == ".gimbal") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if inputs && d.IsDir() && path != root {
			normalized := filepath.ToSlash(rel)
			if normalized == "web/build" || normalized == "native/build" || strings.HasSuffix(normalized, "/web/build") || strings.HasSuffix(normalized, "/native/build") {
				return filepath.SkipDir
			}
			switch d.Name() {
			case ".git", ".skgo", "node_modules", ".svelte-kit", "bin", "dist", "tmp", "ephemeral", ".cache", ".claude", ".tools", "docs", "docs-site", "e2e", "DerivedData", ".build", ".zig-cache", "zig-out":
				return filepath.SkipDir
			}
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			// Authored symlinks must include their target bytes in freshness.
			info, err := os.Stat(path)
			if os.IsNotExist(err) {
				target, err := os.Readlink(path)
				if err != nil {
					return err
				}
				io.WriteString(h, rel+"\x00"+target)
				return nil
			}
			if err != nil {
				return err
			}
			if info.IsDir() {
				return fmt.Errorf("build input directory symlink is unsupported: %s", path)
			}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", rel, len(data))
		h.Write(data)
		return nil
	})
	return hex.EncodeToString(h.Sum(nil)), err
}

// Local reuse remains sensitive to the full environment. Imported release assets
// deliberately retain the producer's build, then become bound to this process.
func environmentKey(source string) string {
	h := sha256.New()
	io.WriteString(h, source+"\x00"+runtime.Version()+"\x00")
	env := os.Environ()
	sort.Strings(env)
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		switch name {
		case "PWD", "OLDPWD", "SHLVL", "_":
			continue
		}
		io.WriteString(h, entry+"\x00")
	}
	return hex.EncodeToString(h.Sum(nil))
}
