package gen

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/tylergannon/skgo/internal/adapter"
)

// checkInstalledAdapter refuses to generate against a `sveltekit-adapter-skgo` that is
// not this module's.
//
// The unbypassable gate is at startup: the manifest the adapter writes names
// which adapter wrote it, and `skgo.ReadManifest` refuses any other. That check
// cannot be moved earlier, because a developer can swap `node_modules` after
// generating and only the binary reading its own fingerprint against what
// actually built catches that.
//
// This is the convenience in front of it. A mismatched install is the ordinary
// failure — a `pnpm install` that resolved an older release than the `go.mod`
// asks for — and the difference between learning that here, in a second, and
// learning it after a full frontend build and a server start is the whole cost
// of the mistake.
//
// An app with nothing installed yet is not a mismatch and is not reported: the
// vite build says `Cannot find package 'sveltekit-adapter-skgo'` perfectly well, and a
// `go generate` that refused to run before an install would be wrong about a
// tree that is merely in the wrong order.
func checkInstalledAdapter(cfg Config) error {
	dir := filepath.Join(cfg.Web, "node_modules", filepath.FromSlash(adapter.Package))

	installed, err := adapter.FingerprintOf(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("skgo: reading the %s installed in %s: %w", adapter.Package, cfg.Web, err)
	}
	if installed == adapter.Fingerprint() {
		return nil
	}

	// The version is only for the message, and only the installed half of it is
	// read from disk — a package.json that cannot be read still leaves a
	// fingerprint worth naming.
	version, _ := adapter.VersionOf(dir)

	// A checkout has no released version to name; the package it is paired
	// with is the one in its own tree. A released skgo names the exact npm
	// package version paired with its Go module version.
	install := "`pnpm add -D " + adapter.Package + "`"
	if v := adapter.Version(); v != "devel" {
		install = "`pnpm add -D " + adapter.Package + "@" + adapter.RegistrySpec(v) + "`"
	}

	return fmt.Errorf(
		"skgo: the %s installed in this app is not the one this skgo publishes.\n"+
			"\tinstalled in %s: %s\n"+
			"\tthis program:  %s\n"+
			"They are one contract in two languages, released together, so the frontend this "+
			"would build could not be served by the program that generated it.\n"+
			"Install the matching one — %s — and run `go generate ./...` again.",
		adapter.Package,
		dir,
		adapter.Identity(version, installed),
		adapter.Identity(adapter.Version(), adapter.Fingerprint()),
		install)
}
