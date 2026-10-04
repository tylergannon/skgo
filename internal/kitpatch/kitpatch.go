// Package kitpatch configures the exact Kit compatibility patch required by
// declared prerender Inputs. It authors only files in the selected frontend
// project; pnpm owns workspace configuration serialization and installation.
package kitpatch

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tylergannon/skgo/internal/adapter"
	"gopkg.in/yaml.v3"
)

const (
	kitPackageName = "@sveltejs/kit"
	ownedPatchName = "skgo-kit-3.0.0-queue.patch"
	pnpmVersion    = "12.9.1"
)

type Options struct {
	Web       string
	Workspace string
	PNPM      string
	Out       io.Writer
	run       func(dir, name string, args ...string) ([]byte, error)
}

type project struct {
	web           string
	root          string
	packagePath   string
	packageBytes  []byte
	packageUpdate []byte
	metadata      adapter.KitQueueMetadata
	patch         []byte
	patchPath     string
	patched       map[string]string
	kitVersion    string
}

// Configure pins the selected app's existing Kit dependency, writes the
// packaged correction into that app, and merges its exact pnpm patch entry.
// It never installs dependencies or edits node_modules.
func Configure(options Options) error {
	p, err := prepare(options)
	if err != nil {
		return err
	}
	if err := checkInstalledIfPresent(p); err != nil {
		return err
	}
	if err := checkPatchDestination(p); err != nil {
		return err
	}
	if err := checkPatchMap(p); err != nil {
		return err
	}
	if err := writePatch(p); err != nil {
		return err
	}
	if !bytes.Equal(p.packageBytes, p.packageUpdate) {
		if err := atomicReplace(p.packagePath, p.packageUpdate); err != nil {
			return fmt.Errorf("write Kit 3.0.0 pin to %s: %w", p.packagePath, err)
		}
	}
	if _, configured := p.patched[patchKey(p)]; !configured {
		p.patched[patchKey(p)] = relativePatchValue(p)
		if err := setPatchedDependencies(options, p); err != nil {
			return err
		}
	}
	if options.Out != nil {
		fmt.Fprintf(options.Out, "Configured the Kit 3.0.0 prerender queue correction in %s.\n", p.root)
		fmt.Fprintf(options.Out, "Files authored: %s, %s, and %s.\n", p.packagePath, p.patchPath, filepath.Join(p.root, "pnpm-workspace.yaml"))
		fmt.Fprintf(options.Out, "Installation is still required: run `%s install --no-frozen-lockfile` from %s, then run `go tool skgo kit-patch --web %s --check` from the Go app root.\n", filepath.Join("node_modules", ".bin", "vp"), p.web, p.web)
	}
	return nil
}

// Verify checks project configuration and the actual resolved installed Kit
// bytes. A configured patch without a successful native install does not pass.
func Verify(options Options) error {
	p, err := prepare(options)
	if err != nil {
		return err
	}
	if p.kitVersion != p.metadata.Version {
		return fmt.Errorf("%s must be pinned directly to %s (found %q)", kitPackageName, p.metadata.Version, p.kitVersion)
	}
	if err := requirePatchDestination(p); err != nil {
		return err
	}
	if err := checkPatchMap(p); err != nil {
		return err
	}
	if err := requirePatchMap(p); err != nil {
		return err
	}
	if err := checkInstalledCorrected(p); err != nil {
		return err
	}
	if options.Out != nil {
		fmt.Fprintf(options.Out, "Verified %s %s and its installed prerender queue patch in %s.\n", kitPackageName, p.metadata.Version, p.web)
	}
	return nil
}

