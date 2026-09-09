package adapter

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The adapter is published to npm as sveltekit-adapter-skgo and embedded in this module,
// and the two have to be the same bytes. Not "the same source, built twice" —
// the same bytes, because the fingerprint the adapter stamps into every
// manifest is a hash of the files it finds on disk at build time, and the
// fingerprint the Go server checks it against is a hash of the files below.
// A package that shipped one file fewer would name itself something else and be
// refused by every binary it was installed beside.
//
// So this directory is the npm package root: there is no copy step to get
// wrong. What can still be wrong is package.json's `files` list, which decides
// what actually leaves for the registry, and nothing about editing a runtime
// file makes anyone remember it. These tests are what remembers.

// packageFiles is what a developer who runs `pnpm add -D sveltekit-adapter-skgo` must
// receive, written out here rather than read from the package: the entry vite
// imports, every runtime file the SSR build reaches for, the types the app's
// `vite.config.ts` is checked against, and the three files npm carries for any
// package.
var packageFiles = []string{
	"LICENSE",
	"README.md",
	"index.d.ts",
	"package.json",
	"skgo-adapter.js",
	"skgo-adapter/app-paths.js",
	"skgo-adapter/app-server.js",
	"skgo-adapter/entry.js",
	"skgo-adapter/env.js",
	"skgo-adapter/identity.js",
	"skgo-adapter/polyfill.js",
}

func TestThePublishedPackageHoldsExactlyWhatTheAdapterNeeds(t *testing.T) {
	tarball := pack(t)

	var got []string
	for name := range tarball {
		got = append(got, name)
	}
	slices.Sort(got)

	if !slices.Equal(got, packageFiles) {
		t.Fatalf("`pnpm pack` would publish:\n  %s\nthe adapter needs:\n  %s\n"+
			"package.json's `files` decides this.",
			strings.Join(got, "\n  "), strings.Join(packageFiles, "\n  "))
	}
}

func TestThePublishedPackageIsTheAdapterThisModuleEmbeds(t *testing.T) {
	tarball := pack(t)

	embedded := map[string]bool{}
	err := fs.WalkDir(files, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		embedded[name] = true

		mine, err := fs.ReadFile(files, name)
		if err != nil {
			return err
		}
		theirs, ok := tarball[name]
		if !ok {
			t.Errorf("the Go binary embeds %s and the published package does not carry it; "+
				"every build made with that package would be refused at startup", name)
			return nil
		}
		if !bytes.Equal(mine, theirs) {
			t.Errorf("%s differs between the tarball and the copy this module embeds "+
				"(%d bytes published, %d embedded)", name, len(theirs), len(mine))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(embedded) == 0 {
		t.Fatal("this module embeds no adapter files at all")
	}

	// The other direction: a runtime file that ships but is not embedded would
	// be one the fingerprint on the app's side covers and the one on this side
	// does not, which is the same refusal read backwards.
	for name := range tarball {
		if strings.HasPrefix(name, "skgo-adapter") && !embedded[name] {
			t.Errorf("the published package carries %s and this module does not embed it", name)
		}
	}
}

// TestThePublishedPackageDeclaresWhatAnAdapterDeclares holds the package to the
// shape every adapter a SvelteKit developer has installed already has: kit as a
// peer dependency rather than a hard one, an exports map so nothing imports a
// path, and types beside it.
func TestThePublishedPackageDeclaresWhatAnAdapterDeclares(t *testing.T) {
	var pkg struct {
		Name             string            `json:"name"`
		Type             string            `json:"type"`
		Types            string            `json:"types"`
		License          string            `json:"license"`
		Exports          map[string]any    `json:"exports"`
		PeerDependencies map[string]string `json:"peerDependencies"`
		Dependencies     map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal(pack(t)["package.json"], &pkg); err != nil {
		t.Fatal(err)
	}

	if pkg.Name != Package {
		t.Errorf("the package publishes as %q; the Go that looks for it in node_modules looks for %q",
			pkg.Name, Package)
	}
	if pkg.Type != "module" || pkg.Types != "index.d.ts" || pkg.License == "" {
		t.Errorf("package.json = %+v; want an ESM package with types and a license", pkg)
	}
	if _, ok := pkg.Exports["."]; !ok {
		t.Errorf("the package has no `.` export, so `import skgo from 'sveltekit-adapter-skgo'` resolves nothing")
	}
	if pkg.PeerDependencies["@sveltejs/kit"] == "" {
		t.Errorf("the adapter does not declare @sveltejs/kit as a peer dependency; every kit adapter does")
	}
	// The adapter builds with the app's vite, kit and Svelte, deliberately: a
	// second copy of any of them is a second module realm, and kit's own
	// `instanceof` checks do not recognise it.
	if len(pkg.Dependencies) != 0 {
		t.Errorf("the adapter declares dependencies %v; it must build with the app's own", pkg.Dependencies)
	}
}

// pack publishes the package the way the release workflow will, and reads back
// what actually left: `pnpm pack` obeys package.json's `files` exactly as `npm
// publish` does, so the tarball is the artifact and not a description of one.
func pack(t *testing.T) map[string][]byte {
	t.Helper()
	if _, err := exec.LookPath("pnpm"); err != nil {
		t.Fatalf("pnpm is not on PATH, so this test cannot run — which is not the same as passing: %v", err)
	}
	dest := t.TempDir()
	cmd := exec.Command("pnpm", "pack", "--pack-destination", dest)
	cmd.Dir = "."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("pnpm pack: %v\n%s", err, out)
	}

	entries, err := filepath.Glob(filepath.Join(dest, "*.tgz"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("pnpm pack wrote %v (%v); want one tarball", entries, err)
	}
	return untar(t, entries[0])
}

// untar reads a npm tarball, whose every member sits under `package/`.
func untar(t *testing.T, name string) map[string][]byte {
	t.Helper()
	raw, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	zipped, err := gzip.NewReader(raw)
	if err != nil {
		t.Fatal(err)
	}

	out := map[string][]byte{}
	archive := tar.NewReader(zipped)
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		rel, ok := strings.CutPrefix(path.Clean(header.Name), "package/")
		if !ok {
			t.Fatalf("the tarball holds %s, which is not under package/", header.Name)
		}
		body, err := io.ReadAll(archive)
		if err != nil {
			t.Fatal(err)
		}
		out[rel] = body
	}
	return out
}
