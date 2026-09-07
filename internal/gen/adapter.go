package gen

import (
	"path/filepath"

	"github.com/tylergannon/skgo/internal/adapter"
)

// adapterFileName is what `vite.config.ts` imports. Kit's adapter is an
// ordinary module the vite config names, so the file has to sit in the vite
// root under the name the config expects.
const adapterFileName = "skgo-adapter.js"

// writeAdapter puts this module's adapter in the vite root, overwriting
// whatever is there.
//
// Overwriting is the point. The adapter used to be vendored by hand, and a
// copy that had fallen behind the Go it was paired with failed the build with
// an error about something else entirely — a missing esbuild import, in the
// one case that reached the field. An adapter that is written by the generator
// cannot be a different version than the Go that reads what it writes.
func writeAdapter(cfg Config) error {
	return write(cfg, filepath.Join(cfg.Web, adapterFileName), string(adapter.Source()))
}
