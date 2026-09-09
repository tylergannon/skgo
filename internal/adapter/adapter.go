// Package adapter carries skgo's SvelteKit adapter — the JavaScript half of
// the contract — and names it by its own bytes.
//
// The adapter writes `skgo.manifest.json` and the Go in this same module reads
// it, so the two are one contract in two languages. They used to be two files
// in two repositories: the adapter was vendored into each app by hand, and an
// app that had moved its Go on could still be building with an adapter from
// months earlier. That failure does not announce itself as a version skew —
// the one seen in the field surfaced as `Could not resolve 'esbuild'`.
//
// The JavaScript beside this file is the `sveltekit-adapter-skgo` npm package: this
// directory is its package root, so what a developer installs and what Go
// embeds are literally the same files. The adapter fingerprints itself at build
// time the same way Fingerprint does here, over the same bytes in the same
// order, and stamps the answer into every manifest it writes. Go refuses a
// manifest carrying any other fingerprint, which makes an installed package
// that has fallen behind the binary a named failure at startup rather than
// something unrelated later.
package adapter

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
)

// The adapter as files: the entry `vite.config.ts` imports, and beside it the
// JavaScript that runs rather than the JavaScript that builds — the entry the
// engine's bundle is built from, the `$app/server` it is built against, the
// polyfill that becomes its banner, the vite plugin that declares the
// environment kit builds it in, and the module that computes the identity
// above. Kit's own adapters carry theirs the same way, in a `files/` directory
// located from `import.meta.url`.
//
//go:embed skgo-adapter.js skgo-adapter
var files embed.FS

// entryFile is what `vite.config.ts` reaches through the package's `exports`,
// and filesDir is the directory of runtime files beside it. Both spellings are
// the package's own layout, so they are also the paths the fingerprint is taken
// over.
const (
	entryFile = "skgo-adapter.js"
	filesDir  = "skgo-adapter"
)

// Package is the npm package this directory publishes as.
const Package = "sveltekit-adapter-skgo"

// module is the module path the version is looked up under.
const module = "github.com/tylergannon/skgo"

// RegistrySpec is the exact npm version paired with a Go module release. Go
// tags carry a leading v, while npm package versions do not.
func RegistrySpec(version string) string {
	return strings.TrimPrefix(version, "v")
}

// Fingerprint identifies the adapter by its own bytes. It is the half of the
// identity that is always meaningful: two skgo checkouts both call themselves
// devel, and the question a build manifest has to answer is whether the adapter
// that wrote it is the adapter this module carries.
//
// Every file the adapter is made of goes into it, not only the entry: the
// engine's own entry point and its `$app/server` are files beside the entry,
// and a fingerprint that did not cover them would call an app carrying last
// month's render entry current.
func Fingerprint() string {
	fingerprintOnce.Do(func() {
		f, err := fingerprintFS(files)
		if err != nil {
			// Embedded: unreachable unless this package was built wrong.
			panic("skgo: fingerprinting the embedded adapter: " + err.Error())
		}
		fingerprint = f
	})
	return fingerprint
}

var (
	fingerprint     string
	fingerprintOnce sync.Once
)

// FingerprintOf names the adapter installed in dir — a `sveltekit-adapter-skgo` package
// in an app's `node_modules` — by the same bytes in the same order, so that it
// can be compared with Fingerprint.
func FingerprintOf(dir string) (string, error) {
	return fingerprintFS(os.DirFS(dir))
}

// VersionOf reads the published version out of an installed package's
// package.json.
func VersionOf(dir string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return "", err
	}
	var pkg struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(raw, &pkg); err != nil {
		return "", err
	}
	return pkg.Version, nil
}

// fingerprintFS is the one definition of the name: the entry's bytes, then each
// runtime file under its own slash-separated path in sorted order, and the
// leading twelve hex digits of the SHA-256 of all of it. The adapter's own
// identity.js computes the same thing over the same tree in JavaScript.
func fingerprintFS(fsys fs.FS) (string, error) {
	sum := sha256.New()
	entry, err := fs.ReadFile(fsys, entryFile)
	if err != nil {
		return "", err
	}
	sum.Write(entry)

	var names []string
	err = fs.WalkDir(fsys, filesDir, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			names = append(names, name)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(names)
	for _, name := range names {
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return "", err
		}
		// The name is hashed too, so that moving a file between paths changes
		// the fingerprint even when no byte of it does.
		sum.Write([]byte(path.Clean(name) + "\x00"))
		sum.Write(data)
	}
	return hex.EncodeToString(sum.Sum(nil))[:12], nil
}

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

// Polyfill is the engine's globals as a plain script: the classes kit compares
// against with `instanceof` and the two a render's own `event.fetch` builds,
// plus the webcontainer flag Svelte's no-AsyncLocalStorage path is gated on.
//
// A build makes it the SSR bundle's banner. Dev has no bundle, so Go runs it
// in a fresh runtime before the first module evaluates — which is the same
// position, and it has to be that position: the flag has to be set before any
// module reads it, or the first async component throws
// `async_local_storage_unavailable`.
func Polyfill() []byte {
	data, err := files.ReadFile(path.Join(filesDir, "polyfill.js"))
	if err != nil {
		// Embedded: unreachable unless this package was built wrong.
		panic("skgo: reading the embedded polyfill: " + err.Error())
	}
	return data
}
