package plugininstall

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tylergannon/skgo/internal/buildinfo"
	"github.com/tylergannon/skgo/internal/pluginbuild"
	"github.com/tylergannon/skgo/internal/pluginstore"
	"github.com/tylergannon/skgo/internal/templates"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

type Options struct {
	Host                  buildinfo.Info
	Home, CWD, ManualDirs string
	Out                   io.Writer
}
type Exports struct {
	Templates, Addons []string
	Diagnostics       []string
}

func (o Options) location() (string, string, error) {
	digest, err := pluginstore.Digest(o.Host.Executable)
	if err != nil {
		return "", "", err
	}
	dir := pluginstore.Dir(o.Home, digest)
	if err = os.MkdirAll(dir, 0755); err != nil {
		return "", "", err
	}
	return digest, dir, nil
}
func Install(ctx context.Context, ref string, o Options) error {
	if o.Out == nil {
		o.Out = io.Discard
	}
	if o.Host.Build == nil {
		return fmt.Errorf("executing host has no Go build identity")
	}
	if o.Host.Build.Main.Path == buildinfo.Module && !semver.IsValid(o.Host.Build.Main.Version) && o.Host.Build.Main.Replace == nil {
		return fmt.Errorf("plugin install cannot reproduce this development host; use a released skgo executable or a module build with an absolute recorded source replacement")
	}
	digest, dir, err := o.location()
	if err != nil {
		return err
	}
	fmt.Fprintf(o.Out, "Target host: %s, skgo %s, %s, SHA256 %s.\n", o.Host.Executable, o.Host.SkgoVersion, o.Host.Build.GoVersion, digest)
	s, cleanup, err := fetch(ctx, ref, o.Host.Build.GoVersion, o.Out)
	if err != nil {
		return err
	}
	defer cleanup()
	candidateDir, err := os.MkdirTemp("", "skgo-plugin-candidate-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(candidateDir)
	candidate := filepath.Join(candidateDir, "plugin.so")
	if err = pluginbuild.Build(ctx, pluginbuild.Options{Host: o.Host.Executable, Source: s.Root, Package: s.Package, Output: candidate, VersionSymbol: "main.skgoVersion", Generate: true, Log: o.Out}); err != nil {
		return err
	}
	fmt.Fprintln(o.Out, "Acquire host installation lock:", filepath.Join(dir, ".install.lock"))
	unlock, err := lock(ctx, filepath.Join(dir, ".install.lock"))
	if err != nil {
		return err
	}
	defer unlock()
	slot := pluginstore.Slot(o.Home, digest, s.Path)
	files, diagnostics, err := templates.Files(o.ManualDirs, o.Home, o.CWD, dir)
	if err != nil {
		return err
	}
	for _, d := range diagnostics {
		fmt.Fprintln(o.Out, "warning:", d)
	}
	oldSlot := slot
	if canonical, e := filepath.EvalSymlinks(slot); e == nil {
		oldSlot = canonical
	}
	peers := files[:0]
	for _, f := range files {
		if f != oldSlot {
			peers = append(peers, f)
		}
	}
	peers = append(peers, candidate)
	exports, err := qualify(ctx, o.Host.Executable, candidate, peers)
	if err != nil {
		return err
	}
	for _, d := range exports.Diagnostics {
		fmt.Fprintln(o.Out, "warning:", d)
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = publish(candidate, slot, pluginstore.Source{Module: s.Path, Version: s.Version}); err != nil {
		return err
	}
	fmt.Fprintf(o.Out, "Installed %s@%s for this host at %s.\nTemplates: %s\nAdd-ons: %s\n", s.Path, s.Version, slot, strings.Join(exports.Templates, ", "), strings.Join(exports.Addons, ", "))
	return nil
}
func qualify(ctx context.Context, host, candidate string, files []string) (Exports, error) {
	var result Exports
	encoded, err := json.Marshal(files)
	if err != nil {
		return result, err
	}
	report := filepath.Join(filepath.Dir(candidate), "probe.json")
	c := pluginbuild.ChildCommand(ctx, host, "_plugin-probe", candidate, report)
	c.Env = append(os.Environ(), pluginstore.ProbeFiles+"="+string(encoded))
	out, err := c.CombinedOutput()
	if err != nil {
		return result, fmt.Errorf("candidate failed qualification with installed peers: %w\n%s", err, out)
	}
	b, err := os.ReadFile(report)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(b, &result)
	return result, err
}
func writeTemp(dir string, data []byte, mode os.FileMode) (string, error) {
	f, err := os.CreateTemp(dir, ".skgo-install-")
	if err != nil {
		return "", err
	}
	path := f.Name()
	if _, err = f.Write(data); err == nil {
		err = f.Chmod(mode)
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}
func publish(candidate, slot string, source pluginstore.Source) error {
	b, err := os.ReadFile(candidate)
	if err != nil {
		return err
	}
	temp, err := writeTemp(filepath.Dir(slot), b, 0755)
	if err != nil {
		return err
	}
	defer os.Remove(temp)
	ref := pluginstore.Reference(slot)
	old, oldErr := os.ReadFile(ref)
	if oldErr != nil && !os.IsNotExist(oldErr) {
		return oldErr
	}
	b, err = json.Marshal(source)
	if err != nil {
		return err
	}
	next, err := writeTemp(filepath.Dir(slot), b, 0644)
	if err != nil {
		return err
	}
	defer os.Remove(next)
	if err = os.Rename(next, ref); err != nil {
		return err
	}
	if err = os.Rename(temp, slot); err != nil {
		if oldErr == nil {
			_ = os.WriteFile(ref, old, 0644)
		} else {
			_ = os.Remove(ref)
		}
		return err
	}
	return nil
}
func Remove(ctx context.Context, path string, o Options) error {
	if err := module.CheckPath(path); err != nil {
		return err
	}
	if o.Out == nil {
		o.Out = io.Discard
	}
	digest, dir, err := o.location()
	if err != nil {
		return err
	}
	fmt.Fprintln(o.Out, "Acquire host installation lock:", filepath.Join(dir, ".install.lock"))
	unlock, err := lock(ctx, filepath.Join(dir, ".install.lock"))
	if err != nil {
		return err
	}
	defer unlock()
	slot := pluginstore.Slot(o.Home, digest, path)
	for _, file := range []string{slot, pluginstore.Reference(slot)} {
		err = os.Remove(file)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		fmt.Fprintln(o.Out, "Removed if present:", file)
	}
	return nil
}
