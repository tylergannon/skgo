// Package plugininstall installs trusted source plugins for an actual skgo host.
package plugininstall

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tylergannon/skgo/internal/pluginbuild"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/sumdb/dirhash"
	modzip "golang.org/x/mod/zip"
)

type resolution struct {
	Path, Version, Sum, GoModSum string
	Origin                       *struct{ VCS, URL, Hash, Subdir string }
	Error                        string
}
type source struct {
	resolution
	Root, Package, Policy string
}

func parseReference(ref string) (path, query string, err error) {
	path, query, has := strings.Cut(ref, "@")
	if !has {
		query = "latest"
	}
	if err = module.CheckPath(path); err != nil {
		return "", "", err
	}
	if query == "" || strings.ContainsAny(query, "@ \t\r\n") {
		return "", "", fmt.Errorf("invalid module version query %q", query)
	}
	return path, query, nil
}
func manifest(root string) (string, error) {
	b, err := os.ReadFile(filepath.Join(root, "skgo-plugin.json"))
	if err != nil {
		return "", fmt.Errorf("read skgo-plugin.json: %w", err)
	}
	var m struct {
		Package string `json:"package"`
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err = d.Decode(&m); err != nil {
		return "", fmt.Errorf("skgo-plugin.json: %w", err)
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return "", fmt.Errorf("skgo-plugin.json must contain one object")
	}
	p := m.Package
	if p != "." && (!strings.HasPrefix(p, "./") || !filepath.IsLocal(strings.TrimPrefix(p, "./")) || filepath.ToSlash(filepath.Clean(p)) != strings.TrimPrefix(p, "./") || strings.ContainsAny(p, "\\*?[") || strings.Contains(p, "...")) {
		return "", fmt.Errorf("skgo-plugin.json package must be . or one clean ./relative/path")
	}
	return p, nil
}
func command(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
	c := pluginbuild.ChildCommand(ctx, name, args...)
	c.Dir = dir
	c.Env = append(os.Environ(), env...)
	b, err := c.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, b)
	}
	return b, nil
}
func fetch(ctx context.Context, ref, goVersion string, out io.Writer) (s source, cleanup func(), err error) {
	path, query, err := parseReference(ref)
	if err != nil {
		return s, nil, err
	}
	temp, err := os.MkdirTemp("", "skgo-plugin-source-")
	if err != nil {
		return s, nil, err
	}
	cleanup = func() { _ = os.RemoveAll(temp) }
	defer func() {
		if err != nil {
			cleanup()
		}
	}()
	// Resolve the installed exact Go tool first, so its own distribution is not
	// downloaded afresh into the disposable source module cache.
	goroot, err := command(ctx, temp, []string{"GOWORK=off", "GOTOOLCHAIN=" + goVersion, "GOFLAGS="}, "go", "env", "GOROOT")
	if err != nil {
		return s, cleanup, err
	}
	goexe := filepath.Join(strings.TrimSpace(string(goroot)), "bin", "go")
	env := []string{"GOWORK=off", "GOTOOLCHAIN=local", "GOPROXY=direct", "GOMODCACHE=" + filepath.Join(temp, "cache"), "GOFLAGS=-modcacherw"}
	fmt.Fprintf(out, "Resolve trusted source directly through canonical VCS: %s@%s (GOPROXY=direct).\n", path, query)
	policy, err := command(ctx, temp, env, goexe, "env", "-json", "GOSUMDB", "GONOSUMDB", "GOPRIVATE")
	if err != nil {
		return s, cleanup, err
	}
	var p map[string]string
	if err = json.Unmarshal(policy, &p); err != nil {
		return s, cleanup, err
	}
	s.Policy = "checksum database " + p["GOSUMDB"]
	if p["GOSUMDB"] == "off" {
		s.Policy = "caller GOSUMDB=off"
	} else if module.MatchPrefixPatterns(p["GONOSUMDB"], path) {
		s.Policy = "exempt by GONOSUMDB=" + p["GONOSUMDB"] + " (GOPRIVATE=" + p["GOPRIVATE"] + ")"
	}
	fmt.Fprintln(out, "Source checksum policy:", s.Policy)
	b, err := command(ctx, temp, env, goexe, "mod", "download", "-json", path+"@"+query)
	if err != nil {
		return s, cleanup, err
	}
	if err = json.Unmarshal(b, &s.resolution); err != nil {
		return s, cleanup, err
	}
	if s.Error != "" {
		return s, cleanup, fmt.Errorf("resolve plugin source: %s", s.Error)
	}
	o := s.Origin
	if s.Path != path || s.Version == "" || s.Sum == "" || s.GoModSum == "" || o == nil || o.VCS != "git" || o.URL == "" || o.Hash == "" || strings.HasPrefix(o.URL, "-") || (o.Subdir != "" && (!filepath.IsLocal(o.Subdir) || filepath.Clean(o.Subdir) != o.Subdir)) {
		return s, cleanup, fmt.Errorf("unsupported source: resolver did not supply canonical Git provenance, module hashes and subdirectory")
	}
	repo := filepath.Join(temp, "repo")
	if err = os.Mkdir(repo, 0755); err != nil {
		return s, cleanup, err
	}
	for _, args := range [][]string{{"init", "--quiet"}, {"remote", "add", "origin", o.URL}, {"fetch", "--quiet", "--depth=1", "origin", o.Hash}, {"checkout", "--quiet", "--detach", "FETCH_HEAD"}} {
		if _, err = command(ctx, repo, nil, "git", args...); err != nil {
			return s, cleanup, err
		}
	}
	head, err := command(ctx, repo, nil, "git", "rev-parse", "HEAD")
	if err != nil {
		return s, cleanup, err
	}
	if strings.TrimSpace(string(head)) != o.Hash {
		return s, cleanup, fmt.Errorf("checkout commit mismatch: want %s, got %s", o.Hash, head)
	}
	zipPath := filepath.Join(temp, "source.zip")
	z, err := os.Create(zipPath)
	if err != nil {
		return s, cleanup, err
	}
	err = modzip.CreateFromVCS(z, module.Version{Path: s.Path, Version: s.Version}, repo, o.Hash, o.Subdir)
	closeErr := z.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return s, cleanup, err
	}
	sum, err := dirhash.HashZip(zipPath, dirhash.Hash1)
	if err != nil {
		return s, cleanup, err
	}
	if sum != s.Sum {
		return s, cleanup, fmt.Errorf("source checksum mismatch: want %s, got %s", s.Sum, sum)
	}
	s.Root = filepath.Join(repo, o.Subdir)
	modBytes, err := os.ReadFile(filepath.Join(s.Root, "go.mod"))
	if err != nil {
		return s, cleanup, err
	}
	sum, err = dirhash.Hash1([]string{"go.mod"}, func(string) (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(modBytes)), nil })
	if err != nil {
		return s, cleanup, err
	}
	if sum != s.GoModSum {
		return s, cleanup, fmt.Errorf("go.mod checksum mismatch: want %s, got %s", s.GoModSum, sum)
	}
	m, err := modfile.Parse("go.mod", modBytes, nil)
	if err != nil {
		return s, cleanup, err
	}
	if m.Module == nil || m.Module.Mod.Path != path {
		return s, cleanup, fmt.Errorf("checked-out module declaration does not match %s", path)
	}
	s.Package, err = manifest(s.Root)
	if err != nil {
		return s, cleanup, err
	}
	fmt.Fprintf(out, "Resolved %s to %s@%s, Git %s %s; module and go.mod hashes match.\n", ref, s.Path, s.Version, o.URL, o.Hash)
	return s, cleanup, nil
}
