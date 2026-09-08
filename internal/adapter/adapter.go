// Package adapter carries skgo's SvelteKit adapter — the JavaScript half of
// the contract — and stamps it with the identity of the skgo it came from.
//
// The adapter writes `skgo.manifest.json` and the Go in this same module reads
// it, so the two are one contract in two languages. They used to be two files
// in two repositories: the adapter was vendored into each app by hand, and an
// app that had moved its Go on could still be building with an adapter from
// months earlier. That failure does not announce itself as a version skew —
// the one seen in the field surfaced as `Could not resolve 'esbuild'`.
//
// So the adapter is generated now. `skgo generate` writes it into the vite
// root from this embedded copy on every run, which makes it impossible for the
// adapter and the Go that reads its output to be different versions, and every
// manifest records which skgo wrote it so a stale copy is refused by name.
package adapter

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"runtime/debug"
	"sort"
	"sync"
)

//go:embed skgo-adapter.js
var source []byte

// The runtime JavaScript, as files rather than as strings inside the adapter:
// the entry the engine's bundle is built from, the `$app/server` it is built
// against, the polyfill that becomes its banner, and the vite plugin that
// declares the environment kit builds it in. Kit's own adapters carry theirs
// the same way, in a `files/` directory located from `import.meta.url`; skgo's
// adapter is written into the vite root, so its files go in a directory beside
// it and it finds them relative to itself.
//
//go:embed skgo-adapter
var files embed.FS

// filesDir is both the directory in this package and the directory the adapter
// looks for its runtime files in, relative to wherever it was written.
const filesDir = "skgo-adapter"

// module is the module path the version is looked up under.
const module = "github.com/tylergannon/skgo"

// stamp is the line the adapter declares its own identity on. `skgo generate`
// replaces it wholesale; the file in this package carries the unstamped
// spelling, which is also what the fingerprint is taken over.
var stamp = regexp.MustCompile(`(?m)^const SKGO = .*$`)

// Source is the adapter as it should be written into a vite root: this
// package's copy with its identity stamped in.
func Source() []byte {
	if !stamp.Match(source) {
		// An adapter that went out unstamped would be refused by every server
		// that read its manifest, for a reason nobody could act on.
		panic("skgo: internal/adapter/skgo-adapter.js has no `const SKGO = ...` line to stamp")
	}
	// Single quotes: the file goes through the app's prettier and svelte-check
	// like the rest of its source, and this line should look like every other
	// line in it.
	stamped := stamp.ReplaceAll(source, fmt.Appendf(nil,
		"const SKGO = { version: '%s', adapter: '%s' };", Version(), Fingerprint()))
	return stamped
}

// Files is the adapter's runtime JavaScript, keyed by the path it should be
// written to relative to the vite root. The adapter resolves them relative to
// its own file, so the layout here is the layout an app gets.
func Files() map[string][]byte {
	out := map[string][]byte{}
	for _, name := range fileNames() {
		data, err := files.ReadFile(name)
		if err != nil {
			// Embedded: unreachable unless this package was built wrong.
			panic("skgo: reading the embedded adapter file " + name + ": " + err.Error())
		}
		out[name] = data
	}
	return out
}

// fileNames is every embedded runtime file, in a stable order, so that the
// fingerprint below is a fact about the bytes and not about directory order.
func fileNames() []string {
	var names []string
	err := fs.WalkDir(files, filesDir, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			names = append(names, name)
		}
		return nil
	})
	if err != nil {
		panic("skgo: walking the embedded adapter files: " + err.Error())
	}
	sort.Strings(names)
	return names
}

// Fingerprint identifies the adapter by its own bytes, before stamping. It is
// the half of the identity that is always meaningful: two skgo checkouts both
// call themselves devel, and the question a build manifest has to answer is
// whether the adapter that wrote it is the adapter this module carries.
//
// Every file the adapter is made of goes into it, not only the entry: the
// engine's own entry point and its `$app/server` moved out of the adapter into
// files beside it, and a fingerprint that did not cover them would call an app
// carrying last month's render entry current.
func Fingerprint() string {
	fingerprintOnce.Do(func() {
		sum := sha256.New()
		sum.Write(source)
		for _, name := range fileNames() {
			data, err := files.ReadFile(name)
			if err != nil {
				panic("skgo: reading the embedded adapter file " + name + ": " + err.Error())
			}
			// The name is hashed too, so that moving a file between paths
			// changes the fingerprint even when no byte of it does.
			sum.Write([]byte(path.Clean(name) + "\x00"))
			sum.Write(data)
		}
		fingerprint = hex.EncodeToString(sum.Sum(nil))[:12]
	})
	return fingerprint
}

var (
	fingerprint     string
	fingerprintOnce sync.Once
)

// Version is the released version of skgo this binary was built from, or
// "devel" for a binary built from a checkout — the same answer whether skgo is
// the main module (the `skgo` command) or a dependency (an app's server).
func Version() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "devel"
	}
	v := ""
	// The `skgo` command is skgo's own main module; an app's server carries it
	// as a dependency.
	if bi.Main.Path == module && bi.Main.Replace == nil {
		v = bi.Main.Version
	}
	for _, dep := range bi.Deps {
		if dep.Path != module {
			continue
		}
		// A replaced module reports the version its require line names, which
		// is whatever placeholder the replacing developer wrote — this
		// repository's own example says v0.0.0. The code being built is a
		// checkout, so it is devel, and saying so keeps the stamp the same in
		// every tree that builds from a checkout.
		if dep.Replace != nil {
			v = ""
			break
		}
		v = dep.Version
	}
	if v == "" || v == "(devel)" {
		return "devel"
	}
	return v
}

// Identity is how a skgo is named in a message a developer reads: the version,
// and the adapter that version carries.
func Identity(version, fingerprint string) string {
	if fingerprint == "" {
		return "an adapter too old to record which skgo it came from"
	}
	if version == "" {
		version = "an unnamed skgo"
	} else {
		version = "skgo " + version
	}
	return version + " (adapter " + fingerprint + ")"
}
