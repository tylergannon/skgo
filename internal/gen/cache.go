package gen

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"golang.org/x/tools/imports"
)

const generationCacheFormat = "skgo-generation-plan-v2"
const maxGenerationCacheBytes = 32 << 20

// The cache stores SKGO's own completed, formatted file plan. It never tries to
// deserialize compiler objects, and it does not cache failures or advice.
type generationPlan struct {
	Files   map[string]string
	GoFiles []string
}
type generationEntry struct {
	InputHash  string
	Sources    cacheSources
	SourceHash string
	Metadata   *kitRouteMetadata
	Plan       generationPlan
}
type generationCache struct{ plan generationPlan }

var generatorIdentity = sync.OnceValues(func() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	return cacheFileHash(path)
})

func cacheFileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func generationCachePath(cfg Config) (string, string, error) {
	producer, err := generatorIdentity()
	if err != nil {
		return "", "", err
	}
	goPath, err := exec.LookPath("go")
	if err != nil {
		return "", "", err
	}
	goIdentity, err := cacheFileHash(goPath)
	if err != nil {
		return "", "", err
	}
	nodePath, err := exec.LookPath("node")
	if err != nil {
		return "", "", err
	}
	nodePath, err = filepath.EvalSymlinks(nodePath)
	if err != nil {
		return "", "", err
	}
	nodeIdentity, err := cacheFileHash(nodePath)
	if err != nil {
		return "", "", err
	}
	// Effective Go settings are validated separately; launcher-only GOFILE,
	// GOLINE and GOPACKAGE must not split go generate from direct invocations.
	// PATH selects Node here; the frontend formatter is a fixed project path.
	// Keep only tool inputs. Arbitrary authored JS inputs are represented by its
	// live semantic result, so environment values such as secrets aren't stored.
	var env []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if key == "GODEBUG" || strings.HasPrefix(key, "CGO") || strings.HasPrefix(key, "NODE_") || key == "CC" || key == "CXX" || key == "PKG_CONFIG" || key == "HOME" || key == "XDG_CONFIG_HOME" {
			env = append(env, entry)
		}
	}
	sort.Strings(env)
	input := struct {
		Format, Producer, GoTool, NodePath, NodeTool, Checker, ImportPrefix                                          string
		Web, Out, Package, LocalsPackage, LocalsType, HookPackage, HookSymbol, ConfigPackage, ConfigSymbol, SwiftOut string
		SwiftRemotes                                                                                                 []string
		Language                                                                                                     Language
		Env                                                                                                          []string
	}{generationCacheFormat, producer, goIdentity, nodePath, nodeIdentity, runtime.Version(), imports.LocalPrefix, cfg.Web, cfg.Out, cfg.Package, cfg.LocalsPackage, cfg.LocalsType, cfg.HookPackage, cfg.HookSymbol, cfg.ConfigPackage, cfg.ConfigSymbol, cfg.SwiftOut, cfg.SwiftRemotes, cfg.Language, env}
	raw, err := json.Marshal(input)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(raw)
	target, _ := json.Marshal([]string{cfg.Web, cfg.Out, cfg.SwiftOut})
	slot := sha256.Sum256(target)
	return filepath.Join(cfg.CacheDir, fmt.Sprintf("%x.plan", slot)), fmt.Sprintf("%x", sum), nil
}

