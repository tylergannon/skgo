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
	_ "embed"
	"encoding/hex"
	"fmt"
	"regexp"
	"runtime/debug"
	"sync"
)

//go:embed skgo-adapter.js
var source []byte

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

// Fingerprint identifies the adapter by its own bytes, before stamping. It is
// the half of the identity that is always meaningful: two skgo checkouts both
// call themselves devel, and the question a build manifest has to answer is
// whether the adapter that wrote it is the adapter this module carries.
func Fingerprint() string {
	fingerprintOnce.Do(func() {
		sum := sha256.Sum256(source)
		fingerprint = hex.EncodeToString(sum[:])[:12]
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