func prepare(options Options) (*project, error) {
	metadata, patch, err := adapter.KitQueueCompatibility()
	if err != nil {
		return nil, fmt.Errorf("load packaged Kit queue correction: %w", err)
	}
	if metadata.Package != kitPackageName || metadata.Version != "3.0.0" || metadata.QueuePath != "src/core/postbuild/queue.js" {
		return nil, fmt.Errorf("unsupported packaged Kit queue correction metadata: package=%q version=%q path=%q", metadata.Package, metadata.Version, metadata.QueuePath)
	}
	if digest(patch) != metadata.PatchSHA256 {
		return nil, fmt.Errorf("packaged Kit queue patch digest mismatch: got %s, metadata says %s", digest(patch), metadata.PatchSHA256)
	}
	if !strings.Contains(string(patch), "diff --git a/"+metadata.QueuePath+" b/"+metadata.QueuePath+"\n") {
		return nil, fmt.Errorf("packaged Kit queue patch does not target %s", metadata.QueuePath)
	}
	web, err := resolveWeb(options)
	if err != nil {
		return nil, err
	}
	packagePath := filepath.Join(web, "package.json")
	packageBytes, err := readRegularFile(packagePath)
	if err != nil {
		return nil, fmt.Errorf("read frontend package manifest: %w", err)
	}
	_, currentVersion, update, err := pinKit(packageBytes, metadata.Version)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(web, "patches", ownedPatchName)
	patched, err := getPatchedDependencies(options, web)
	if err != nil {
		return nil, err
	}
	p := &project{
		web: web, root: web, packagePath: packagePath, packageBytes: packageBytes,
		packageUpdate: update, metadata: metadata, patch: patch, patchPath: path,
		patched: patched, kitVersion: currentVersion,
	}
	if value, configured := patched[patchKey(p)]; configured && sameConfiguredPath(p.root, value, p.patchPath) {
		if value != relativePatchValue(p) {
			return nil, fmt.Errorf("existing %s entry must use the portable relative path %q; found %q", patchKey(p), relativePatchValue(p), value)
		}
	}
	return p, nil
}

func resolveWeb(options Options) (string, error) {
	webArg := options.Web
	if webArg == "" {
		webArg = "web"
	}
	web, err := filepath.Abs(webArg)
	if err != nil {
		return "", fmt.Errorf("resolve frontend path %q: %w", webArg, err)
	}
	info, err := os.Stat(web)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("frontend path %s is not a directory: %w", web, err)
	}
	web, err = filepath.EvalSymlinks(web)
	if err != nil {
		return "", fmt.Errorf("resolve physical frontend path: %w", err)
	}
	selectedWorkspace := false
	if info, err := os.Lstat(filepath.Join(web, "pnpm-workspace.yaml")); err == nil {
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("selected frontend pnpm-workspace.yaml must be a regular file, not a symlink or special file")
		}
		selectedWorkspace = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect selected frontend pnpm-workspace.yaml: %w", err)
	}
	if _, err := os.Stat(filepath.Join(web, "package.json")); err != nil {
		return "", fmt.Errorf("frontend %s has no package.json: %w", web, err)
	}
	if !selectedWorkspace {
		for parent := filepath.Dir(web); ; parent = filepath.Dir(parent) {
			workspaceFile := filepath.Join(parent, "pnpm-workspace.yaml")
			if _, err := os.Stat(workspaceFile); err == nil {
				return "", fmt.Errorf("refusing to modify parent pnpm workspace %s for frontend %s; skgo kit-patch only configures the selected frontend root", parent, web)
			} else if !errors.Is(err, os.ErrNotExist) {
				return "", fmt.Errorf("inspect parent workspace file %s: %w", workspaceFile, err)
			}
			next := filepath.Dir(parent)
			if next == parent {
				break
			}
		}
	}
	if options.Workspace != "" {
		workspace, err := filepath.Abs(options.Workspace)
		if err != nil {
			return "", fmt.Errorf("resolve --workspace %q: %w", options.Workspace, err)
		}
		workspace, err = filepath.EvalSymlinks(workspace)
		if err != nil {
			return "", fmt.Errorf("resolve physical --workspace root: %w", err)
		}
		if workspace != web {
			return "", fmt.Errorf("--workspace must equal the selected frontend root %s; parent-workspace mutation is not supported", web)
		}
	}
	return web, nil
}

