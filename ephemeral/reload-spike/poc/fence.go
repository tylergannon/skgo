package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
)

// No TTL or mtime shortcut: the experimental fast lane is a single file over
// byte-identical frozen dependencies/compiler. A changed dependency is a
// restart, not something this POC tries to incrementally relink.
type fence struct {
	files       []string
	dirs        []string
	environment map[string]string
	digest      [32]byte
}

var buildEnvironment = []string{"PATH", "GOROOT", "GOTOOLDIR", "GOFLAGS", "GOENV", "GOTOOLCHAIN", "GOOS", "GOARCH", "GOARM64", "GOEXPERIMENT", "CGO_ENABLED", "GOWORK"}

func newFence(goBinary, metadata string) (*fence, error) {
	if build, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range build.Settings {
			switch setting.Key {
			case "-race", "-msan", "-asan":
				if setting.Value == "true" {
					return nil, fmt.Errorf("%w: instrumented host", ErrRestart)
				}
			case "-gcflags", "-asmflags":
				if setting.Value != "" {
					return nil, fmt.Errorf("%w: custom host compiler flags", ErrRestart)
				}
			case "GOEXPERIMENT":
				if setting.Value != "" {
					return nil, fmt.Errorf("%w: host experiments", ErrRestart)
				}
			}
		}
	}
	// The tool that compiles the candidate must match the already running host.
	cmd := exec.Command(goBinary, "env", "-json", "GOVERSION", "GOOS", "GOARCH", "GOARM64", "GOROOT", "GOTOOLDIR", "GOFLAGS", "GOEXPERIMENT", "CGO_ENABLED")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	env := map[string]string{}
	if err := json.Unmarshal(out, &env); err != nil {
		return nil, err
	}
	if env["GOVERSION"] != runtime.Version() || env["GOOS"] != runtime.GOOS || env["GOARCH"] != runtime.GOARCH || env["GOROOT"] != runtime.GOROOT() || env["GOFLAGS"] != "" || env["GOEXPERIMENT"] != "" || env["GOARM64"] != "v8.0" || env["CGO_ENABLED"] != "1" {
		return nil, fmt.Errorf("%w: compiler/environment does not match supported host", ErrRestart)
	}
	f := &fence{environment: map[string]string{}, files: []string{goBinary, filepath.Join(env["GOTOOLDIR"], "compile"), filepath.Join(env["GOTOOLDIR"], "asm")}}
	for _, key := range buildEnvironment {
		f.environment[key] = os.Getenv(key)
	}
	decoder := json.NewDecoder(strings.NewReader(metadata))
	seen := map[string]bool{}
	for {
		var pkg struct{ Dir, Export string }
		if err := decoder.Decode(&pkg); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		if pkg.Export != "" {
			f.files = append(f.files, pkg.Export)
		}
		if pkg.Dir != "" && !seen[pkg.Dir] {
			f.dirs = append(f.dirs, pkg.Dir)
			seen[pkg.Dir] = true
		}
	}
	sort.Strings(f.files)
	sort.Strings(f.dirs)
	f.digest, err = f.sum()
	return f, err
}

func (f *fence) sum() ([32]byte, error) {
	h := sha256.New()
	files := append([]string(nil), f.files...)
	// Include names and content of ALL direct package source/build inputs. This
	// catches additions/removals, ignored-file/tag edits and same-mtime edits.
	for _, dir := range f.dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return [32]byte{}, err
		}
		for _, entry := range entries {
			if !entry.IsDir() && !strings.HasSuffix(entry.Name(), "_test.go") {
				files = append(files, filepath.Join(dir, entry.Name()))
			}
		}
	}
	sort.Strings(files)
	for _, name := range files {
		fmt.Fprintf(h, "%d:%s\x00", len(name), name)
		file, err := os.Open(name)
		if err != nil {
			return [32]byte{}, err
		}
		_, err = io.Copy(h, file)
		file.Close()
		if err != nil {
			return [32]byte{}, err
		}
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum, nil
}

func (f *fence) check() error {
	for _, key := range buildEnvironment {
		if os.Getenv(key) != f.environment[key] {
			return fmt.Errorf("%w: environment %s changed", ErrRestart, key)
		}
	}
	sum, err := f.sum()
	if err != nil {
		return fmt.Errorf("%w: frozen input unavailable: %v", ErrRestart, err)
	}
	if sum != f.digest {
		return fmt.Errorf("%w: dependency or compiler bytes/membership changed", ErrRestart)
	}
	return nil
}
