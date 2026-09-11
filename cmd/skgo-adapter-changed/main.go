// Command skgo-adapter-changed reports whether the npm package this tree would
// publish differs from the newest one already on the registry, so a release
// whose adapter is byte-for-byte the last one publishes no new version of it.
//
// It prints "changed" or "unchanged", and lists the differences on stderr. The
// published tarball comes straight from the registry and is checked against
// its integrity hash; it is compared with the files the tree would publish,
// ignoring only the version a release stamps into package.json. Nothing here
// runs npm.
//
// Skipping is safe because of how skgo names the adapter it pairs with: the
// newest version at or below its own (adapter.RegistrySpec). As long as every
// change to the package is published at the release that carries it, that
// version holds exactly the files this release would have published.
//
// With -stamp VERSION it instead writes VERSION into the tree's package.json,
// which is how a release names what it publishes.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/tylergannon/skgo/internal/adapter"
)

const registry = "https://registry.npmjs.org/"

func main() {
	dir := flag.String("dir", "internal/adapter", "the package root this tree would publish")
	stamp := flag.String("stamp", "", "write this version into the package root's package.json instead")
	flag.Parse()
	if flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}
	if *stamp != "" {
		if err := stampVersion(*dir, strings.TrimPrefix(*stamp, "v")); err != nil {
			fail(err)
		}
		return
	}
	changed, err := run(*dir)
	if err != nil {
		fail(err)
	}
	if changed {
		fmt.Println("changed")
	} else {
		fmt.Println("unchanged")
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "skgo-adapter-changed:", err)
	os.Exit(1)
}

func run(dir string) (bool, error) {
	latest, err := latestPublished()
	if err != nil {
		return false, err
	}
	if latest == nil {
		fmt.Fprintf(os.Stderr, "%s has never been published\n", adapter.Package)
		return true, nil
	}
	published, err := latest.files()
	if err != nil {
		return false, err
	}
	tree, err := treeFiles(dir)
	if err != nil {
		return false, err
	}
	differences, err := compare(tree, published)
	if err != nil {
		return false, err
	}
	if len(differences) == 0 {
		fmt.Fprintf(os.Stderr, "the package is identical to %s@%s\n", adapter.Package, latest.Version)
		return false, nil
	}
	fmt.Fprintf(os.Stderr, "the package differs from %s@%s:\n", adapter.Package, latest.Version)
	for _, d := range differences {
		fmt.Fprintln(os.Stderr, "\t"+d)
	}
	return true, nil
}

type release struct {
	Version string
	Dist    struct {
		Tarball   string `json:"tarball"`
		Integrity string `json:"integrity"`
	} `json:"dist"`
}

var client = &http.Client{Timeout: time.Minute}

// latestPublished is the registry's newest version of the package, or nil when
// it has none. Any other failure is an error: a release that cannot tell
// whether the package changed must not guess.
func latestPublished() (*release, error) {
	req, err := http.NewRequest(http.MethodGet, registry+strings.Replace(adapter.Package, "/", "%2f", 1), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.npm.install-v1+json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the registry answered %s for %s", resp.Status, adapter.Package)
	}
	var doc struct {
		DistTags map[string]string  `json:"dist-tags"`
		Versions map[string]release `json:"versions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, fmt.Errorf("reading the registry's answer for %s: %w", adapter.Package, err)
	}
	version := doc.DistTags["latest"]
	r, ok := doc.Versions[version]
	if version == "" || !ok || r.Dist.Tarball == "" {
		return nil, fmt.Errorf("the registry names no latest tarball for %s", adapter.Package)
	}
	r.Version = version
	return &r, nil
}

// files downloads the release's tarball, refuses it unless it matches the
// integrity hash the registry published, and reads it into files by path.
func (r *release) files() (map[string][]byte, error) {
	resp, err := client.Get(r.Dist.Tarball)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading %s: %s", r.Dist.Tarball, resp.Status)
	}
	tgz, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	sum := sha512.Sum512(tgz)
	if want := "sha512-" + base64.StdEncoding.EncodeToString(sum[:]); r.Dist.Integrity != want {
		return nil, fmt.Errorf("%s does not match its integrity %s", r.Dist.Tarball, r.Dist.Integrity)
	}
	return untar(tgz)
}

// treeFiles is what publishing the package root would put in the tarball:
// every path its package.json `files` names, and the package.json, README and
// licence npm always carries.
func treeFiles(dir string) (map[string][]byte, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return nil, err
	}
	var pkg struct {
		Files []string `json:"files"`
	}
	if err := json.Unmarshal(raw, &pkg); err != nil {
		return nil, fmt.Errorf("parsing %s/package.json: %w", dir, err)
	}
	root := os.DirFS(dir)
	out := map[string][]byte{"package.json": raw}
	add := func(name string) error {
		content, err := fs.ReadFile(root, name)
		if err != nil {
			return err
		}
		out[name] = content
		return nil
	}
	for _, entry := range pkg.Files {
		if strings.ContainsAny(entry, "*?[]{}!") {
			return nil, fmt.Errorf("package.json names %q in files, and this command reads only plain paths", entry)
		}
		name := path.Clean(strings.TrimPrefix(entry, "./"))
		err := fs.WalkDir(root, name, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			return add(p)
		})
		if err != nil {
			return nil, fmt.Errorf("package.json names %q in files: %w", entry, err)
		}
	}
	entries, err := fs.ReadDir(root, ".")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		upper := strings.ToUpper(e.Name())
		if !e.IsDir() && (strings.HasPrefix(upper, "README") || strings.HasPrefix(upper, "LICENSE") || strings.HasPrefix(upper, "LICENCE")) {
			if err := add(e.Name()); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// compare lists every file one side has and the other lacks, and every file
// whose content differs. package.json is compared as JSON without its version,
// which is the one thing a release writes into it.
func compare(tree, published map[string][]byte) ([]string, error) {
	var differences []string
	for name, content := range tree {
		other, ok := published[name]
		switch {
		case !ok:
			differences = append(differences, "only in this tree: "+name)
		case name == "package.json":
			same, err := sameManifest(content, other)
			if err != nil {
				return nil, err
			}
			if !same {
				differences = append(differences, "changed: "+name)
			}
		case !bytes.Equal(content, other):
			differences = append(differences, "changed: "+name)
		}
	}
	for name := range published {
		if _, ok := tree[name]; !ok {
			differences = append(differences, "only on the registry: "+name)
		}
	}
	sort.Strings(differences)
	return differences, nil
}

func sameManifest(a, b []byte) (bool, error) {
	var left, right map[string]any
	if err := json.Unmarshal(a, &left); err != nil {
		return false, err
	}
	if err := json.Unmarshal(b, &right); err != nil {
		return false, err
	}
	delete(left, "version")
	delete(right, "version")
	return reflect.DeepEqual(left, right), nil
}

// untar reads a package tarball into its regular files by path.
func untar(tgz []byte) (map[string][]byte, error) {
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

var versionField = regexp.MustCompile(`("version"\s*:\s*)"[^"]*"`)

// stampVersion rewrites the one version field in package.json and leaves every
// other byte of it as it was.
func stampVersion(dir, version string) error {
	file := filepath.Join(dir, "package.json")
	raw, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	if n := len(versionField.FindAllIndex(raw, -1)); n != 1 {
		return fmt.Errorf("%s has %d version fields; want exactly one", file, n)
	}
	stamped := versionField.ReplaceAll(raw, []byte(`${1}"`+version+`"`))
	return os.WriteFile(file, stamped, 0o644)
}