func pinKit(data []byte, version string) (string, string, []byte, error) {
	if !json.Valid(data) {
		return "", "", nil, fmt.Errorf("package.json is not valid JSON")
	}
	sections := []string{"dependencies", "devDependencies", "optionalDependencies", "peerDependencies"}
	var found []string
	var current string
	var update []byte
	for _, section := range sections {
		span, present, err := objectValueSpan(data, section)
		if err != nil {
			return "", "", nil, fmt.Errorf("parse package.json %s: %w", section, err)
		}
		if !present {
			continue
		}
		value := data[span.start:span.end]
		kitSpan, hasKit, err := objectValueSpan(value, kitPackageName)
		if err != nil {
			return "", "", nil, fmt.Errorf("parse package.json %s: %w", section, err)
		}
		if !hasKit {
			continue
		}
		found = append(found, section)
		var declaration string
		if err := json.Unmarshal(value[kitSpan.start:kitSpan.end], &declaration); err != nil {
			return "", "", nil, fmt.Errorf("%s dependency in %s must be a string: %w", kitPackageName, section, err)
		}
		current = declaration
		newValue, _ := json.Marshal(version)
		newSection := splice(value, kitSpan, newValue)
		update = splice(data, span, newSection)
	}
	if len(found) != 1 {
		return "", "", nil, fmt.Errorf("package.json must declare %s in exactly one dependency section; found %v", kitPackageName, found)
	}
	return found[0], current, update, nil
}

type span struct{ start, end int }

// objectValueSpan finds a direct JSON object's string key and returns the
// byte range of its value, leaving all unrelated source bytes untouched.
func objectValueSpan(data []byte, wanted string) (span, bool, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return span{}, false, fmt.Errorf("expected JSON object: %w", err)
	}
	var found span
	count := 0
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return span{}, false, err
		}
		key, ok := token.(string)
		if !ok {
			return span{}, false, fmt.Errorf("object key has type %T", token)
		}
		start, err := valueStart(data, int(decoder.InputOffset()))
		if err != nil {
			return span{}, false, err
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return span{}, false, err
		}
		end := int(decoder.InputOffset())
		if key == wanted {
			found = span{start: start, end: end}
			count++
		}
	}
	if count > 1 {
		return span{}, false, fmt.Errorf("duplicate JSON object key %q", wanted)
	}
	return found, count == 1, nil
}

func valueStart(data []byte, offset int) (int, error) {
	for offset < len(data) && (data[offset] == ' ' || data[offset] == '\n' || data[offset] == '\r' || data[offset] == '\t') {
		offset++
	}
	if offset >= len(data) || data[offset] != ':' {
		return 0, fmt.Errorf("expected colon at byte %d", offset)
	}
	offset++
	for offset < len(data) && (data[offset] == ' ' || data[offset] == '\n' || data[offset] == '\r' || data[offset] == '\t') {
		offset++
	}
	if offset >= len(data) {
		return 0, fmt.Errorf("missing object value at byte %d", offset)
	}
	return offset, nil
}

func splice(source []byte, at span, replacement []byte) []byte {
	result := make([]byte, 0, len(source)-at.end+at.start+len(replacement))
	result = append(result, source[:at.start]...)
	result = append(result, replacement...)
	result = append(result, source[at.end:]...)
	return result
}

func checkPatchDestination(p *project) error {
	patchDir := filepath.Dir(p.patchPath)
	if info, err := os.Lstat(patchDir); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("patch directory %s is not a real directory", patchDir)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect patch directory %s: %w", patchDir, err)
	}
	info, err := os.Lstat(p.patchPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect patch destination %s: %w", p.patchPath, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("patch destination %s exists and is not a regular file", p.patchPath)
	}
	data, err := os.ReadFile(p.patchPath)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, p.patch) {
		return fmt.Errorf("refusing to overwrite different patch bytes at %s", p.patchPath)
	}
	return nil
}

func requirePatchDestination(p *project) error {
	if err := checkPatchDestination(p); err != nil {
		return err
	}
	if _, err := os.Lstat(p.patchPath); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("canonical Kit queue patch is missing at %s", p.patchPath)
	} else if err != nil {
		return err
	}
	return nil
}

func checkPatchMap(p *project) error {
	wantKey := patchKey(p)
	for key, value := range p.patched {
		if key == wantKey {
			if filepath.IsAbs(value) || !sameConfiguredPath(p.root, value, p.patchPath) {
				return fmt.Errorf("conflicting %s patch entry points to %q; expected %s", key, value, relativePatchValue(p))
			}
			continue
		}
		if key == kitPackageName || strings.HasPrefix(key, kitPackageName+"@") {
			return fmt.Errorf("conflicting Kit patch entry %q exists; refusing to compose patches", key)
		}
	}
	return nil
}