func readGenerationEntry(path string) (generationEntry, bool) {
	var entry generationEntry
	f, err := os.Open(path)
	if err != nil {
		return entry, false
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxGenerationCacheBytes+1))
	if err != nil || len(data) > maxGenerationCacheBytes || len(data) < sha256.Size {
		return entry, false
	}
	sum := sha256.Sum256(data[sha256.Size:])
	if !bytes.Equal(sum[:], data[:sha256.Size]) {
		return entry, false
	}
	if err := json.Unmarshal(data[sha256.Size:], &entry); err != nil || entry.Plan.Files == nil || entry.Metadata == nil {
		return generationEntry{}, false
	}
	return entry, true
}
func writeGenerationEntry(path string, entry generationEntry) error {
	raw, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if len(raw) > maxGenerationCacheBytes-sha256.Size {
		return fmt.Errorf("generation plan exceeds cache size limit")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".plan-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	sum := sha256.Sum256(raw)
	if _, err = f.Write(sum[:]); err == nil {
		_, err = f.Write(raw)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func runCached(cfg Config) error {
	var err error
	cfg, err = normalizeConfig(cfg)
	if err != nil {
		return err
	}
	// Canonical source paths make the private-module mapping unambiguous, including
	// macOS /var aliases. The ordinary path retains its existing behavior.
	cfg.Web, err = sourcePath(cfg.Web)
	if err != nil {
		return err
	}
	cfg.Out, err = sourcePath(cfg.Out)
	if err != nil {
		return err
	}
	if cfg.SwiftOut != "" {
		cfg.SwiftOut, err = sourcePath(cfg.SwiftOut)
		if err != nil {
			return err
		}
	}
	bypass := func(reason error) error {
		cfg.Logf("generation cache bypass: %v", reason)
		cfg.CacheDir = ""
		return runUncached(cfg)
	}
	if driver := os.Getenv("GOPACKAGESDRIVER"); driver != "off" {
		if driver != "" {
			return bypass(fmt.Errorf("external package driver"))
		}
		if _, err := exec.LookPath("gopackagesdriver"); err == nil {
			return bypass(fmt.Errorf("external package driver"))
		}
	}
	path, inputHash, err := generationCachePath(cfg)
	if err != nil {
		return bypass(err)
	}
	files, err := findSourceFiles(cfg.Web)
	if err != nil {
		return err
	}
	metadata, err := captureKitRouteMetadata(cfg, files)
	if err != nil {
		return err
	}
	cfg.kitMetadata = metadata
	if entry, ok := readGenerationEntry(path); cfg.CacheDir != "off" && ok && entry.InputHash == inputHash {
		host, _, moduleErr := moduleOf(cfg.Out)
		environment, envErr := currentCacheEnvironment(host)
		_, digest, err := entry.Sources.capture(cfg)
		a, _ := json.Marshal(metadata)
		b, _ := json.Marshal(entry.Metadata)
		if moduleErr == nil && envErr == nil && cacheEnvironmentHash(environment) == entry.Sources.EnvironmentHash && err == nil && digest == entry.SourceHash && bytes.Equal(a, b) {
			cfg.Logf("generation cache hit")
			return publishGenerationPlan(cfg, entry.Plan)
		}
	}
	sources, err := acquireCacheSources(cfg)
	if err != nil {
		return bypass(err)
	}
	contents, digest, err := sources.capture(cfg)
	if err != nil {
		return bypass(err)
	}
	cfg.Logf("generation cache miss")
	view, cleanup, err := sources.materialize(cfg, contents)
	if err != nil {
		return bypass(err)
	}
	defer cleanup()
	result := &generationCache{}
	view.cache = result
	if err := runUncached(view); err != nil {
		if view.sourceView.cgoUsed {
			return bypass(fmt.Errorf("local cgo source is not cached"))
		}
		// Analysis consumed the captured view. Preserve its error without running
		// the compiler a second time against possibly different source.
		return fmt.Errorf("%s", view.sourceView.originalText(err.Error()))
	}
	// The computed plan is bound to the copied bytes, not live files. A changed
	// current view asks the ordinary path to handle the new invocation instead.
	if _, after, err := sources.capture(cfg); err != nil || after != digest {
		return bypass(fmt.Errorf("source changed during generation"))
	}
	plan := result.plan
	if err := publishGenerationPlan(cfg, plan); err != nil {
		return err
	}
	if view.sourceView.cgoUsed {
		cfg.Logf("generation cache write skipped: local cgo source")
		return nil
	}
	if cfg.CacheDir == "off" {
		return nil
	}
	if err := writeGenerationEntry(path, generationEntry{InputHash: inputHash, Sources: sources, SourceHash: digest, Metadata: metadata, Plan: plan}); err != nil {
		cfg.Logf("generation cache write skipped: %v", err)
	}
	return nil
}

func (c *generationCache) capture(cfg Config) {
	c.plan = generationPlan{Files: map[string]string{}}
	for path, source := range cfg.generation.files {
		c.plan.Files[cfg.sourceView.mapPath(path, false)] = cfg.sourceView.originalText(source)
	}
	for path := range cfg.generation.parts {
		c.plan.GoFiles = append(c.plan.GoFiles, cfg.sourceView.mapPath(path, false))
	}
	sort.Strings(c.plan.GoFiles)
}

func publishGenerationPlan(cfg Config, plan generationPlan) error {
	// A retained owner replays many plans; destination ownership is per replay.
	cfg.produced = map[string]struct{}{}
	if cfg.SwiftOut != "" {
		if err := checkSwiftOutput(cfg.SwiftOut); err != nil {
			return err
		}
	}
	g, err := newGeneration(cfg)
	if err != nil {
		return err
	}
	defer g.close()
	cfg.generation = g
	for path, source := range plan.Files {
		g.files[path] = source
		cfg.produced[path] = struct{}{}
	}
	for _, path := range plan.GoFiles {
		g.parts[path] = []string{plan.Files[path]}
	}
	// These conditional outputs depend on current destination ownership, not Go
	// analysis. In particular the adapter owns existing environment values.
	values := filepath.Join(cfg.Web, ".svelte-kit", "skgo-env-values.js")
	if _, err := os.Stat(values); err == nil {
		delete(g.files, values)
	} else if !os.IsNotExist(err) {
		return err
	}
	_, environment := g.files[filepath.Join(cfg.Web, "skgo.env.json")]
	ownedEnvironment := false
	for _, stem := range []string{"env", "hooks.server"} {
		for _, ext := range []string{".ts", ".js"} {
			path := filepath.Join(cfg.Web, "src", stem+ext)
			data, err := os.ReadFile(path)
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			if _, produced := g.files[path]; produced && err == nil && !generatedArtifact(data) {
				return fmt.Errorf("skgo: %s is authored; refusing to overwrite it", path)
			}
			if generatedArtifact(data) {
				if stem == "env" {
					ownedEnvironment = true
				}
				if _, produced := g.files[path]; !produced {
					g.removed[path] = true
				}
			}
		}
	}
	if !environment && ownedEnvironment {
		g.removed[filepath.Join(cfg.Web, "skgo.env.json")] = true
	}
	if err := pruneStaleArtifacts(cfg); err != nil {
		return err
	}
	return g.publish(cfg)
}
