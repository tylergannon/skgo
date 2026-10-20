package hotreload

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/mod/modfile"
	"os"
	"path/filepath"
	"strings"
)

// HostView prepares compiler-owned overlays and module declarations. Original
// authored and generated production files are never overwritten. The caller
// owns parent until all build commands using the returned toolchain finish.
func (t Toolchain) HostView(ctx context.Context, dir, parent string, sources map[string][]byte, importcfg ...string) (Toolchain, error) {
	t, err := t.hostOverlay(dir, parent, sources, importcfg...)
	if err != nil {
		return Toolchain{}, err
	}
	return t.hostModules(ctx, dir, parent, filepath.Join(parent, "native"))
}

// hostOverlay writes the host's sources beside the module and names them in an
// overlay.
func (t Toolchain) hostOverlay(dir, parent string, sources map[string][]byte, importcfg ...string) (Toolchain, error) {
	if err := os.MkdirAll(parent, 0755); err != nil {
		return Toolchain{}, err
	}
	overlay := struct{ Replace map[string]string }{map[string]string{}}
	deps := "SKGO_HOST_DEPENDENCIES"
	if len(importcfg) > 1 {
		return Toolchain{}, fmt.Errorf("expected one compiler importcfg")
	}
	if len(importcfg) == 1 {
		var err error
		deps, err = dependencyIdentity(importcfg[0])
		if err != nil {
			return Toolchain{}, err
		}
	}
	for name, body := range sources {
		if filepath.Base(name) == "skgo_hot.go" {
			body = []byte(strings.Replace(string(body), `const _skgo_build_identity = "SKGO_HOST_IDENTITY"`, `const _skgo_build_identity = "`+t.Identity+`"`, 1))
			body = []byte(strings.Replace(string(body), `const _skgo_dependency_identity = "SKGO_HOST_DEPENDENCIES"`, `const _skgo_dependency_identity = "`+deps+`"`, 1))
		}
		rel, err := filepath.Rel(dir, name)
		if err != nil {
			return Toolchain{}, err
		}
		if rel == ".." || filepath.IsAbs(rel) || len(rel) > 3 && rel[:3] == "../" {
			return Toolchain{}, fmt.Errorf("host source outside module: %s", name)
		}
		target := filepath.Join(parent, "sources", rel)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return Toolchain{}, err
		}
		if err := os.WriteFile(target, body, 0644); err != nil {
			return Toolchain{}, err
		}
		overlay.Replace[name] = target
	}
	raw, err := json.Marshal(overlay)
	if err != nil {
		return Toolchain{}, err
	}
	t.Overlay = filepath.Join(parent, "overlay.json")
	if err := os.WriteFile(t.Overlay, raw, 0644); err != nil {
		return Toolchain{}, err
	}
	return t, nil
}

// hostModules gives the host a private module file that also requires the
// native loader, which is prepared under loaders.
func (t Toolchain) hostModules(ctx context.Context, dir, parent, loaders string) (Toolchain, error) {
	mod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return Toolchain{}, err
	}
	t.ModFile = filepath.Join(parent, "host.mod")
	if err := os.WriteFile(t.ModFile, mod, 0644); err != nil {
		return Toolchain{}, err
	}
	if sums, err := os.ReadFile(filepath.Join(dir, "go.sum")); err == nil {
		if err := os.WriteFile(filepath.Join(parent, "host.sum"), sums, 0644); err != nil {
			return Toolchain{}, err
		}
	} else if !os.IsNotExist(err) {
		return Toolchain{}, err
	}
	// -modfile is forbidden in workspace mode. Prepare loader requirements in
	// the private modfile, then add the private loader as a workspace member.
	// All captured application members and workspace replacements stay selected.
	work := t.envValue("GOWORK")
	loaderTC := t
	if work != "" && work != "off" {
		loaderTC.Env = append(t.environment(), "GOWORK=off")
	}
	loader, err := loaderTC.PrepareLoader(ctx, dir, loaders)
	if err != nil {
		return Toolchain{}, err
	}
	if work != "" && work != "off" {
		raw, err := os.ReadFile(work)
		if err != nil {
			return Toolchain{}, err
		}
		wf, err := modfile.ParseWork(work, raw, nil)
		if err != nil {
			return Toolchain{}, err
		}
		if err = wf.AddUse(loader, ""); err != nil {
			return Toolchain{}, err
		}
		privateWork := filepath.Join(parent, "go.work")
		if err = os.WriteFile(privateWork, modfile.Format(wf.Syntax), 0644); err != nil {
			return Toolchain{}, err
		}
		t.ModFile = ""
		t.Env = append(t.environment(), "GOWORK="+privateWork)
	}
	return t, nil
}