func requirePatchMap(p *project) error {
	value, ok := p.patched[patchKey(p)]
	if !ok {
		return fmt.Errorf("project pnpm config is missing %s; run `go tool skgo kit-patch --web %s --apply`", patchKey(p), p.web)
	}
	if filepath.IsAbs(value) || !sameConfiguredPath(p.root, value, p.patchPath) {
		return fmt.Errorf("project pnpm config maps %s to %q; expected %s", patchKey(p), value, relativePatchValue(p))
	}
	return nil
}

func checkInstalledIfPresent(p *project) error {
	installed := filepath.Join(p.web, "node_modules", filepath.FromSlash(p.metadata.Package))
	if _, err := os.Lstat(installed); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect installed Kit package %s: %w", installed, err)
	}
	name, version, queueHash, err := installedQueue(p)
	if err != nil {
		return err
	}
	if name != p.metadata.Package || version != p.metadata.Version {
		return fmt.Errorf("installed Kit is %s@%s; supported package is %s@%s", name, version, p.metadata.Package, p.metadata.Version)
	}
	if queueHash != p.metadata.StockSHA256 && queueHash != p.metadata.CorrectedSHA256 {
		return fmt.Errorf("installed Kit 3.0.0 queue source has unexpected SHA-256 %s; expected stock %s or corrected %s", queueHash, p.metadata.StockSHA256, p.metadata.CorrectedSHA256)
	}
	return nil
}

func checkInstalledCorrected(p *project) error {
	name, version, queueHash, err := installedQueue(p)
	if err != nil {
		return fmt.Errorf("Kit patch is configured but installed Kit could not be verified: %w", err)
	}
	if name != p.metadata.Package || version != p.metadata.Version {
		return fmt.Errorf("installed Kit is %s@%s; expected %s@%s", name, version, p.metadata.Package, p.metadata.Version)
	}
	if queueHash != p.metadata.CorrectedSHA256 {
		return fmt.Errorf("installed Kit queue SHA-256 is %s; expected corrected %s (run the native VitePlus install)", queueHash, p.metadata.CorrectedSHA256)
	}
	return nil
}

func installedQueue(p *project) (name, version, queueHash string, err error) {
	packagePath := filepath.Join(p.web, "node_modules", filepath.FromSlash(p.metadata.Package), "package.json")
	data, err := os.ReadFile(packagePath)
	if err != nil {
		return "", "", "", err
	}
	var installed struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &installed); err != nil {
		return "", "", "", fmt.Errorf("parse installed Kit manifest: %w", err)
	}
	queuePath := filepath.Join(filepath.Dir(packagePath), filepath.FromSlash(p.metadata.QueuePath))
	queue, err := os.ReadFile(queuePath)
	if err != nil {
		return "", "", "", fmt.Errorf("read installed Kit queue %s: %w", queuePath, err)
	}
	return installed.Name, installed.Version, digest(queue), nil
}

func writePatch(p *project) error {
	patchDir := filepath.Dir(p.patchPath)
	if info, err := os.Lstat(patchDir); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("patch directory %s is not a real directory", patchDir)
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(patchDir, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("create patch directory %s: %w", patchDir, err)
		}
	} else {
		return err
	}
	if _, err := os.Lstat(p.patchPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	tmp, err := os.CreateTemp(patchDir, ".skgo-kit-patch-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(p.patch); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Link(tmpName, p.patchPath); err != nil {
		if errors.Is(err, os.ErrExist) {
			return checkPatchDestination(p)
		}
		return fmt.Errorf("write patch %s without overwriting existing files: %w", p.patchPath, err)
	}
	return nil
}

func atomicReplace(path string, data []byte) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to replace non-regular file %s", path)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".skgo-package-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func getPatchedDependencies(options Options, web string) (map[string]string, error) {
	output, err := runPnpm(options, web, "config", "get", "patchedDependencies", "--location=project", "--json")
	if err != nil {
		return nil, fmt.Errorf("read project pnpm patchedDependencies: %w%s", err, formatOutput(output))
	}
	trimmed := bytes.TrimSpace(output)
	var native map[string]string
	if len(trimmed) != 0 && !bytes.Equal(trimmed, []byte("null")) {
		if err := json.Unmarshal(trimmed, &native); err != nil {
			return nil, fmt.Errorf("pnpm config get returned invalid patchedDependencies JSON: %w", err)
		}
	}
	if native == nil {
		native = map[string]string{}
	}
	raw, err := readRawPatchedDependencies(web)
	if err != nil {
		return nil, err
	}
	if len(native) != len(raw) {
		return nil, fmt.Errorf("pnpm project configuration is ambiguous: native patchedDependencies has %d entries but pnpm-workspace.yaml has %d", len(native), len(raw))
	}
	for key, rawPath := range raw {
		nativePath, ok := native[key]
		if !ok || !sameConfiguredPath(web, nativePath, resolvedConfigPath(web, rawPath)) {
			return nil, fmt.Errorf("pnpm project configuration is ambiguous for patchedDependencies entry %q", key)
		}
	}
	return raw, nil
}

