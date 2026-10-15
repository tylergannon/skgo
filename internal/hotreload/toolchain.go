// Package hotreload builds the private native development profile. The SDK and
// build driver deliberately do not import the native loader.
package hotreload

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
)

const LoaderVersion = "v0.0.26-0.20260915095715-2695cfc53506"

// Toolchain identifies the stock compiler and private object-reader source
// view used together by hosts, replacement objects and dependency archives.
type Toolchain struct {
	Go, Root, Version, OS, Arch, Identity string
	// Private host build inputs, empty for ordinary candidate compilation.
	ModFile, Overlay string
	AuthoredPaths    map[string]string
	Settings         map[string]string
}

// PrepareToolchain leaves the installed toolchain untouched. The private view
// follows GoLoader's documented cmd/internal -> cmd/objfile preparation, without
// modifying shared source through a symlink. Its parent is caller-owned.
func PrepareToolchain(ctx context.Context, goTool, parent string) (Toolchain, error) {
	var t Toolchain
	if err := t.checkFlags(); err != nil {
		return t, err
	}
	cmd := exec.CommandContext(ctx, goTool, "env", "-json", "GOROOT", "GOVERSION", "GOOS", "GOARCH", "CGO_ENABLED", "GOEXPERIMENT", "GOAMD64", "GOARM64", "GOFLAGS")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	raw, err := cmd.Output()
	if err != nil {
		return t, fmt.Errorf("inspect Go toolchain: %w", err)
	}
	var env map[string]string
	if err := json.Unmarshal(raw, &env); err != nil {
		return t, err
	}
	if env["GOVERSION"] != "go1.27.2" {
		return t, fmt.Errorf("native development requires qualified Go 1.27.2; got %s", env["GOVERSION"])
	}
	profile := env["GOOS"] + "/" + env["GOARCH"]
	if profile != "darwin/arm64" && profile != "linux/amd64" {
		return t, fmt.Errorf("native development is not qualified on %s", profile)
	}
	if env["GOOS"] != runtime.GOOS || env["GOARCH"] != runtime.GOARCH {
		return t, fmt.Errorf("native development requires the running platform %s/%s; selected %s", runtime.GOOS, runtime.GOARCH, profile)
	}
	if env["GOEXPERIMENT"] != "" {
		return t, fmt.Errorf("native development profile does not support GOEXPERIMENT=%s", env["GOEXPERIMENT"])
	}
	root := env["GOROOT"]
	// Bind the actual tool executable as well as its version/path. The object
	// reader is copied from the same installation, not a separately chosen Go.
	compile, err := os.ReadFile(filepath.Join(root, "pkg", "tool", env["GOOS"]+"_"+env["GOARCH"], "compile"))
	if err != nil {
		return t, fmt.Errorf("native compiler prerequisite is missing from %s: %w", root, err)
	}
	for _, tool := range []string{"asm", "link"} {
		if _, err := os.Stat(filepath.Join(root, "pkg/tool", env["GOOS"]+"_"+env["GOARCH"], tool)); err != nil {
			return t, fmt.Errorf("native %s prerequisite is missing: %w", tool, err)
		}
	}
	h := sha256.New()
	h.Write(raw)
	h.Write(compile)
	h.Write([]byte("skgo-object-source-v4-funcdata:" + LoaderVersion + ":" + PuregoVersion))
	for _, name := range []string{"dl_unix.go.txt", "mmap_darwin_arm64.go.txt", "typelink_go127.go.txt"} {
		source, err := loaderOS.ReadFile("loaderos/" + name)
		if err != nil {
			return t, err
		}
		h.Write(source)
	}
	objSource := filepath.Join(root, "src/cmd/internal")
	readerSources := map[string][]byte{}
	err = filepath.WalkDir(objSource, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(objSource, path)
		if err != nil {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		readerSources[rel] = body
		return nil
	})
	if err != nil {
		return t, fmt.Errorf("capture stock object reader: %w", err)
	}
	var names []string
	for name := range readerSources {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write(readerSources[name])
		h.Write([]byte{0})
	}
	identity := hex.EncodeToString(h.Sum(nil))
	private := filepath.Join(parent, identity)
	if err := os.MkdirAll(private, 0755); err != nil {
		return t, err
	}
	// Three directories must be real; everything else is immutable stock
	// toolchain content. cmd/objfile itself is always a private copy.
	for _, rel := range []string{".", "src", "src/cmd"} {
		entries, err := os.ReadDir(filepath.Join(root, rel))
		if err != nil {
			return t, err
		}
		for _, e := range entries {
			name := filepath.Join(rel, e.Name())
			if name == "src" || name == "src/cmd" {
				if err := os.MkdirAll(filepath.Join(private, name), 0755); err != nil {
					return t, err
				}
				continue
			}
			if name == "src/cmd/objfile" {
				continue
			}
			dest := filepath.Join(private, name)
			if err := os.Symlink(filepath.Join(root, name), dest); err != nil && !os.IsExist(err) {
				return t, err
			}
		}
	}
	for _, rel := range names {
		if err := ctx.Err(); err != nil {
			return t, err
		}
		dest := filepath.Join(private, "src/cmd/objfile", rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return t, err
		}
		if err := os.WriteFile(dest, readerSources[rel], 0644); err != nil {
			return t, fmt.Errorf("prepare private object reader: %w", err)
		}
	}
	return Toolchain{Go: filepath.Join(private, "bin/go"), Root: private, Version: env["GOVERSION"], OS: env["GOOS"], Arch: env["GOARCH"], Identity: identity, Settings: map[string]string{"GOOS": env["GOOS"], "GOARCH": env["GOARCH"], "GOEXPERIMENT": env["GOEXPERIMENT"], "GOAMD64": env["GOAMD64"], "GOARM64": env["GOARM64"], "GOFLAGS": env["GOFLAGS"]}}, nil
}

func (t Toolchain) command(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, t.Go, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOROOT="+t.Root, "GOWORK=off", "GOTOOLCHAIN=local", "CGO_ENABLED=0")
	for name, value := range t.Settings {
		cmd.Env = append(cmd.Env, name+"="+value)
	}
	if t.ModFile != "" || t.Overlay != "" {
		flags := os.Getenv("GOFLAGS")
		if t.Settings != nil {
			flags = t.Settings["GOFLAGS"]
		}
		if t.ModFile != "" {
			flags += " " + strconv.Quote("-modfile="+t.ModFile)
		}
		if t.Overlay != "" {
			flags += " " + strconv.Quote("-overlay="+t.Overlay)
		}
		cmd.Env = append(cmd.Env, "GOFLAGS="+flags)
	}
	return cmd
}
