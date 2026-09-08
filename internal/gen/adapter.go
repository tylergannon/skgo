package gen

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tylergannon/skgo/internal/adapter"
)

// adapterFileName is what `vite.config.ts` imports. Kit's adapter is an
// ordinary module the vite config names, so the file has to sit in the vite
// root under the name the config expects.
const adapterFileName = "skgo-adapter.js"

// adapterFilesDir is the directory beside it holding the JavaScript that runs
// rather than the JavaScript that builds: the engine's entry point, the
// `$app/server` it is compiled against, the polyfill that becomes its banner,
// and the vite plugin declaring the environment kit builds it in. The adapter
// finds them relative to its own file.
const adapterFilesDir = "skgo-adapter"

// writeAdapter puts this module's adapter in the vite root, overwriting
// whatever is there.
//
// Overwriting is the point. The adapter used to be vendored by hand, and a
// copy that had fallen behind the Go it was paired with failed the build with
// an error about something else entirely — a missing esbuild import, in the
// one case that reached the field. An adapter that is written by the generator
// cannot be a different version than the Go that reads what it writes.
func writeAdapter(cfg Config) error {
	if err := write(cfg, filepath.Join(cfg.Web, adapterFileName), string(adapter.Source())); err != nil {
		return err
	}

	written := map[string]bool{}
	for name, content := range adapter.Files() {
		path := filepath.Join(cfg.Web, filepath.FromSlash(name))
		if err := write(cfg, path, string(content)); err != nil {
			return err
		}
		written[filepath.Base(path)] = true
	}

	// A file this skgo no longer carries has to go. The directory is the
	// adapter's, not the app's, and a module left behind by an older skgo is
	// still resolvable — an import that should have failed loudly would keep
	// building against a program nothing here writes any more.
	return pruneAdapterFiles(cfg, written)
}

func pruneAdapterFiles(cfg Config, keep map[string]bool) error {
	dir := filepath.Join(cfg.Web, adapterFilesDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var stale []string
	for _, entry := range entries {
		if entry.IsDir() || keep[entry.Name()] || !strings.HasSuffix(entry.Name(), ".js") {
			continue
		}
		stale = append(stale, entry.Name())
	}
	sort.Strings(stale)
	for _, name := range stale {
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			return err
		}
		cfg.Logf("removed %s", filepath.Join(dir, name))
	}
	return nil
}