// readRawPatchedDependencies decodes the selected workspace file without
// rewriting it. pnpm's JSON getter resolves patch paths to absolute paths;
// keeping this decoded source map lets config set preserve unrelated authored
// relative values. pnpm remains the sole writer and full config owner.
func readRawPatchedDependencies(root string) (map[string]string, error) {
	path := filepath.Join(root, "pnpm-workspace.yaml")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read selected pnpm workspace config %s: %w", path, err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document map[string]any
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("parse selected pnpm workspace config %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("selected pnpm workspace config %s contains multiple YAML documents", path)
		}
		return nil, fmt.Errorf("parse selected pnpm workspace config %s: %w", path, err)
	}
	if document == nil {
		return nil, fmt.Errorf("selected pnpm workspace config %s must contain a YAML mapping", path)
	}
	value, present := document["patchedDependencies"]
	if !present {
		return map[string]string{}, nil
	}
	entries, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("patchedDependencies in %s must be a mapping of string keys to string paths", path)
	}
	patched := make(map[string]string, len(entries))
	for key, value := range entries {
		value, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("patchedDependencies entry %q in %s must have a string path", key, path)
		}
		patched[key] = value
	}
	return patched, nil
}

func setPatchedDependencies(options Options, p *project) error {
	data, err := json.Marshal(p.patched)
	if err != nil {
		return err
	}
	output, err := runPnpm(options, p.web, "config", "set", "patchedDependencies", string(data), "--location=project", "--json")
	if err != nil {
		return fmt.Errorf("pnpm could not write project patchedDependencies: %w%s", err, formatOutput(output))
	}
	return nil
}

func runPnpm(options Options, dir string, args ...string) ([]byte, error) {
	runner := options.run
	if runner == nil {
		runner = func(dir, name string, args ...string) ([]byte, error) {
			cmd := exec.Command(name, args...)
			cmd.Dir = dir
			return cmd.CombinedOutput()
		}
	}
	name := options.PNPM
	if name == "" {
		name = "pnpm"
	}
	versionOutput, err := runner(dir, name, "--version")
	if err != nil {
		return versionOutput, fmt.Errorf("pnpm is required (install VitePlus/pnpm 12.9.1): %w%s", err, formatOutput(versionOutput))
	}
	if strings.TrimSpace(string(versionOutput)) != pnpmVersion {
		return versionOutput, fmt.Errorf("pnpm %s is required, found %q", pnpmVersion, strings.TrimSpace(string(versionOutput)))
	}
	return runner(dir, name, args...)
}

func formatOutput(output []byte) string {
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return ""
	}
	return ": " + trimmed
}

func patchKey(p *project) string { return p.metadata.Package + "@" + p.metadata.Version }

func relativePatchValue(p *project) string {
	rel, err := filepath.Rel(p.root, p.patchPath)
	if err != nil {
		return p.patchPath
	}
	return filepath.ToSlash(rel)
}

func resolvedConfigPath(root, configured string) string {
	path := filepath.FromSlash(configured)
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	return path
}

func sameConfiguredPath(root, configured, expected string) bool {
	path := configured
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, filepath.FromSlash(path))
	}
	abs, err := canonicalPath(path)
	if err != nil {
		return false
	}
	want, err := canonicalPath(expected)
	return err == nil && abs == want
}

func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return filepath.Clean(resolved), nil
	}
	parent, err := canonicalPath(filepath.Dir(abs))
	if err != nil {
		return "", err
	}
	return filepath.Clean(filepath.Join(parent, filepath.Base(abs))), nil
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func readRegularFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	return os.ReadFile(path)
}
