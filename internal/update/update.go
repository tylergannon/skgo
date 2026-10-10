// Package update installs the selected released CLI, then lets that executable
// complete alignment using its own frontend compatibility pins.
package update

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tylergannon/skgo/internal/buildinfo"
	"github.com/tylergannon/skgo/internal/newapp"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/semver"
)

type Options struct {
	Root, VP, CompleteVersion string
	Out                       io.Writer
	run                       func(context.Context, command) ([]byte, error)
	info                      func() (buildinfo.Info, error)
	packages                  func(string) (string, string, error)
}
type command struct {
	Dir, Name string
	Args, Env []string
	Quiet     bool
}

func Run(ctx context.Context, o Options) error {
	if o.Out == nil {
		o.Out = io.Discard
	}
	if o.info == nil {
		o.info = buildinfo.Current
	}
	if o.packages == nil {
		o.packages = newapp.CompatiblePackages
	}
	if o.run == nil {
		o.run = func(ctx context.Context, c command) ([]byte, error) {
			cmd := exec.CommandContext(ctx, c.Name, c.Args...)
			cmd.Dir = c.Dir
			cmd.Env = append(os.Environ(), c.Env...)
			var b bytes.Buffer
			if c.Quiet {
				cmd.Stdout = &b
			} else {
				cmd.Stdout = io.MultiWriter(&b, o.Out)
			}
			cmd.Stderr = o.Out
			if err := cmd.Run(); err != nil {
				return b.Bytes(), fmt.Errorf("%s %s: %w", c.Name, strings.Join(c.Args, " "), err)
			}
			return b.Bytes(), nil
		}
	}
	info, err := o.info()
	if err != nil {
		return err
	}
	if o.CompleteVersion != "" {
		if info.SkgoVersion != o.CompleteVersion {
			return fmt.Errorf("completion requires skgo %s; executing %s", o.CompleteVersion, info.SkgoVersion)
		}
		return complete(ctx, o, info)
	}
	root, err := projectRoot(o.Root)
	if err != nil {
		return err
	}
	o.Root = root
	data, err := o.run(ctx, command{Name: "go", Args: []string{"env", "-json", "GOBIN", "GOPATH", "GOCACHE"}, Quiet: true})
	if err != nil {
		return fmt.Errorf("update requires an installed Go toolchain: %w", err)
	}
	var env struct{ GOBIN, GOPATH, GOCACHE string }
	if err = json.Unmarshal(data, &env); err != nil {
		return err
	}
	dir := filepath.Dir(info.Executable)
	cache := env.GOCACHE
	if c, e := filepath.EvalSymlinks(cache); e == nil {
		cache = c
	}
	if rel, e := filepath.Rel(cache, info.Executable); e == nil && filepath.IsLocal(rel) {
		dir = env.GOBIN
		if dir == "" {
			paths := filepath.SplitList(env.GOPATH)
			if len(paths) == 0 || paths[0] == "" {
				return fmt.Errorf("Go has no installation directory")
			}
			dir = filepath.Join(paths[0], "bin")
		}
		fmt.Fprintf(o.Out, "Project Go-tool invocation: install completion CLI in %s; cached executable remains untouched.\n", dir)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	probe, err := os.CreateTemp(dir, ".skgo-update-write-")
	if err != nil {
		return fmt.Errorf("CLI installation directory is not writable: %w", err)
	}
	probe.Close()
	os.Remove(probe.Name())
	data, err = o.run(ctx, command{Name: "go", Args: []string{"list", "-m", "-json", buildinfo.Module + "@latest"}, Env: []string{"GOWORK=off"}, Quiet: true})
	if err != nil {
		return err
	}
	var latest struct{ Version string }
	if err = json.Unmarshal(data, &latest); err != nil {
		return err
	}
	if !semver.IsValid(latest.Version) || semver.Prerelease(latest.Version) != "" {
		return fmt.Errorf("latest skgo version is not a stable release: %q", latest.Version)
	}
	fmt.Fprintf(o.Out, "Installing skgo %s with Go.\n", latest.Version)
	if _, err = o.run(ctx, command{Name: "go", Args: []string{"install", buildinfo.Module + "/cmd/skgo@" + latest.Version}, Env: []string{"GOWORK=off", "GOBIN=" + dir}}); err != nil {
		return err
	}
	installed := filepath.Join(dir, "skgo")
	data, err = o.run(ctx, command{Name: installed, Args: []string{"buildinfo", "--json"}, Quiet: true})
	if err != nil {
		return err
	}
	var next buildinfo.Info
	if err = json.Unmarshal(data, &next); err != nil {
		return err
	}
	if next.SkgoVersion != latest.Version {
		return fmt.Errorf("installed CLI reports %s, expected %s", next.SkgoVersion, latest.Version)
	}
	fmt.Fprintf(o.Out, "CLI installed: %s (%s) -> %s (%s).\n", info.Executable, info.SkgoVersion, installed, next.SkgoVersion)
	pathBin, e := exec.LookPath("skgo")
	if e == nil {
		if p, e := filepath.EvalSymlinks(pathBin); e == nil {
			pathBin = p
		}
	}
	canonical, e := filepath.EvalSymlinks(installed)
	if e != nil {
		canonical = installed
	}
	if pathBin != canonical {
		fmt.Fprintf(o.Out, "PATH still resolves skgo to %q; use %s or adjust PATH.\n", pathBin, installed)
	}
	args := []string{"update", "--complete-version", latest.Version}
	if root != "" {
		args = append(args, "--root", root)
	}
	if o.VP != "" {
		args = append(args, "--vp", o.VP)
	}
	if _, err = o.run(ctx, command{Name: installed, Args: args}); err != nil {
		return fmt.Errorf("CLI %s is installed, but toolchain/project update is incomplete: %w", latest.Version, err)
	}
	return nil
}
func projectRoot(explicit string) (string, error) {
	root := explicit
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	for {
		data, e := os.ReadFile(filepath.Join(root, "go.mod"))
		if e == nil {
			mod, e := modfile.Parse("go.mod", data, nil)
			if e != nil {
				return "", e
			}
			uses := false
			for _, r := range mod.Require {
				if r.Mod.Path == buildinfo.Module {
					uses = true
				}
			}
			if uses {
				if _, e := os.Stat(filepath.Join(root, "web/package.json")); e == nil {
					return root, nil
				}
			}
			if explicit != "" {
				return "", fmt.Errorf("%s is not an skgo application with web/package.json", root)
			}
			return "", nil
		}
		if e != nil && !os.IsNotExist(e) {
			return "", e
		}
		if explicit != "" {
			return "", fmt.Errorf("%s has no go.mod", root)
		}
		parent := filepath.Dir(root)
		if parent == root {
			return "", nil
		}
		root = parent
	}
}
