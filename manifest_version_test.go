package skgo

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
)

// The adapter writes skgo.manifest.json and this package reads it: one contract
// in two languages, and until now nothing stopped an app from pairing an
// adapter it had copied by hand months earlier with a Go module that had moved
// on. The build failed later, somewhere else, saying something unrelated —
// `Could not resolve 'esbuild'` was the one that reached the field.
//
// So a manifest carries the identity of the adapter that wrote it, and a
// manifest from any other adapter is refused here, by name, before anything is
// served with it.

// buildFrom is a build directory holding nothing but a manifest saying which
// adapter wrote it.
func buildFrom(skgoVersion, adapterFingerprint string) fstest.MapFS {
	manifest := map[string]any{
		"appDir":  "_app",
		"base":    "",
		"version": "1737000000000",
		"routes":  []any{},
	}
	if skgoVersion != "" {
		manifest["skgo"] = skgoVersion
	}
	if adapterFingerprint != "" {
		manifest["skgoAdapter"] = adapterFingerprint
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		panic(err)
	}
	return fstest.MapFS{"skgo.manifest.json": &fstest.MapFile{Data: raw}}
}

// thisAdapter is the fingerprint of the adapter this module carries, computed
// here from the files on disk rather than asked of the code under test: an
// expectation read out of the thing being tested is satisfied by every wrong
// answer.
//
// The adapter is more than one file. What builds the app is skgo-adapter.js;
// what runs in the engine — the render entry, the `$app/server` it is compiled
// against, the polyfill that becomes its banner — is a file each beside it, and
// an app carrying last month's render entry is exactly as stale as one carrying
// last month's adapter. So all of them go into the name, each under its own
// slash-separated path, in sorted order.
func thisAdapter(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("internal", "adapter")
	sum := sha256.New()
	sum.Write(readForFingerprint(t, filepath.Join(dir, "skgo-adapter.js")))

	var names []string
	err := filepath.WalkDir(filepath.Join(dir, "skgo-adapter"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			names = append(names, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		t.Fatal("internal/adapter/skgo-adapter holds no files; the adapter's runtime JavaScript is missing")
	}
	sort.Strings(names)
	for _, name := range names {
		sum.Write([]byte(name + "\x00"))
		sum.Write(readForFingerprint(t, filepath.Join(dir, filepath.FromSlash(name))))
	}
	return hex.EncodeToString(sum.Sum(nil))[:12]
}

func readForFingerprint(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestAManifestFromAnotherSkgosAdapterIsRefused(t *testing.T) {
	// A version and a fingerprint no skgo has ever had: this build was made by
	// somebody else's adapter.
	const theirVersion = "v0.1.7"
	const theirAdapter = "0123456789ab"

	_, err := ReadManifest(buildFrom(theirVersion, theirAdapter))
	if err == nil {
		t.Fatal("a build made by another skgo's adapter was accepted")
	}
	// The message has to name both halves, because the developer's next move
	// depends on knowing which one is behind.
	for _, want := range []string{theirVersion, theirAdapter, thisAdapter(t), "go generate"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %q:\n%v", want, err)
		}
	}
}

func TestAManifestFromAnAdapterTooOldToSayIsRefused(t *testing.T) {
	// The hand-vendored adapters in the field stamp nothing at all, which is
	// exactly the case the check exists for.
	_, err := ReadManifest(buildFrom("", ""))
	if err == nil {
		t.Fatal("a build made by an adapter that records no version was accepted")
	}
	for _, want := range []string{thisAdapter(t), "go generate"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %q:\n%v", want, err)
		}
	}
}

func TestAManifestFromThisModulesAdapterIsRead(t *testing.T) {
	m, err := ReadManifest(buildFrom("devel", thisAdapter(t)))
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	if m.Version != "1737000000000" {
		t.Errorf("Manifest = %#v", m)
	}
}
