// Command skgo-adapter-changed reports whether the npm package this tree would
// publish differs from the newest one already on the registry, so a release
// whose adapter is byte-for-byte the last one publishes no new version of it.
//
// It prints "changed" or "unchanged". Both packages are made the same way —
// this tree stamped with the registry's version and packed by npm, against the
// registry's own tarball — so the only differences left are real ones, and
// they are listed on stderr.
//
// Skipping is safe because of how skgo names the adapter it pairs with: the
// newest version at or below its own (adapter.RegistrySpec). As long as every
// change to the package is published at the release that carries it, that
// version holds exactly the files this release would have published.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tylergannon/skgo/internal/adapter"
)

func main() {
	dir := flag.String("dir", "internal/adapter", "the package root this tree would publish")
	flag.Parse()
	if flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}
	changed, err := run(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "skgo-adapter-changed:", err)
		os.Exit(1)
	}
	if changed {
		fmt.Println("changed")
	} else {
		fmt.Println("unchanged")
	}
}

func run(dir string) (bool, error) {
	latest, err := latestVersion()
	if err != nil {
		return false, err
	}
	if latest == "" {
		fmt.Fprintf(os.Stderr, "%s has never been published\n", adapter.Package)
		return true, nil
	}

	tmp, err := os.MkdirTemp("", "skgo-adapter-changed")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(tmp)

	candidate, err := packTree(dir, latest, tmp)
	if err != nil {
		return false, err
	}
	published, err := pack(tmp, adapter.Package+"@"+latest)
	if err != nil {
		return false, err
	}

	differences, err := compare(candidate, published)
	if err != nil {
		return false, err
	}
	if len(differences) == 0 {
		fmt.Fprintf(os.Stderr, "the package is identical to %s@%s\n", adapter.Package, latest)
		return false, nil
	}
	fmt.Fprintf(os.Stderr, "the package differs from %s@%s:\n", adapter.Package, latest)
	for _, d := range differences {
		fmt.Fprintln(os.Stderr, "\t"+d)
	}
	return true, nil
}

// latestVersion is the registry's newest version of the package, or "" when
// it has none. Any other failure is an error: a release that cannot tell
// whether the package changed must not guess.
func latestVersion() (string, error) {
	out, err := exec.Command("npm", "view", adapter.Package, "version", "--json").CombinedOutput()
	if err != nil {
		if bytes.Contains(out, []byte("E404")) {
			return "", nil
		}
		return "", fmt.Errorf("npm view %s: %v\n%s", adapter.Package, err, out)
	}
	var version string
	if err := json.Unmarshal(out, &version); err != nil || version == "" {
		return "", fmt.Errorf("npm view %s answered %q", adapter.Package, out)
	}
	return version, nil
}

// packTree packs a copy of the package root stamped with version, the same
// stamp the release gives it, so the version field is not a difference.
func packTree(dir, version, tmp string) ([]byte, error) {
	root := filepath.Join(tmp, "tree")
	if err := os.CopyFS(root, os.DirFS(dir)); err != nil {
		return nil, fmt.Errorf("copying %s: %w", dir, err)
	}
	stamp := exec.Command("npm", "pkg", "set", "version="+version)
	stamp.Dir = root
	if out, err := stamp.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("npm pkg set version=%s: %v\n%s", version, err, out)
	}
	return pack(tmp, root)
}

// pack runs `npm pack` on what — a directory or a registry spec — and returns
// the tarball.
func pack(tmp, what string) ([]byte, error) {
	dest, err := os.MkdirTemp(tmp, "pack")
	if err != nil {
		return nil, err
	}
	out, err := exec.Command("npm", "pack", what, "--json", "--pack-destination", dest).Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			out = append(out, exit.Stderr...)
		}
		return nil, fmt.Errorf("npm pack %s: %v\n%s", what, err, out)
	}
	var packed []struct {
		Filename string `json:"filename"`
	}
	if err := json.Unmarshal(out, &packed); err != nil || len(packed) != 1 {
		return nil, fmt.Errorf("npm pack %s answered %q", what, out)
	}
	return os.ReadFile(filepath.Join(dest, packed[0].Filename))
}

// compare lists every file one tarball has and the other lacks, and every
// file whose content differs.
func compare(a, b []byte) ([]string, error) {
	left, err := files(a)
	if err != nil {
		return nil, err
	}
	right, err := files(b)
	if err != nil {
		return nil, err
	}
	var differences []string
	for name, content := range left {
		other, ok := right[name]
		switch {
		case !ok:
			differences = append(differences, "only in this tree: "+name)
		case !bytes.Equal(content, other):
			differences = append(differences, "changed: "+name)
		}
	}
	for name := range right {
		if _, ok := left[name]; !ok {
			differences = append(differences, "only on the registry: "+name)
		}
	}
	sort.Strings(differences)
	return differences, nil
}

// files reads a package tarball into its regular files by path.
func files(tgz []byte) (map[string][]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(tgz))
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	r := tar.NewReader(gz)
	for {
		h, err := r.Next()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		if !h.FileInfo().Mode().IsRegular() {
			continue
		}
		content, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		out[strings.TrimPrefix(h.Name, "package/")] = content
	}
}
