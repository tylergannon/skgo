package hotreload

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/tylergannon/skgo/internal/gen"
	"golang.org/x/mod/modfile"
)

// HostCache holds what the successive hosts of one development session share,
// so a rebuild neither prepares the private toolchain again nor resolves the
// host's module requirements again. Dir must outlive every host compiled with
// the cache.
type HostCache struct {
	Dir string

	mu        sync.Mutex
	inspected string
	toolchain Toolchain
	modules   map[string]hostModules
}

// hostModules is a tidied host module file, with local replacements at their
// authored paths, and its checksums.
type hostModules struct{ mod, sum []byte }

func (c *HostCache) cachedToolchain(key string) (Toolchain, bool) {
	if c == nil {
		return Toolchain{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.toolchain, c.inspected == key
}

func (c *HostCache) keepToolchain(key string, t Toolchain) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.inspected, c.toolchain = key, t
}

// resolveModules gives a host view the application's module requirements plus
// the native loader's, tidied. The same toolchain, module files and packages
// resolve to the same requirements, so the cache reuses them.
func (c *HostCache) resolveModules(ctx context.Context, t Toolchain, inputs *gen.CompilerInputs, parent string, packages []string) (Toolchain, error) {
	var key string
	if c != nil {
		mod, err := os.ReadFile(filepath.Join(inputs.Dir, "go.mod"))
		if err != nil {
			return Toolchain{}, err
		}
		// Each revision materializes local replacements at new private paths.
		mod, err = relocate(mod, inputs.AuthoredPath)
		if err != nil {
			return Toolchain{}, err
		}
		sum, err := os.ReadFile(filepath.Join(inputs.Dir, "go.sum"))
		if err != nil && !os.IsNotExist(err) {
			return Toolchain{}, err
		}
		h := sha256.New()
		fmt.Fprintf(h, "%s\x00%d\x00%s\x00%d\x00%s\x00", t.Identity, len(mod), mod, len(sum), sum)
		for _, path := range packages {
			fmt.Fprintf(h, "%s\x00", path)
		}
		key = hex.EncodeToString(h.Sum(nil))
		c.mu.Lock()
		cached, ok := c.modules[key]
		c.mu.Unlock()
		if ok {
			mod, err := relocate(cached.mod, inputs.PrivatePath)
			if err != nil {
				return Toolchain{}, err
			}
			t.ModFile = filepath.Join(parent, "host.mod")
			if err := os.WriteFile(t.ModFile, mod, 0644); err != nil {
				return Toolchain{}, err
			}
			if cached.sum != nil {
				if err := os.WriteFile(filepath.Join(parent, "host.sum"), cached.sum, 0644); err != nil {
					return Toolchain{}, err
				}
			}
			return t, nil
		}
	}
	loaders := filepath.Join(parent, "native")
	if c != nil {
		loaders = filepath.Join(c.Dir, "native")
	}
	t, err := t.hostModules(ctx, inputs.Dir, parent, loaders)
	if err != nil {
		return Toolchain{}, err
	}
	// A workspace selects the loader through its private go.work instead.
	if t.ModFile == "" {
		return t, nil
	}
	if raw, e := t.command(ctx, inputs.Dir, "mod", "tidy").CombinedOutput(); e != nil {
		return Toolchain{}, fmt.Errorf("native host dependencies: %w\n%s", e, raw)
	}
	if c != nil {
		mod, err := os.ReadFile(t.ModFile)
		if err != nil {
			return Toolchain{}, err
		}
		if mod, err = relocate(mod, inputs.AuthoredPath); err != nil {
			return Toolchain{}, err
		}
		sum, err := os.ReadFile(filepath.Join(parent, "host.sum"))
		if err != nil && !os.IsNotExist(err) {
			return Toolchain{}, err
		}
		c.mu.Lock()
		if c.modules == nil {
			c.modules = map[string]hostModules{}
		}
		c.modules[key] = hostModules{mod, sum}
		c.mu.Unlock()
	}
	return t, nil
}

// relocate maps a module file's local replacement directories through path.
func relocate(data []byte, path func(string) string) ([]byte, error) {
	f, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return nil, err
	}
	var local []*modfile.Replace
	for _, r := range f.Replace {
		if r.New.Version == "" && filepath.IsAbs(r.New.Path) {
			local = append(local, r)
		}
	}
	for _, r := range local {
		if err := f.AddReplace(r.Old.Path, r.Old.Version, path(r.New.Path), ""); err != nil {
			return nil, err
		}
	}
	return modfile.Format(f.Syntax), nil
}
